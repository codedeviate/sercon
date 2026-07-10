package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSSecretsManager_ListSecrets(t *testing.T) {
	var gotTarget string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"SecretList": [
				{"Name": "db-password", "ARN": "arn:aws:secretsmanager:eu-north-1:123456789012:secret:db-password-abc123"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerListSecrets(context.Background(), awsConfig{}, awsSecretsManagerArgs{})
	if err != nil {
		t.Fatalf("listSecrets: %v", err)
	}
	if gotTarget != "secretsmanager.ListSecrets" {
		t.Fatalf("expected X-Amz-Target secretsmanager.ListSecrets, got %q", gotTarget)
	}
	m := out.(map[string]any)
	secrets, ok := m["SecretList"].([]any)
	if !ok || len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %#v", m["SecretList"])
	}
	s := secrets[0].(map[string]any)
	if s["Name"] != "db-password" {
		t.Fatalf("expected Name db-password, got %#v", s["Name"])
	}
}

func TestAWSSecretsManager_DescribeSecret(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"Name": "db-password",
			"ARN": "arn:aws:secretsmanager:eu-north-1:123456789012:secret:db-password-abc123"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerDescribeSecret(context.Background(), awsConfig{}, awsSecretsManagerArgs{secretId: "db-password"})
	if err != nil {
		t.Fatalf("describeSecret: %v", err)
	}
	if gotTarget != "secretsmanager.DescribeSecret" {
		t.Fatalf("expected X-Amz-Target secretsmanager.DescribeSecret, got %q", gotTarget)
	}
	if gotBody["SecretId"] != "db-password" {
		t.Fatalf("expected SecretId db-password, got %#v", gotBody["SecretId"])
	}
	m := out.(map[string]any)
	if m["Name"] != "db-password" {
		t.Fatalf("expected Name db-password, got %#v", m["Name"])
	}
}

func TestAWSSecretsManager_CreateSecret(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"Name": "new-secret",
			"ARN": "arn:aws:secretsmanager:eu-north-1:123456789012:secret:new-secret-abc123",
			"VersionId": "v1"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerCreateSecret(context.Background(), awsConfig{}, awsSecretsManagerArgs{
		name: "new-secret", secretString: `{"user":"admin"}`,
	})
	if err != nil {
		t.Fatalf("createSecret: %v", err)
	}
	if gotTarget != "secretsmanager.CreateSecret" {
		t.Fatalf("expected X-Amz-Target secretsmanager.CreateSecret, got %q", gotTarget)
	}
	if gotBody["Name"] != "new-secret" {
		t.Fatalf("expected Name new-secret, got %#v", gotBody["Name"])
	}
	if gotBody["SecretString"] != `{"user":"admin"}` {
		t.Fatalf("expected SecretString to round-trip, got %#v", gotBody["SecretString"])
	}
	m := out.(map[string]any)
	if m["Name"] != "new-secret" {
		t.Fatalf("expected Name new-secret, got %#v", m["Name"])
	}
}

func TestAWSSecretsManager_GetSecretValue_SecretString(t *testing.T) {
	var gotTarget string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"Name": "db-password",
			"SecretString": "hunter2"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerGetSecretValue(context.Background(), awsConfig{}, awsSecretsManagerArgs{secretId: "db-password"})
	if err != nil {
		t.Fatalf("getSecretValue: %v", err)
	}
	if gotTarget != "secretsmanager.GetSecretValue" {
		t.Fatalf("expected X-Amz-Target secretsmanager.GetSecretValue, got %q", gotTarget)
	}
	m := out.(map[string]any)
	if m["value"] != "hunter2" {
		t.Fatalf("expected value hunter2, got %#v", m["value"])
	}
}

// TestAWSSecretsManager_GetSecretValue_SecretBinary proves the SecretBinary
// path: the wire value is base64 (awsjson1.1 blob encoding), decoded by the
// SDK into raw []byte before it reaches our binding, so no further
// base64-decoding must happen in awsSecretsManagerGetSecretValue.
func TestAWSSecretsManager_GetSecretValue_SecretBinary(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		// "binary-secret" base64-encoded, per the awsjson1.1 blob wire format.
		_, _ = w.Write([]byte(`{
			"Name": "bin-secret",
			"SecretBinary": "YmluYXJ5LXNlY3JldA=="
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerGetSecretValue(context.Background(), awsConfig{}, awsSecretsManagerArgs{secretId: "bin-secret"})
	if err != nil {
		t.Fatalf("getSecretValue: %v", err)
	}
	m := out.(map[string]any)
	if m["value"] != "binary-secret" {
		t.Fatalf("expected decoded value binary-secret, got %#v", m["value"])
	}
}

func TestAWSSecretsManager_PutSecretValue(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"Name": "db-password",
			"VersionId": "v2"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerPutSecretValue(context.Background(), awsConfig{}, awsSecretsManagerArgs{
		secretId: "db-password", secretString: "newvalue",
	})
	if err != nil {
		t.Fatalf("putSecretValue: %v", err)
	}
	if gotTarget != "secretsmanager.PutSecretValue" {
		t.Fatalf("expected X-Amz-Target secretsmanager.PutSecretValue, got %q", gotTarget)
	}
	if gotBody["SecretId"] != "db-password" || gotBody["SecretString"] != "newvalue" {
		t.Fatalf("expected SecretId/SecretString to round-trip, got %#v", gotBody)
	}
	m := out.(map[string]any)
	if m["VersionId"] != "v2" {
		t.Fatalf("expected VersionId v2, got %#v", m["VersionId"])
	}
}

func TestAWSSecretsManager_DeleteSecret(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"ARN": "arn:aws:secretsmanager:eu-north-1:123456789012:secret:db-password-abc123",
			"Name": "db-password"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerDeleteSecret(context.Background(), awsConfig{}, awsSecretsManagerArgs{secretId: "db-password"})
	if err != nil {
		t.Fatalf("deleteSecret: %v", err)
	}
	if gotTarget != "secretsmanager.DeleteSecret" {
		t.Fatalf("expected X-Amz-Target secretsmanager.DeleteSecret, got %q", gotTarget)
	}
	if gotBody["SecretId"] != "db-password" {
		t.Fatalf("expected SecretId db-password, got %#v", gotBody["SecretId"])
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

// TestAWSSecretsManager_ErrorPathThrows mirrors TestAWSIAM_ErrorPathThrows:
// proves an awsjson1.1 error response is mapped end to end (SDK response ->
// smithy APIError -> mapAWSError) into a structured awsError.
func TestAWSSecretsManager_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"__type": "ResourceNotFoundException",
			"message": "Secrets Manager can't find the specified secret."
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSecretsManagerGetSecretValue(context.Background(), awsConfig{}, awsSecretsManagerArgs{secretId: "ghost"})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", ae.status)
	}
	if ae.code != "ResourceNotFoundException" {
		t.Fatalf("expected code ResourceNotFoundException, got %q", ae.code)
	}
}

func TestAWSSecretsManager_ListSecrets_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"SecretList": [
				{"Name": "db-password", "ARN": "arn:aws:secretsmanager:eu-north-1:123456789012:secret:db-password-abc123"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).secretsmanager().listSecrets();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	secrets, ok := m["SecretList"].([]any)
	if !ok || len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %#v", m["SecretList"])
	}
}
