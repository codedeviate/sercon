package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSSTS_GetCallerIdentity(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Account>123456789012</Account>
    <Arn>arn:aws:iam::123456789012:user/alice</Arn>
    <UserId>AIDAEXAMPLE1</UserId>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>req-1</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSTSGetCallerIdentity(context.Background(), awsConfig{}, awsSTSArgs{})
	if err != nil {
		t.Fatalf("getCallerIdentity: %v", err)
	}
	if gotAction != "GetCallerIdentity" {
		t.Fatalf("expected Action=GetCallerIdentity, got %q", gotAction)
	}
	m := out.(map[string]any)
	if m["Account"] != "123456789012" {
		t.Fatalf("expected Account 123456789012, got %#v", m["Account"])
	}
	if m["Arn"] != "arn:aws:iam::123456789012:user/alice" {
		t.Fatalf("expected Arn, got %#v", m["Arn"])
	}
}

func TestAWSSTS_AssumeRole(t *testing.T) {
	var gotAction, gotRoleArn, gotSessionName, gotDuration string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotRoleArn = r.Form.Get("RoleArn")
		gotSessionName = r.Form.Get("RoleSessionName")
		gotDuration = r.Form.Get("DurationSeconds")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAEXAMPLE</AccessKeyId>
      <SecretAccessKey>secretvalue</SecretAccessKey>
      <SessionToken>tokenvalue</SessionToken>
      <Expiration>2024-01-01T01:00:00Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::123456789012:assumed-role/my-role/my-session</Arn>
      <AssumedRoleId>AROAEXAMPLE:my-session</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata>
    <RequestId>req-2</RequestId>
  </ResponseMetadata>
</AssumeRoleResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSTSAssumeRole(context.Background(), awsConfig{}, awsSTSArgs{
		roleArn:         "arn:aws:iam::123456789012:role/my-role",
		roleSessionName: "my-session",
		durationSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("assumeRole: %v", err)
	}
	if gotAction != "AssumeRole" {
		t.Fatalf("expected Action=AssumeRole, got %q", gotAction)
	}
	if gotRoleArn != "arn:aws:iam::123456789012:role/my-role" {
		t.Fatalf("expected RoleArn, got %q", gotRoleArn)
	}
	if gotSessionName != "my-session" {
		t.Fatalf("expected RoleSessionName my-session, got %q", gotSessionName)
	}
	if gotDuration != "3600" {
		t.Fatalf("expected DurationSeconds=3600, got %q", gotDuration)
	}
	m := out.(map[string]any)
	creds, ok := m["Credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected Credentials object, got %#v", m["Credentials"])
	}
	if creds["AccessKeyId"] != "ASIAEXAMPLE" {
		t.Fatalf("expected AccessKeyId ASIAEXAMPLE, got %#v", creds["AccessKeyId"])
	}
	if creds["SecretAccessKey"] != "secretvalue" {
		t.Fatalf("expected SecretAccessKey secretvalue, got %#v", creds["SecretAccessKey"])
	}
	if creds["SessionToken"] != "tokenvalue" {
		t.Fatalf("expected SessionToken tokenvalue, got %#v", creds["SessionToken"])
	}
}

func TestAWSSTS_AssumeRole_NoDurationOmitsParam(t *testing.T) {
	var sawDuration bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		_, sawDuration = r.Form["DurationSeconds"]
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <AccessKeyId>ASIAEXAMPLE</AccessKeyId>
      <SecretAccessKey>secretvalue</SecretAccessKey>
      <SessionToken>tokenvalue</SessionToken>
      <Expiration>2024-01-01T01:00:00Z</Expiration>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::123456789012:assumed-role/my-role/my-session</Arn>
      <AssumedRoleId>AROAEXAMPLE:my-session</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata>
    <RequestId>req-2b</RequestId>
  </ResponseMetadata>
</AssumeRoleResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	if _, err := awsSTSAssumeRole(context.Background(), awsConfig{}, awsSTSArgs{
		roleArn: "arn:aws:iam::123456789012:role/my-role", roleSessionName: "my-session",
	}); err != nil {
		t.Fatalf("assumeRole: %v", err)
	}
	if sawDuration {
		t.Fatalf("expected DurationSeconds to be omitted when not provided")
	}
}

func TestAWSSTS_GetSessionToken(t *testing.T) {
	var gotAction, gotDuration string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotDuration = r.Form.Get("DurationSeconds")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetSessionTokenResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetSessionTokenResult>
    <Credentials>
      <AccessKeyId>ASIASESSION</AccessKeyId>
      <SecretAccessKey>sessionsecret</SecretAccessKey>
      <SessionToken>sessiontoken</SessionToken>
      <Expiration>2024-01-01T12:00:00Z</Expiration>
    </Credentials>
  </GetSessionTokenResult>
  <ResponseMetadata>
    <RequestId>req-3</RequestId>
  </ResponseMetadata>
</GetSessionTokenResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSTSGetSessionToken(context.Background(), awsConfig{}, awsSTSArgs{durationSeconds: 900})
	if err != nil {
		t.Fatalf("getSessionToken: %v", err)
	}
	if gotAction != "GetSessionToken" {
		t.Fatalf("expected Action=GetSessionToken, got %q", gotAction)
	}
	if gotDuration != "900" {
		t.Fatalf("expected DurationSeconds=900, got %q", gotDuration)
	}
	m := out.(map[string]any)
	creds, ok := m["Credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected Credentials object, got %#v", m["Credentials"])
	}
	if creds["AccessKeyId"] != "ASIASESSION" {
		t.Fatalf("expected AccessKeyId ASIASESSION, got %#v", creds["AccessKeyId"])
	}
}

// TestAWSSTS_ErrorPathThrows mirrors TestAWSIAM_ErrorPathThrows: proves an STS
// query-protocol error response is mapped end to end (SDK response -> smithy
// APIError -> mapAWSError) into a structured awsError.
func TestAWSSTS_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error>
    <Type>Sender</Type>
    <Code>AccessDenied</Code>
    <Message>User is not authorized to perform sts:AssumeRole.</Message>
  </Error>
  <RequestId>req-err</RequestId>
</ErrorResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSTSAssumeRole(context.Background(), awsConfig{}, awsSTSArgs{
		roleArn: "arn:aws:iam::123456789012:role/denied", roleSessionName: "s",
	})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.status != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", ae.status)
	}
	if ae.code != "AccessDenied" {
		t.Fatalf("expected code AccessDenied, got %q", ae.code)
	}
}

func TestAWSSTS_GetCallerIdentity_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Account>123456789012</Account>
    <Arn>arn:aws:iam::123456789012:user/alice</Arn>
    <UserId>AIDAEXAMPLE1</UserId>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>req-js-1</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).sts().getCallerIdentity();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	if m["Account"] != "123456789012" {
		t.Fatalf("expected Account 123456789012, got %#v", m["Account"])
	}
}
