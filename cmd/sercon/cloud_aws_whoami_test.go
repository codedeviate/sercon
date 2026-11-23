package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const stsIdentityXML = `<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Account>123456789012</Account>
    <Arn>arn:aws:iam::123456789012:user/alice</Arn>
    <UserId>AIDAEXAMPLE1</UserId>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`

func stsErrorServer(t *testing.T, status int, code, msg string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Sender</Type><Code>` + code + `</Code><Message>` + msg + `</Message></Error>
  <RequestId>req-err</RequestId>
</ErrorResponse>`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestAWSWhoami_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(stsIdentityXML))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `const __result = await cloud.aws({ region: "eu-north-1" }).whoami();`)
	want := map[string]any{
		"account": "123456789012",
		"arn":     "arn:aws:iam::123456789012:user/alice",
		"userId":  "AIDAEXAMPLE1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("whoami = %#v, want %#v", got, want)
	}
}

func TestAWSWhoami_ExpiredTokenIsUnauthenticated(t *testing.T) {
	ts := stsErrorServer(t, http.StatusForbidden, "ExpiredToken", "The security token included in the request is expired")
	withMockAWS(t, ts)

	_, err := awsWhoami(context.Background(), awsConfig{})
	var ae awsError
	if !errors.As(err, &ae) {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.code != "UNAUTHENTICATED" || ae.status != http.StatusForbidden {
		t.Fatalf("got code=%q status=%d", ae.code, ae.status)
	}
	if !strings.Contains(ae.message, "aws sso login") || !strings.Contains(ae.message, "ExpiredToken") {
		t.Fatalf("message lacks hint/cause: %q", ae.message)
	}
}

// withFailingAWSCreds routes AWS clients at ts but makes credential
// retrieval fail with err — the shape real SSO/IMDS failures take, which the
// STS client surfaces as "get identity: get credentials: …".
func withFailingAWSCreds(t *testing.T, ts *httptest.Server, err error) {
	t.Helper()
	withMockAWS(t, ts)
	awsTestOptions.creds = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
		return aws.Credentials{}, err
	})
}

func wantAWSUnauth(t *testing.T, err error) {
	t.Helper()
	var ae awsError
	if !errors.As(err, &ae) {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.code != "UNAUTHENTICATED" {
		t.Fatalf("code = %q (%s)", ae.code, ae.message)
	}
	if !strings.Contains(ae.message, "aws sso login") {
		t.Fatalf("message lacks hint: %q", ae.message)
	}
}

func identityServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(stsIdentityXML))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestAWSWhoami_SSOExpiredIsUnauthenticated(t *testing.T) {
	withFailingAWSCreds(t, identityServer(t), &ssocreds.InvalidTokenError{Err: errors.New("the SSO session has expired or is invalid")})
	_, err := awsWhoami(context.Background(), awsConfig{})
	wantAWSUnauth(t, err)
}

func TestAWSWhoami_SSOOIDCInvalidGrantIsUnauthenticated(t *testing.T) {
	withFailingAWSCreds(t, identityServer(t), &smithy.GenericAPIError{Code: "InvalidGrantException", Message: "refresh token expired"})
	_, err := awsWhoami(context.Background(), awsConfig{})
	wantAWSUnauth(t, err)
}

func TestAWSWhoami_NoCredentialsIMDSIsUnauthenticated(t *testing.T) {
	// No creds anywhere: the chain ends at IMDS, which fails with a network
	// error — that still means "not logged in", not "offline".
	imds := fmt.Errorf("no EC2 IMDS role found, %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("i/o timeout")})
	withFailingAWSCreds(t, identityServer(t), imds)
	_, err := awsWhoami(context.Background(), awsConfig{})
	wantAWSUnauth(t, err)
}

func TestAWSWhoami_CredentialFetchOfflineIsTransport(t *testing.T) {
	// A valid SSO login, but the SSO endpoint is unreachable: offline.
	offline := &smithyhttp.RequestSendError{Err: &net.DNSError{Err: "no such host", Name: "portal.sso.eu-north-1.amazonaws.com"}}
	withFailingAWSCreds(t, identityServer(t), offline)
	_, err := awsWhoami(context.Background(), awsConfig{})
	var ae awsError
	if !errors.As(err, &ae) || ae.code != "" {
		t.Fatalf("expected transport awsError (code \"\"), got %T %v", err, err)
	}
}

func TestAWSWhoami_TransportNotUnauthenticated(t *testing.T) {
	err := classifyAWSAuthError(errors.New("dial tcp: lookup sts.eu-north-1.amazonaws.com: no such host"))
	var ae awsError
	if !errors.As(err, &ae) {
		t.Fatalf("expected awsError, got %T", err)
	}
	if ae.code != "" {
		t.Fatalf("transport error classified as %q", ae.code)
	}
}

func TestAWSWhoami_AccessDeniedPassesThrough(t *testing.T) {
	ts := stsErrorServer(t, http.StatusForbidden, "AccessDenied", "nope")
	withMockAWS(t, ts)

	_, err := awsWhoami(context.Background(), awsConfig{})
	var ae awsError
	if !errors.As(err, &ae) || ae.code != "AccessDenied" {
		t.Fatalf("expected AccessDenied awsError, got %T %v", err, err)
	}
}
