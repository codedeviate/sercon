package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIAM_ListServiceAccounts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accounts":[{"email":"a@p.iam.gserviceaccount.com"},{"email":"b@p.iam.gserviceaccount.com"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamListServiceAccounts(context.Background(), googleConfig{}, iamArgs{project: "p"})
	if err != nil {
		t.Fatalf("listServiceAccounts: %v", err)
	}
	m := out.(map[string]any)
	accounts, ok := m["accounts"].([]any)
	if !ok || len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", m["accounts"])
	}
}

func TestIAM_GetServiceAccount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email":"a@p.iam.gserviceaccount.com","displayName":"A"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamGetServiceAccount(context.Background(), googleConfig{}, iamArgs{project: "p", email: "a@p.iam.gserviceaccount.com"})
	if err != nil {
		t.Fatalf("getServiceAccount: %v", err)
	}
	m := out.(map[string]any)
	if m["email"] != "a@p.iam.gserviceaccount.com" {
		t.Fatalf("expected email a@p.iam.gserviceaccount.com, got %#v", m["email"])
	}
}

func TestIAM_CreateServiceAccount(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email":"new@p.iam.gserviceaccount.com","displayName":"New"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamCreateServiceAccount(context.Background(), googleConfig{}, iamArgs{
		project: "p", accountId: "new", displayName: "New",
	})
	if err != nil {
		t.Fatalf("createServiceAccount: %v", err)
	}
	m := out.(map[string]any)
	if m["email"] != "new@p.iam.gserviceaccount.com" {
		t.Fatalf("expected email new@p.iam.gserviceaccount.com, got %#v", m["email"])
	}
	if gotBody["accountId"] != "new" {
		t.Fatalf("expected uploaded accountId new, got %#v", gotBody["accountId"])
	}
	sa, ok := gotBody["serviceAccount"].(map[string]any)
	if !ok || sa["displayName"] != "New" {
		t.Fatalf("expected uploaded serviceAccount.displayName New, got %#v", gotBody["serviceAccount"])
	}
}

func TestIAM_DeleteServiceAccount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamDeleteServiceAccount(context.Background(), googleConfig{}, iamArgs{project: "p", email: "a@p.iam.gserviceaccount.com"})
	if err != nil {
		t.Fatalf("deleteServiceAccount: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

func TestIAM_ListKeys(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"keys":[{"name":"key-1"},{"name":"key-2"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamListKeys(context.Background(), googleConfig{}, iamArgs{project: "p", email: "a@p.iam.gserviceaccount.com"})
	if err != nil {
		t.Fatalf("listKeys: %v", err)
	}
	m := out.(map[string]any)
	keys, ok := m["keys"].([]any)
	if !ok || len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %#v", m["keys"])
	}
}

func TestIAM_CreateKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"key-3","privateKeyData":"c2VjcmV0"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamCreateKey(context.Background(), googleConfig{}, iamArgs{project: "p", email: "a@p.iam.gserviceaccount.com"})
	if err != nil {
		t.Fatalf("createKey: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "key-3" {
		t.Fatalf("expected name key-3, got %#v", m["name"])
	}
}

func TestIAM_GetIamPolicy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bindings":[{"role":"roles/viewer","members":["user:x@example.com"]}],"etag":"abc"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamGetIamPolicy(context.Background(), googleConfig{}, iamArgs{resource: "projects/p/serviceAccounts/a@p.iam.gserviceaccount.com"})
	if err != nil {
		t.Fatalf("getIamPolicy: %v", err)
	}
	m := out.(map[string]any)
	bindings, ok := m["bindings"].([]any)
	if !ok || len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %#v", m["bindings"])
	}
}

func TestIAM_SetIamPolicy(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bindings":[{"role":"roles/viewer","members":["user:x@example.com"]}],"etag":"def"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := iamSetIamPolicy(context.Background(), googleConfig{}, iamArgs{
		resource: "projects/p/serviceAccounts/a@p.iam.gserviceaccount.com",
		policy: map[string]any{
			"bindings": []any{
				map[string]any{"role": "roles/viewer", "members": []any{"user:x@example.com"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("setIamPolicy: %v", err)
	}
	m := out.(map[string]any)
	if m["etag"] != "def" {
		t.Fatalf("expected etag def, got %#v", m["etag"])
	}
	policyBody, ok := gotBody["policy"].(map[string]any)
	if !ok {
		t.Fatalf("expected uploaded body to have policy key, got %#v", gotBody)
	}
	bindings, ok := policyBody["bindings"].([]any)
	if !ok || len(bindings) != 1 {
		t.Fatalf("expected uploaded policy.bindings to have 1 entry, got %#v", policyBody["bindings"])
	}
}

// TestIAM_ListServiceAccounts_ViaJS exercises the full script path (cloud
// namespace registration, the iam() accessor's .Func unwrap, iamExtract's
// goja .Export of the options object, and Promise resolution via await) —
// the same pattern TestCompute_ListInstances_ViaJS established for Task 6.
func TestIAM_ListServiceAccounts_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accounts":[{"email":"a@p.iam.gserviceaccount.com"},{"email":"b@p.iam.gserviceaccount.com"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	got := runCloudScript(t, `
		const c = await cloud.google({ project: "p" }).iam().listServiceAccounts({ project: "p" });
		const __result = c;
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%#v)", got, got)
	}
	accounts, ok := m["accounts"].([]any)
	if !ok || len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", m["accounts"])
	}
}
