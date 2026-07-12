package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// azureConfig is the resolved config for a cloud.azure(...) handle. clientSecret
// and any token material are NEVER logged.
type azureConfig struct {
	subscriptionID, tenantID, clientID, clientSecret string
}

// azureTestSeam (tests only) supplies a stub token credential + a custom
// transport so clients hit an httptest server without real Azure auth. nil in
// production.
//
// endpoint (added alongside the first ARM SDK client consumer, resourceGroups
// in Task 4) is the httptest server's base URL. The azureCallWork escape
// hatch routes requests manually (its own endpointBase field) and only needs
// the Transport override to reach an httptest server. But a generated ARM SDK
// client (armresources.NewResourceGroupsClient et al.) resolves its target
// host from arm.ClientOptions.Cloud — set via cloud.ResourceManager below —
// *before* handing the request to the Transport; a Transport override alone
// still targets the real https://management.azure.com and, over a real
// network, would actually leave the sandbox. endpoint closes that gap so
// every ARM client (Tasks 4-8) is routed to the mock server.
type azureTestSeam struct {
	transport policy.Transporter
	cred      azcore.TokenCredential
	endpoint  string
}

var azureTestOptions *azureTestSeam

// parseAzureConfig reads the optional first-argument options object. Runs on
// the event loop (inside the host call), so touching goja values is safe here.
// Type assertions are guarded: a JS array or other non-object value must
// produce a catchable error, not a panic (Phase 1/2 lesson).
func parseAzureConfig(vm *goja.Runtime, call goja.FunctionCall) (azureConfig, error) {
	var cfg azureConfig
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return cfg, nil
	}
	obj, ok := arg.(*goja.Object)
	if !ok {
		return cfg, errors.New("cloud.azure: options must be an object")
	}
	opts, ok := obj.Export().(map[string]any)
	if !ok {
		return cfg, errors.New("cloud.azure: options must be an object")
	}
	cfg.subscriptionID = optString(opts, "subscriptionId", "")
	cfg.tenantID = optString(opts, "tenantId", "")
	cfg.clientID = optString(opts, "clientId", "")
	cfg.clientSecret = optString(opts, "clientSecret", "")
	return cfg, nil
}

// credential resolves the token credential: the test stub when the seam is
// active, else an explicit client-secret credential, else the default chain
// (env / managed identity / az login).
func (c azureConfig) credential() (azcore.TokenCredential, error) {
	if azureTestOptions != nil {
		return azureTestOptions.cred, nil
	}
	if c.tenantID != "" && c.clientID != "" && c.clientSecret != "" {
		cred, err := azidentity.NewClientSecretCredential(c.tenantID, c.clientID, c.clientSecret, nil)
		if err != nil {
			return nil, mapAzureError(err)
		}
		return cred, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return cred, nil
}

// subscription resolves the subscription ID: explicit config, else
// AZURE_SUBSCRIPTION_ID, else an error (ARM services + the call escape hatch
// all call this).
func (c azureConfig) subscription() (string, error) {
	if c.subscriptionID != "" {
		return c.subscriptionID, nil
	}
	if s := os.Getenv("AZURE_SUBSCRIPTION_ID"); s != "" {
		return s, nil
	}
	return "", errors.New("cloud.azure: subscriptionId is required (set it in cloud.azure({subscriptionId}) or AZURE_SUBSCRIPTION_ID)")
}

// armClientOptions injects the test transport (and, when set, the test
// endpoint override) when the seam is active, else nil (SDK defaults).
func (c azureConfig) armClientOptions() *arm.ClientOptions {
	if azureTestOptions != nil && azureTestOptions.transport != nil {
		opts := &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
			Transport: azureTestOptions.transport,
			// The mock server is a plain httptest.NewServer (http://), and the
			// SDK's bearer-token policy refuses to attach an Authorization
			// header to a non-HTTPS request unless explicitly told this is
			// safe. Test-only: azureTestOptions is nil in production.
			InsecureAllowCredentialWithHTTP: true,
		}}
		if azureTestOptions.endpoint != "" {
			opts.Cloud = cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {Endpoint: azureTestOptions.endpoint, Audience: "https://management.azure.com"},
				},
			}
		}
		return opts
	}
	return nil
}

// coreClientOptions injects the test transport (and, when the seam is
// active, permission to attach a bearer token to a plain-HTTP request) for
// data-plane clients (blob, keyvaultSecrets — Tasks 7-8), else a zero-value
// azcore.ClientOptions (SDK defaults). Unlike armClientOptions, this never
// sets a Cloud/endpoint override: a data-plane client is built directly from
// the caller-supplied accountURL/vaultURL, which in tests already points at
// the mock server — there is no separate "real" host to redirect away from.
func (c azureConfig) coreClientOptions() azcore.ClientOptions {
	if azureTestOptions != nil && azureTestOptions.transport != nil {
		return azcore.ClientOptions{
			Transport:                       azureTestOptions.transport,
			InsecureAllowCredentialWithHTTP: true,
		}
	}
	return azcore.ClientOptions{}
}

// String renders a redacted summary; clientSecret is NEVER included.
func (c azureConfig) String() string {
	auth := "default-chain"
	if c.clientSecret != "" {
		auth = "client-secret(redacted)"
	}
	return fmt.Sprintf("azureConfig{subscription:%q tenant:%q client:%q auth:%s}", c.subscriptionID, c.tenantID, c.clientID, auth)
}

// azureHandle builds the object returned by cloud.azure(...): one accessor
// per service namespace, plus the generic ARM call escape hatch.
// resourceGroups (Task 4), compute (Task 5), resources (Task 6), blob
// (Task 7), and keyvaultSecrets (Task 8) are all now implemented — no temp
// stubs remain.
func azureHandle(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig) map[string]any {
	return map[string]any{
		"resourceGroups": func(goja.FunctionCall) goja.Value { return vm.ToValue(azureResourceGroups(vm, loop, cfg)) },
		"compute":        func(goja.FunctionCall) goja.Value { return vm.ToValue(azureCompute(vm, loop, cfg)) },
		"resources":      func(goja.FunctionCall) goja.Value { return vm.ToValue(azureResources(vm, loop, cfg)) },
		// blob and keyvaultSecrets are data-plane services (Tasks 7-8): the
		// accessor reads the endpoint URL argument on-loop (goja values are
		// only safe to touch here) and passes it through — the client is
		// built from that URL, not from cfg's subscription.
		"blob": func(call goja.FunctionCall) goja.Value {
			url := call.Argument(0).String()
			return vm.ToValue(azureBlob(vm, loop, cfg, url))
		},
		"keyvaultSecrets": func(call goja.FunctionCall) goja.Value {
			url := call.Argument(0).String()
			return vm.ToValue(azureKeyvaultSecrets(vm, loop, cfg, url))
		},
		"call": scriptengine.PromisifyAsync(vm, loop, azureCallExtract(cfg),
			func(ctx context.Context, a azureCallArgs) (any, error) { return azureCallWork(ctx, cfg, a) }).Func,
	}
}

// azureCallArgs is the plain-Go carrier for cloud.azure(...).call({...}),
// extracted on-loop by azureCallExtract and consumed off-loop by
// azureCallWork.
type azureCallArgs struct {
	path       string
	apiVersion string
	method     string
	params     map[string]string
	body       any
	// endpointBase is test-only and JS-unreachable: azureCallExtract never
	// populates it. Empty ⇒ https://management.azure.com.
	endpointBase string
}

// azureCallExtract runs on the event loop: read + validate JS args. Does NOT
// read endpointBase — that field is only ever set directly by Go tests.
func azureCallExtract(cfg azureConfig) func(goja.FunctionCall) (azureCallArgs, error) {
	return func(call goja.FunctionCall) (azureCallArgs, error) {
		obj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return azureCallArgs{}, errors.New("cloud.azure.call: an options object is required")
		}
		o, ok := obj.Export().(map[string]any)
		if !ok {
			return azureCallArgs{}, errors.New("cloud.azure.call: an options object is required")
		}
		a := azureCallArgs{
			path:       optString(o, "path", ""),
			apiVersion: optString(o, "apiVersion", ""),
			method:     strings.ToUpper(optString(o, "method", "GET")),
			params:     optStringMap(o, "params"),
			body:       o["body"],
		}
		if a.path == "" || a.apiVersion == "" {
			return a, errors.New("cloud.azure.call: `path` and `apiVersion` are required")
		}
		return a, nil
	}
}

// httpDoer is the minimal interface azureCallWork needs to perform a request.
// Both policy.Transporter and *http.Client satisfy it, so the test seam's
// transport and the production http.DefaultClient are interchangeable.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// azureCallWork runs off the event loop: acquire an ARM bearer token, perform
// the REST call, decode JSON. Every error path is mapped through
// mapAzureError (or an azureError for a non-2xx response).
func azureCallWork(ctx context.Context, cfg azureConfig, a azureCallArgs) (any, error) {
	cred, err := cfg.credential()
	if err != nil {
		return nil, err
	}
	token, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://management.azure.com/.default"}})
	if err != nil {
		return nil, mapAzureError(err)
	}

	base := a.endpointBase
	if base == "" {
		base = "https://management.azure.com"
	}
	u, err := url.Parse(base + a.path)
	if err != nil {
		return nil, mapAzureError(err)
	}
	q := u.Query()
	q.Set("api-version", a.apiVersion)
	for k, v := range a.params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	var bodyReader io.Reader
	if a.body != nil {
		bb, mErr := json.Marshal(a.body)
		if mErr != nil {
			return nil, azureError{message: "cloud.azure.call: body is not JSON-serialisable"}
		}
		bodyReader = bytes.NewReader(bb)
	}
	req, err := http.NewRequestWithContext(ctx, a.method, u.String(), bodyReader)
	if err != nil {
		return nil, mapAzureError(err)
	}
	req.Header.Set("Authorization", "Bearer "+token.Token)
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	var doer httpDoer
	if azureTestOptions != nil && azureTestOptions.transport != nil {
		doer = azureTestOptions.transport
	} else {
		doer = http.DefaultClient
	}
	resp, err := doer.Do(req)
	if err != nil {
		return nil, mapAzureError(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, azureError{status: resp.StatusCode, message: strings.TrimSpace(string(raw))}
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, azureError{status: resp.StatusCode, message: "cloud.azure.call: response was not JSON"}
	}
	return out, nil
}

type azureError struct {
	code    string
	status  int
	message string
	details any
}

func (e azureError) Error() string {
	return fmt.Sprintf("cloud.azure: %s (%d): %s", e.code, e.status, e.message)
}

func (e azureError) ErrorFields() map[string]any {
	return map[string]any{"code": e.code, "status": e.status, "message": e.message, "details": e.details}
}

// mapAzureError normalises an azcore.ResponseError into a structured
// azureError. Non-response errors (transport, credential/token acquisition)
// map to code "" / status 0 with the raw error text as the message.
func mapAzureError(err error) error {
	if err == nil {
		return nil
	}
	var out azureError
	var re *azcore.ResponseError
	if errors.As(err, &re) {
		out.code = re.ErrorCode
		out.status = re.StatusCode
		out.message = re.Error()
	} else {
		out.message = err.Error()
	}
	return out
}
