package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// googleLoginHint is appended to every UNAUTHENTICATED message so a script
// that surfaces the error tells the user how to fix it.
const googleLoginHint = "not logged in to Google Cloud; run `gcloud auth application-default login` (or set GOOGLE_APPLICATION_CREDENTIALS)"

const googleTokeninfoURL = "https://oauth2.googleapis.com/tokeninfo"

const googleDefaultScope = "https://www.googleapis.com/auth/cloud-platform"

// googleWhoamiTestSeam lets tests supply credentials and a tokeninfo endpoint.
// googleWhoamiSeam is nil in production.
type googleWhoamiTestSeam struct {
	creds        *google.Credentials
	tokeninfoURL string
}

var googleWhoamiSeam *googleWhoamiTestSeam

func googleUnauthenticated(cause string) error {
	return googleError{code: http.StatusUnauthorized, status: "UNAUTHENTICATED", message: cause + " — " + googleLoginHint}
}

// googleWhoami resolves cfg's credentials the way the handle's other calls do
// (explicit credentials file/JSON, else Application Default Credentials) and
// reports the identity behind them. See googleWhoamiFrom for the result.
func googleWhoami(ctx context.Context, cfg googleConfig) (map[string]any, error) {
	if s := googleWhoamiSeam; s != nil {
		return googleWhoamiFrom(ctx, s.creds, cfg.project, s.tokeninfoURL, http.DefaultClient)
	}
	scopes := cfg.scopes
	if len(scopes) == 0 {
		scopes = []string{googleDefaultScope}
	}
	params := google.CredentialsParams{Scopes: scopes}

	raw := cfg.credentialsJSON
	if cfg.credentialsFile != "" {
		b, err := os.ReadFile(cfg.credentialsFile)
		if err != nil {
			return nil, googleUnauthenticated(fmt.Sprintf("cannot read credentials file: %v", err))
		}
		raw = b
	}
	var creds *google.Credentials
	var err error
	if len(raw) > 0 {
		var head struct {
			Type string `json:"type"`
		}
		if jerr := json.Unmarshal(raw, &head); jerr != nil {
			return nil, googleUnauthenticated("credentials are not valid JSON")
		}
		creds, err = google.CredentialsFromJSONWithTypeAndParams(ctx, raw, google.CredentialsType(head.Type), params)
	} else {
		creds, err = google.FindDefaultCredentialsWithParams(ctx, params)
	}
	if err != nil {
		return nil, googleUnauthenticated(err.Error())
	}
	return googleWhoamiFrom(ctx, creds, cfg.project, googleTokeninfoURL, http.DefaultClient)
}

// googleWhoamiFrom fetches a token from creds and asks tokeninfo whose it is.
// Result: email (omitted if unknown), type, project (handleProject, else the
// credentials' project; omitted if neither), scopes, expiresIn (seconds).
// The access token is sent to tokeninfo only — never returned or put in an
// error.
func googleWhoamiFrom(ctx context.Context, creds *google.Credentials, handleProject, tokeninfoURL string, hc *http.Client) (map[string]any, error) {
	tok, err := creds.TokenSource.Token()
	if err != nil {
		return nil, classifyGoogleTokenError(err)
	}

	u, err := url.Parse(tokeninfoURL)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	q := u.Query()
	q.Set("access_token", tok.AccessToken)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, googleError{code: 0, status: "TRANSPORT", message: "cloud.google.whoami: cannot build tokeninfo request"}
	}
	resp, err := hc.Do(req)
	if err != nil {
		// *url.Error embeds the URL, which carries the token: drop it.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, mapGoogleError(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		return nil, googleUnauthenticated("tokeninfo rejected the access token: " + strings.TrimSpace(string(body)))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, googleError{code: resp.StatusCode, status: http.StatusText(resp.StatusCode), message: strings.TrimSpace(string(body))}
	}
	var info struct {
		Email     string      `json:"email"`
		Scope     string      `json:"scope"`
		ExpiresIn json.Number `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, googleError{code: resp.StatusCode, status: "DECODE", message: "cloud.google.whoami: tokeninfo response was not JSON"}
	}

	var key struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
	}
	if len(creds.JSON) > 0 {
		_ = json.Unmarshal(creds.JSON, &key)
	}
	out := map[string]any{
		"type":   googleCredType(creds.JSON),
		"scopes": strings.Fields(info.Scope),
	}
	if n, err := strconv.Atoi(info.ExpiresIn.String()); err == nil {
		out["expiresIn"] = n
	}
	email := info.Email
	if email == "" {
		email = key.ClientEmail
	}
	if email != "" {
		out["email"] = email
	}
	project := handleProject
	if project == "" {
		project = creds.ProjectID
	}
	if project != "" {
		out["project"] = project
	}
	return out, nil
}

// classifyGoogleTokenError maps a failure to obtain an access token. The
// token endpoint rejecting the login (an OAuth error code, or a 400/401) is
// UNAUTHENTICATED; a token-endpoint outage (5xx, 429, …) keeps its HTTP
// status; a network failure stays TRANSPORT. Impersonated and
// external-account credentials flatten the source RetrieveError into a plain
// string (fmt %v), so those are recognised by their text — but only when the
// error is not a network error.
func classifyGoogleTokenError(err error) error {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorCode == "" && re.Response != nil &&
			re.Response.StatusCode != http.StatusBadRequest && re.Response.StatusCode != http.StatusUnauthorized {
			return googleError{code: re.Response.StatusCode, status: http.StatusText(re.Response.StatusCode),
				message: "token endpoint: " + strings.TrimSpace(string(re.Body))}
		}
		cause := re.ErrorCode
		if re.ErrorDescription != "" {
			cause += ": " + re.ErrorDescription
		}
		if cause == "" {
			cause = "token refresh rejected"
		}
		return googleUnauthenticated(cause)
	}
	var ue *url.Error
	var ne net.Error
	if !errors.As(err, &ue) && !errors.As(err, &ne) {
		msg := err.Error()
		for _, marker := range []string{"invalid_grant", "invalid_rapt", "status code 400", "status code 401"} {
			if strings.Contains(msg, marker) {
				return googleUnauthenticated(msg)
			}
		}
	}
	return mapGoogleError(err)
}

// googleCredType names the kind of login behind credentials JSON: "metadata"
// when there is none (GCE/GKE/Cloud Run metadata server), "user" for gcloud
// user credentials, else the JSON "type" verbatim.
func googleCredType(raw []byte) string {
	if len(raw) == 0 {
		return "metadata"
	}
	var head struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &head)
	switch head.Type {
	case "authorized_user":
		return "user"
	case "":
		return "unknown"
	default:
		return head.Type
	}
}
