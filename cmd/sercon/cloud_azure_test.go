package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

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
