package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSLambda_ListFunctions(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Functions": [
				{"FunctionName": "my-func", "Runtime": "nodejs20.x"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaListFunctions(context.Background(), awsConfig{}, awsLambdaArgs{})
	if err != nil {
		t.Fatalf("listFunctions: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/2015-03-31/functions" {
		t.Fatalf("expected GET /2015-03-31/functions, got %s %s", gotMethod, gotPath)
	}
	m := out.(map[string]any)
	fns, ok := m["Functions"].([]any)
	if !ok || len(fns) != 1 {
		t.Fatalf("expected 1 function, got %#v", m["Functions"])
	}
	f := fns[0].(map[string]any)
	if f["FunctionName"] != "my-func" {
		t.Fatalf("expected FunctionName my-func, got %#v", f["FunctionName"])
	}
}

func TestAWSLambda_GetFunction(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Configuration": {"FunctionName": "my-func", "Runtime": "nodejs20.x"}
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaGetFunction(context.Background(), awsConfig{}, awsLambdaArgs{functionName: "my-func"})
	if err != nil {
		t.Fatalf("getFunction: %v", err)
	}
	if gotPath != "/2015-03-31/functions/my-func" {
		t.Fatalf("expected path /2015-03-31/functions/my-func, got %q", gotPath)
	}
	m := out.(map[string]any)
	cfg, ok := m["Configuration"].(map[string]any)
	if !ok || cfg["FunctionName"] != "my-func" {
		t.Fatalf("expected Configuration.FunctionName my-func, got %#v", m["Configuration"])
	}
}

// TestAWSLambda_Invoke proves the special-cased return shape: out.Payload
// ([]byte on the wire, the raw HTTP response body) must come back as a JS
// string, not a numeric byte array, and statusCode must reflect the HTTP
// response status Invoke succeeded with.
func TestAWSLambda_Invoke(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"echo":42}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaInvoke(context.Background(), awsConfig{}, awsLambdaArgs{
		functionName: "my-func",
		payload:      map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/2015-03-31/functions/my-func/invocations" {
		t.Fatalf("expected POST .../my-func/invocations, got %s %s", gotMethod, gotPath)
	}
	if string(gotBody) != `{"key":"value"}` {
		t.Fatalf("expected marshalled payload in request body, got %q", string(gotBody))
	}
	m := out.(map[string]any)
	if m["statusCode"] != int32(200) {
		t.Fatalf("expected statusCode 200, got %#v", m["statusCode"])
	}
	if m["payload"] != `{"ok":true,"echo":42}` {
		t.Fatalf("expected payload string, got %#v", m["payload"])
	}
	if _, present := m["functionError"]; present {
		t.Fatalf("expected no functionError key, got %#v", m["functionError"])
	}
}

// TestAWSLambda_Invoke_StringPayload proves a string opts.payload is passed
// through as raw UTF-8 bytes, not re-marshalled as a JSON string.
func TestAWSLambda_Invoke_StringPayload(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`"done"`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaInvoke(context.Background(), awsConfig{}, awsLambdaArgs{
		functionName: "my-func",
		payload:      `{"raw":"json"}`,
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if string(gotBody) != `{"raw":"json"}` {
		t.Fatalf("expected raw string bytes in request body, got %q", string(gotBody))
	}
	m := out.(map[string]any)
	if m["payload"] != `"done"` {
		t.Fatalf("expected payload string, got %#v", m["payload"])
	}
}

func TestAWSLambda_CreateFunction(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"FunctionName": "my-func",
			"Runtime": "nodejs20.x",
			"Role": "arn:aws:iam::123456789012:role/lambda-role",
			"Handler": "index.handler"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaCreateFunction(context.Background(), awsConfig{}, awsLambdaArgs{
		functionName: "my-func",
		role:         "arn:aws:iam::123456789012:role/lambda-role",
		runtime:      "nodejs20.x",
		handler:      "index.handler",
		zipFile:      []byte("fake-zip-bytes"),
	})
	if err != nil {
		t.Fatalf("createFunction: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/2015-03-31/functions" {
		t.Fatalf("expected POST /2015-03-31/functions, got %s %s", gotMethod, gotPath)
	}
	if gotBody["FunctionName"] != "my-func" {
		t.Fatalf("expected FunctionName my-func, got %#v", gotBody["FunctionName"])
	}
	if gotBody["Runtime"] != "nodejs20.x" {
		t.Fatalf("expected Runtime nodejs20.x, got %#v", gotBody["Runtime"])
	}
	if gotBody["Handler"] != "index.handler" {
		t.Fatalf("expected Handler index.handler, got %#v", gotBody["Handler"])
	}
	code, ok := gotBody["Code"].(map[string]any)
	if !ok {
		t.Fatalf("expected Code object, got %#v", gotBody["Code"])
	}
	if code["ZipFile"] != "ZmFrZS16aXAtYnl0ZXM=" { // base64("fake-zip-bytes")
		t.Fatalf("expected base64-encoded ZipFile, got %#v", code["ZipFile"])
	}
	m := out.(map[string]any)
	if m["FunctionName"] != "my-func" {
		t.Fatalf("expected FunctionName my-func in response, got %#v", m["FunctionName"])
	}
}

func TestAWSLambda_DeleteFunction(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaDeleteFunction(context.Background(), awsConfig{}, awsLambdaArgs{functionName: "my-func"})
	if err != nil {
		t.Fatalf("deleteFunction: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/2015-03-31/functions/my-func" {
		t.Fatalf("expected DELETE /2015-03-31/functions/my-func, got %s %s", gotMethod, gotPath)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

// TestAWSLambda_ErrorPathThrows mirrors TestAWSSecretsManager_ErrorPathThrows:
// proves a restjson1 error response (JSON body with __type/message, no
// X-Amzn-Errortype header) is mapped end to end (SDK response -> smithy
// APIError -> mapAWSError) into a structured awsError.
func TestAWSLambda_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
			"__type": "ResourceNotFoundException",
			"message": "Function not found: my-func"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsLambdaGetFunction(context.Background(), awsConfig{}, awsLambdaArgs{functionName: "ghost"})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", ae.status)
	}
	if ae.code != "ResourceNotFoundException" {
		t.Fatalf("expected code ResourceNotFoundException, got %q", ae.code)
	}
}

func TestAWSLambda_ListFunctions_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Functions": [
				{"FunctionName": "my-func", "Runtime": "nodejs20.x"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).lambda().listFunctions();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	fns, ok := m["Functions"].([]any)
	if !ok || len(fns) != 1 {
		t.Fatalf("expected 1 function, got %#v", m["Functions"])
	}
}
