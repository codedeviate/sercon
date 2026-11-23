package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
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

func TestAWSWhoami_SigningErrorIsUnauthenticated(t *testing.T) {
	err := classifyAWSAuthError(&v4.SigningError{Err: errors.New("failed to retrieve credentials: token expired")})
	var ae awsError
	if !errors.As(err, &ae) {
		t.Fatalf("expected awsError, got %T", err)
	}
	if ae.code != "UNAUTHENTICATED" || ae.status != 0 {
		t.Fatalf("got code=%q status=%d", ae.code, ae.status)
	}
	if !strings.Contains(ae.message, "aws sso login") {
		t.Fatalf("message lacks hint: %q", ae.message)
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
