package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// stubTokenCredential is a fake azcore.TokenCredential for tests: it never
// touches the network and always returns a fixed, obviously-fake token.
type stubTokenCredential struct{}

func (stubTokenCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "test-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// roundTripTransporter adapts an *http.Client (typically ts.Client() from an
// httptest.Server) to policy.Transporter, so ARM SDK clients — and the
// azureCallWork escape hatch — can be pointed at a local mock server.
type roundTripTransporter struct {
	c *http.Client
}

func (t roundTripTransporter) Do(req *http.Request) (*http.Response, error) {
	return t.c.Do(req)
}

// withMockAzure points cloud.azure's test seam at ts for the duration of the
// test, restoring the previous (nil) seam via t.Cleanup. Reused by Tasks 4-8.
func withMockAzure(t *testing.T, ts *httptest.Server) {
	t.Helper()
	prev := azureTestOptions
	azureTestOptions = &azureTestSeam{
		transport: roundTripTransporter{c: ts.Client()},
		cred:      stubTokenCredential{},
	}
	t.Cleanup(func() { azureTestOptions = prev })
}

func runCloudAzureScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("cloud", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return cloudNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "c.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestCloudAzure_HandleShape(t *testing.T) {
	got := runCloudAzureScript(t, `
		const az = cloud.azure({ subscriptionId: "sub-guid" });
		const __result = {
			isFn: typeof cloud.azure === "function",
			resourceGroups: typeof az.resourceGroups, compute: typeof az.compute,
			resources: typeof az.resources, blob: typeof az.blob,
			keyvaultSecrets: typeof az.keyvaultSecrets, call: typeof az.call,
		};
	`)
	m := got.(map[string]any)
	if m["isFn"] != true {
		t.Fatal("cloud.azure must be callable")
	}
	for _, k := range []string{"resourceGroups", "compute", "resources", "blob", "keyvaultSecrets", "call"} {
		if m[k] != "function" {
			t.Fatalf("expected az.%s to be a function, got %v", k, m[k])
		}
	}
}

func TestAzureConfig_CredsNeverLogged(t *testing.T) {
	c := azureConfig{subscriptionID: "sub", tenantID: "ten", clientID: "cli", clientSecret: "SHHH-SECRET"}
	if s := c.String(); strings.Contains(s, "SHHH-SECRET") {
		t.Fatalf("azureConfig.String() leaked clientSecret: %q", s)
	}
}

func TestMapAzureError(t *testing.T) {
	re := &azcore.ResponseError{ErrorCode: "ResourceGroupNotFound", StatusCode: 404,
		RawResponse: &http.Response{StatusCode: 404}}
	ae, ok := mapAzureError(re).(azureError)
	if !ok {
		t.Fatalf("expected azureError, got %T", mapAzureError(re))
	}
	f := ae.ErrorFields()
	if f["code"] != "ResourceGroupNotFound" || f["status"] != 404 {
		t.Fatalf("bad fields: %#v", f)
	}
}

func TestMapAzureError_PlainError(t *testing.T) {
	ae, ok := mapAzureError(errors.New("dial tcp: connection refused")).(azureError)
	if !ok {
		t.Fatalf("expected azureError, got %T", mapAzureError(errors.New("x")))
	}
	f := ae.ErrorFields()
	if f["code"] != "" || f["status"] != 0 {
		t.Fatalf("expected zero code/status for non-response error, got %#v", f)
	}
	if f["message"] != "dial tcp: connection refused" {
		t.Fatalf("expected raw message passthrough, got %#v", f["message"])
	}
}

func TestMapAzureError_Nil(t *testing.T) {
	if got := mapAzureError(nil); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func TestAzureCall_GET(t *testing.T) {
	var gotQuery string
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"rg1"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureCallWork(context.Background(), azureConfig{subscriptionID: "s"}, azureCallArgs{
		endpointBase: ts.URL,
		path:         "/subscriptions/s/resourcegroups",
		apiVersion:   "2021-04-01",
	})
	if err != nil {
		t.Fatalf("azureCallWork error: %v", err)
	}

	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["name"] != "rg1" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}

	if !strings.Contains(gotQuery, "api-version=2021-04-01") {
		t.Fatalf("expected api-version in query, got %q", gotQuery)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("expected Authorization: Bearer test-token, got %q", gotAuth)
	}
}

func TestAzureCall_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ResourceGroupNotFound","message":"not found"}}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	_, err := azureCallWork(context.Background(), azureConfig{subscriptionID: "s"}, azureCallArgs{
		endpointBase: ts.URL,
		path:         "/subscriptions/s/resourcegroups/missing",
		apiVersion:   "2021-04-01",
	})
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
	if !strings.Contains(ae.message, "ResourceGroupNotFound") {
		t.Fatalf("expected message to contain body, got %q", ae.message)
	}
}
