package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// keyvaultChallengeHandler wraps h to emulate Key Vault's two-round
// authentication challenge: azsecrets' bearer-token policy (via
// keyvault/internal's challenge policy) always elicits challenge parameters
// (tenant + resource scope) from an initial 401 WWW-Authenticate response
// before ever attaching a bearer token, then retries the request with
// "Authorization: Bearer ...". A mock server that answered 200 immediately
// would only ever see that unauthorized, bodyless first attempt — for
// SetSecret in particular, the secret value would never actually reach the
// wire. The resource host in the challenge is unchecked by the client here
// because newSecretsClient sets DisableChallengeResourceVerification under
// the withMockAzure test seam (127.0.0.1 can't satisfy the real suffix-match
// rule).
func keyvaultChallengeHandler(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Bearer authorization="https://login.microsoftonline.com/test-tenant", resource="https://vault.azure.net"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// TestAzureKeyvaultSecrets_ListSecrets exercises the "listSecrets" pager
// against a mock server returning the JSON shape azsecrets expects for
// Client.NewListSecretPropertiesPager.
func TestAzureKeyvaultSecrets_ListSecrets(t *testing.T) {
	ts := httptest.NewTLSServer(keyvaultChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"value": [
				{"id": "https://vault.example/secrets/secret1"},
				{"id": "https://vault.example/secrets/secret2"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureKeyvaultSecretsListSecrets(context.Background(), azureConfig{}, ts.URL, azureKeyvaultSecretsArgs{})
	if err != nil {
		t.Fatalf("azureKeyvaultSecretsListSecrets error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 2 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["id"] != "https://vault.example/secrets/secret1" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

// TestAzureKeyvaultSecrets_GetSecret exercises GetSecret against a mock
// server returning a JSON Secret body, and asserts the returned "value"
// string is the decoded secret value.
func TestAzureKeyvaultSecrets_GetSecret(t *testing.T) {
	ts := httptest.NewTLSServer(keyvaultChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": "s3cr3t-payload", "id": "https://vault.example/secrets/mysecret"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureKeyvaultSecretsGetSecret(context.Background(), azureConfig{}, ts.URL, azureKeyvaultSecretsArgs{name: "mysecret"})
	if err != nil {
		t.Fatalf("azureKeyvaultSecretsGetSecret error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	got, ok := m["value"].(string)
	if !ok {
		t.Fatalf("expected string value field, got %T", m["value"])
	}
	if got != "s3cr3t-payload" {
		t.Fatalf("expected %q, got %q", "s3cr3t-payload", got)
	}
}

// TestAzureKeyvaultSecrets_SetSecret exercises SetSecret against a mock
// server, asserting the request body carries the secret value and the
// method is PUT (the SDK's "Set Secret" REST operation). This is the test
// that actually exercises the challenge-handshake retry: without it, the
// SDK's first (bodyless, unauthenticated) request would be accepted as the
// final response and gotBody would stay nil.
func TestAzureKeyvaultSecrets_SetSecret(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	ts := httptest.NewTLSServer(keyvaultChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": "new-value", "id": "https://vault.example/secrets/mysecret"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureKeyvaultSecretsSetSecret(context.Background(), azureConfig{}, ts.URL, azureKeyvaultSecretsArgs{
		name: "mysecret", value: "new-value",
	})
	if err != nil {
		t.Fatalf("azureKeyvaultSecretsSetSecret error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if gotBody["value"] != "new-value" {
		t.Fatalf("expected request body to carry value %q, got %#v", "new-value", gotBody)
	}
	if out == nil {
		t.Fatal("expected a non-nil result")
	}
}

// TestAzureKeyvaultSecrets_DeleteSecret exercises DeleteSecret against a
// mock server returning 200 OK with a JSON body (the only status code the
// SDK accepts for this operation).
func TestAzureKeyvaultSecrets_DeleteSecret(t *testing.T) {
	var gotMethod string
	ts := httptest.NewTLSServer(keyvaultChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": "https://vault.example/secrets/mysecret"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureKeyvaultSecretsDeleteSecret(context.Background(), azureConfig{}, ts.URL, azureKeyvaultSecretsArgs{name: "mysecret"})
	if err != nil {
		t.Fatalf("azureKeyvaultSecretsDeleteSecret error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

// TestAzureKeyvaultSecrets_ListSecrets_ViaJS exercises the full JS surface:
// cloud.azure(cfg).keyvaultSecrets(url).listSecrets() — the vault URL is the
// httptest server's own URL, templated straight into the script body (the
// accessor takes the URL as a plain JS argument, not a config field).
func TestAzureKeyvaultSecrets_ListSecrets_ViaJS(t *testing.T) {
	ts := httptest.NewTLSServer(keyvaultChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value": [{"id": "https://vault.example/secrets/fromjs"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	got := runCloudAzureScript(t, fmt.Sprintf(`
		const az = cloud.azure({});
		const __result = await az.keyvaultSecrets(%q).listSecrets();
	`, ts.URL))
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T: %#v", got, got)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["id"] != "https://vault.example/secrets/fromjs" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

// TestAzureKeyvaultSecrets_ErrorPathThrows exercises the error path: a
// non-2xx response from the mock server must be mapped through
// mapAzureError into an azureError, not returned as a plain error or panic.
func TestAzureKeyvaultSecrets_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewTLSServer(keyvaultChallengeHandler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": {"code": "SecretNotFound", "message": "A secret with (name/id) mysecret was not found in this key vault."}}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	_, err := azureKeyvaultSecretsGetSecret(context.Background(), azureConfig{}, ts.URL, azureKeyvaultSecretsArgs{name: "mysecret"})
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	ae, ok := err.(azureError)
	if !ok {
		t.Fatalf("expected azureError, got %T: %v", err, err)
	}
	if ae.status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", ae.status)
	}
}
