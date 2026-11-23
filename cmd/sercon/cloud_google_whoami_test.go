package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// errTokenSource is an oauth2.TokenSource that always fails with err.
type errTokenSource struct{ err error }

func (s errTokenSource) Token() (*oauth2.Token, error) { return nil, s.err }

func staticCreds(json string, project string) *google.Credentials {
	c := &google.Credentials{
		ProjectID:   project,
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok-secret"}),
	}
	if json != "" {
		c.JSON = []byte(json)
	}
	return c
}

// tokeninfoServer replies with body/status and records the query it saw.
func tokeninfoServer(t *testing.T, status int, body string, gotQuery *string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotQuery != nil {
			*gotQuery = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func wantGoogleUnauth(t *testing.T, err error) {
	t.Helper()
	var ge googleError
	if !errors.As(err, &ge) {
		t.Fatalf("expected googleError, got %T: %v", err, err)
	}
	if ge.code != 401 || ge.status != "UNAUTHENTICATED" {
		t.Fatalf("got code=%d status=%q (%s)", ge.code, ge.status, ge.message)
	}
	if !strings.Contains(ge.message, "gcloud auth application-default login") {
		t.Fatalf("message lacks hint: %q", ge.message)
	}
	if strings.Contains(ge.message, "tok-secret") {
		t.Fatalf("token leaked into message: %q", ge.message)
	}
}

func TestGoogleWhoami_User(t *testing.T) {
	var q string
	ts := tokeninfoServer(t, 200, `{"email":"a@b.se","scope":"openid https://www.googleapis.com/auth/cloud-platform","expires_in":"3599"}`, &q)
	creds := staticCreds(`{"type":"authorized_user","quota_project_id":"q"}`, "p1")

	got, err := googleWhoamiFrom(context.Background(), creds, "", ts.URL, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"email":     "a@b.se",
		"type":      "user",
		"project":   "p1",
		"scopes":    []string{"openid", "https://www.googleapis.com/auth/cloud-platform"},
		"expiresIn": 3599,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
	if !strings.Contains(q, "access_token=tok-secret") {
		t.Fatalf("tokeninfo query = %q", q)
	}
	for k, v := range got {
		if s, ok := v.(string); ok && strings.Contains(s, "tok-secret") {
			t.Fatalf("token leaked in %s", k)
		}
	}
}

func TestGoogleWhoami_ServiceAccountEmailFallback(t *testing.T) {
	ts := tokeninfoServer(t, 200, `{"scope":"https://www.googleapis.com/auth/cloud-platform","expires_in":3600}`, nil)
	creds := staticCreds(`{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com"}`, "p")

	got, err := googleWhoamiFrom(context.Background(), creds, "", ts.URL, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got["email"] != "sa@p.iam.gserviceaccount.com" || got["type"] != "service_account" || got["expiresIn"] != 3600 {
		t.Fatalf("got %#v", got)
	}
}

func TestGoogleWhoami_MetadataType(t *testing.T) {
	ts := tokeninfoServer(t, 200, `{"scope":"https://www.googleapis.com/auth/cloud-platform","expires_in":"100"}`, nil)
	creds := staticCreds("", "")

	got, err := googleWhoamiFrom(context.Background(), creds, "", ts.URL, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	if got["type"] != "metadata" {
		t.Fatalf("type = %#v", got["type"])
	}
	if _, ok := got["email"]; ok {
		t.Fatalf("email should be omitted, got %#v", got["email"])
	}
	if _, ok := got["project"]; ok {
		t.Fatalf("project should be omitted, got %#v", got["project"])
	}
}

func TestGoogleWhoami_ProjectPrecedence(t *testing.T) {
	ts := tokeninfoServer(t, 200, `{"email":"a@b.se","scope":"x","expires_in":"1"}`, nil)
	creds := staticCreds(`{"type":"authorized_user"}`, "c")

	got, err := googleWhoamiFrom(context.Background(), creds, "h", ts.URL, ts.Client())
	if err != nil || got["project"] != "h" {
		t.Fatalf("handle project: got %#v err %v", got["project"], err)
	}
	got, err = googleWhoamiFrom(context.Background(), creds, "", ts.URL, ts.Client())
	if err != nil || got["project"] != "c" {
		t.Fatalf("creds project: got %#v err %v", got["project"], err)
	}
}

func TestGoogleWhoami_RefreshRejected(t *testing.T) {
	creds := &google.Credentials{TokenSource: errTokenSource{&oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "reauth related error"}}}
	_, err := googleWhoamiFrom(context.Background(), creds, "", "http://unused.invalid", http.DefaultClient)
	wantGoogleUnauth(t, err)
}

func TestGoogleWhoami_TokeninfoRejects(t *testing.T) {
	ts := tokeninfoServer(t, 400, `{"error":"invalid_token","error_description":"Invalid Value"}`, nil)
	_, err := googleWhoamiFrom(context.Background(), staticCreds(`{"type":"authorized_user"}`, ""), "", ts.URL, ts.Client())
	wantGoogleUnauth(t, err)
}

func TestGoogleWhoami_TransportNotUnauthenticated(t *testing.T) {
	creds := &google.Credentials{TokenSource: errTokenSource{errors.New("dial tcp: lookup oauth2.googleapis.com: no such host")}}
	_, err := googleWhoamiFrom(context.Background(), creds, "", "http://unused.invalid", http.DefaultClient)
	var ge googleError
	if !errors.As(err, &ge) || ge.status != "TRANSPORT" {
		t.Fatalf("expected TRANSPORT googleError, got %T %v", err, err)
	}
}

func TestGoogleWhoami_NoCredentials(t *testing.T) {
	prev := googleWhoamiSeam
	googleWhoamiSeam = nil
	t.Cleanup(func() { googleWhoamiSeam = prev })
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))

	_, err := googleWhoami(context.Background(), googleConfig{})
	wantGoogleUnauth(t, err)
}

func TestGoogleWhoami_ViaJS(t *testing.T) {
	ts := tokeninfoServer(t, 200, `{"email":"a@b.se","scope":"x","expires_in":"60"}`, nil)
	prev := googleWhoamiSeam
	googleWhoamiSeam = &googleWhoamiTestSeam{creds: staticCreds(`{"type":"authorized_user"}`, ""), tokeninfoURL: ts.URL}
	t.Cleanup(func() { googleWhoamiSeam = prev })

	got := runCloudScript(t, `const __result = await cloud.google({ project: "h" }).whoami();`)
	m, ok := got.(map[string]any)
	if !ok || m["email"] != "a@b.se" || m["project"] != "h" || m["type"] != "user" {
		t.Fatalf("got %#v", got)
	}
}

func TestGoogleWhoami_TokeninfoUnreachableDoesNotLeakToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close()
	_, err := googleWhoamiFrom(context.Background(), staticCreds(`{"type":"authorized_user"}`, ""), "", url, http.DefaultClient)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "tok-secret") {
		t.Fatalf("token leaked: %v", err)
	}
	var ge googleError
	if !errors.As(err, &ge) || ge.status != "TRANSPORT" {
		t.Fatalf("expected TRANSPORT, got %T %v", err, err)
	}
}

func TestGoogleWhoami_ImpersonatedSourceExpiredIsUnauthenticated(t *testing.T) {
	// impersonate.go flattens the source RetrieveError with %v.
	inner := &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "Bad Request"}
	creds := &google.Credentials{TokenSource: errTokenSource{fmt.Errorf("impersonate: unable to generate access token: %v", inner)}}
	_, err := googleWhoamiFrom(context.Background(), creds, "", "http://unused.invalid", http.DefaultClient)
	wantGoogleUnauth(t, err)
}

func TestGoogleWhoami_TokenEndpoint5xxIsNotUnauthenticated(t *testing.T) {
	re := &oauth2.RetrieveError{Response: &http.Response{StatusCode: 503}, Body: []byte("unavailable")}
	creds := &google.Credentials{TokenSource: errTokenSource{re}}
	_, err := googleWhoamiFrom(context.Background(), creds, "", "http://unused.invalid", http.DefaultClient)
	var ge googleError
	if !errors.As(err, &ge) || ge.status == "UNAUTHENTICATED" || ge.code != 503 {
		t.Fatalf("expected non-auth 503 googleError, got %T %+v", err, err)
	}
}

func TestGoogleWhoami_Tokeninfo429IsNotUnauthenticated(t *testing.T) {
	ts := tokeninfoServer(t, 429, `{"error":"rate_limited"}`, nil)
	_, err := googleWhoamiFrom(context.Background(), staticCreds(`{"type":"authorized_user"}`, ""), "", ts.URL, ts.Client())
	var ge googleError
	if !errors.As(err, &ge) || ge.status == "UNAUTHENTICATED" || ge.code != 429 {
		t.Fatalf("expected non-auth 429 googleError, got %T %+v", err, err)
	}
}
