package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecrets_ListSecrets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"secrets":[{"name":"projects/p/secrets/a"},{"name":"projects/p/secrets/b"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := secretsListSecrets(context.Background(), googleConfig{}, secretsArgs{project: "p"})
	if err != nil {
		t.Fatalf("listSecrets: %v", err)
	}
	m := out.(map[string]any)
	secrets, ok := m["secrets"].([]any)
	if !ok || len(secrets) != 2 {
		t.Fatalf("expected 2 secrets, got %#v", m["secrets"])
	}
}

func TestSecrets_GetSecret(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/p/secrets/a","etag":"abc"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := secretsGetSecret(context.Background(), googleConfig{}, secretsArgs{project: "p", name: "a"})
	if err != nil {
		t.Fatalf("getSecret: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "projects/p/secrets/a" {
		t.Fatalf("expected name projects/p/secrets/a, got %#v", m["name"])
	}
}

func TestSecrets_CreateSecret(t *testing.T) {
	var gotBody map[string]any
	var gotSecretID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		gotSecretID = r.URL.Query().Get("secretId")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/p/secrets/new"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := secretsCreateSecret(context.Background(), googleConfig{}, secretsArgs{project: "p", name: "new"})
	if err != nil {
		t.Fatalf("createSecret: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "projects/p/secrets/new" {
		t.Fatalf("expected name projects/p/secrets/new, got %#v", m["name"])
	}
	if gotSecretID != "new" {
		t.Fatalf("expected secretId=new query param, got %#v", gotSecretID)
	}
	repl, ok := gotBody["replication"].(map[string]any)
	if !ok {
		t.Fatalf("expected uploaded body to have replication key, got %#v", gotBody)
	}
	if _, ok := repl["automatic"]; !ok {
		t.Fatalf("expected uploaded replication.automatic, got %#v", repl)
	}
}

func TestSecrets_AddSecretVersion(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/p/secrets/a/versions/1"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := secretsAddSecretVersion(context.Background(), googleConfig{}, secretsArgs{
		project: "p", name: "a", payload: "hunter2",
	})
	if err != nil {
		t.Fatalf("addSecretVersion: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "projects/p/secrets/a/versions/1" {
		t.Fatalf("expected version resource name, got %#v", m["name"])
	}
	payload, ok := gotBody["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected uploaded body to have payload key, got %#v", gotBody)
	}
	data, ok := payload["data"].(string)
	if !ok {
		t.Fatalf("expected payload.data string, got %#v", payload["data"])
	}
	if data == "hunter2" {
		t.Fatalf("expected payload.data to be base64-encoded, got raw plaintext")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatalf("uploaded payload.data was not valid base64: %v", err)
	}
	if string(decoded) != "hunter2" {
		t.Fatalf("expected decoded uploaded payload to be hunter2, got %q", decoded)
	}
}

func TestSecrets_AccessSecretVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoded := base64.StdEncoding.EncodeToString([]byte("hunter2"))
		_, _ = w.Write([]byte(`{"name":"projects/p/secrets/a/versions/latest","payload":{"data":"` + encoded + `"}}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := secretsAccessSecretVersion(context.Background(), googleConfig{}, secretsArgs{project: "p", name: "a"})
	if err != nil {
		t.Fatalf("accessSecretVersion: %v", err)
	}
	m := out.(map[string]any)
	if m["value"] != "hunter2" {
		t.Fatalf("expected decoded value hunter2, got %#v", m["value"])
	}
}

func TestSecrets_DeleteSecret(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := secretsDeleteSecret(context.Background(), googleConfig{}, secretsArgs{project: "p", name: "a"})
	if err != nil {
		t.Fatalf("deleteSecret: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

// TestSecrets_AccessSecretVersion_ViaJS exercises the full script path (cloud
// namespace registration, the secrets() accessor's .Func unwrap,
// secretsExtract's goja .Export of the options object, and Promise
// resolution via await) — the same pattern TestIAM_ListServiceAccounts_ViaJS
// established for Task 7. It also verifies the base64 round-trip survives
// the JS boundary: the mock returns base64, JS sees decoded plaintext.
func TestSecrets_AccessSecretVersion_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoded := base64.StdEncoding.EncodeToString([]byte("hunter2"))
		_, _ = w.Write([]byte(`{"name":"projects/p/secrets/a/versions/latest","payload":{"data":"` + encoded + `"}}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	got := runCloudScript(t, `
		const c = await cloud.google({ project: "p" }).secrets().accessSecretVersion({ project: "p", name: "a" });
		const __result = c;
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%#v)", got, got)
	}
	if m["value"] != "hunter2" {
		t.Fatalf("expected decoded value hunter2, got %#v", m["value"])
	}
}
