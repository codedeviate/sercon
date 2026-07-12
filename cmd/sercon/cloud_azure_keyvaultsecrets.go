package main

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// azureKeyvaultSecretsArgs is the plain-Go carrier for every
// cloud.azure(...).keyvaultSecrets(vaultUrl) method: extracted on-loop by
// azureKeyvaultSecretsExtract, consumed off-loop by the
// azureKeyvaultSecretsXxx work funcs. Mirrors azureBlobArgs (Task 7): a
// data-plane service, so no subscription/ARM host is involved — the
// caller-supplied vaultURL *is* the service.
type azureKeyvaultSecretsArgs struct {
	name  string
	value string
}

// newSecretsClient builds an azsecrets.Client from the caller-supplied
// vaultURL. No subscription is involved (data-plane, not ARM) and no
// Cloud/endpoint override is needed either: the vaultURL passed in *is* the
// endpoint, so in tests it already points at the mock server directly. Only
// the transport (and, when the seam is active, permission to attach a
// bearer token to a plain-HTTP request) come from cfg.coreClientOptions().
//
// Unlike blob, azsecrets authenticates via keyvault/internal's
// "challenge policy": every request round-trips once unauthenticated to
// learn the tenant/scope from a 401 WWW-Authenticate challenge, then that
// challenge's resource host is required to be a domain-suffix of the vault
// host (e.g. "vault.azure.net" for "myvault.vault.azure.net") — a mock
// server's 127.0.0.1 host can never satisfy that, so the test seam also
// disables the check (DisableChallengeResourceVerification), same spirit as
// InsecureAllowCredentialWithHTTP above.
func newSecretsClient(cfg azureConfig, vaultURL string) (*azsecrets.Client, error) {
	cred, err := cfg.credential()
	if err != nil {
		return nil, err
	}
	opts := &azsecrets.ClientOptions{ClientOptions: cfg.coreClientOptions()}
	if azureTestOptions != nil {
		opts.DisableChallengeResourceVerification = true
	}
	client, err := azsecrets.NewClient(vaultURL, cred, opts)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return client, nil
}

// azureKeyvaultSecretsListSecrets lists every secret's properties in the
// vault, paging through every page the SDK's pager returns. Only metadata
// is returned (no secret values) — that matches the SDK's own List
// operation, which never includes values.
func azureKeyvaultSecretsListSecrets(ctx context.Context, cfg azureConfig, vaultURL string, a azureKeyvaultSecretsArgs) (any, error) {
	client, err := newSecretsClient(cfg, vaultURL)
	if err != nil {
		return nil, err
	}
	var all []*azsecrets.SecretProperties
	pager := client.NewListSecretPropertiesPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, mapAzureError(err)
		}
		all = append(all, page.Value...)
	}
	plain, err := toPlain(all)
	if err != nil {
		return nil, err
	}
	return map[string]any{"value": plain}, nil
}

// azureKeyvaultSecretsGetSecret fetches the latest version of a.name and
// returns its decoded value under the "value" key. The secret value is
// NEVER logged anywhere in this path — it is only ever placed in the
// returned map for the caller to consume.
func azureKeyvaultSecretsGetSecret(ctx context.Context, cfg azureConfig, vaultURL string, a azureKeyvaultSecretsArgs) (any, error) {
	client, err := newSecretsClient(cfg, vaultURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetSecret(ctx, a.name, "", nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	value := ""
	if resp.Value != nil {
		value = *resp.Value
	}
	return map[string]any{"value": value}, nil
}

// azureKeyvaultSecretsSetSecret creates a new version of a.name with
// a.value. The value is never logged; it is only ever sent over the wire to
// the vault and echoed back (still not logged) in the SDK's response, which
// this returns as-is via toPlain.
func azureKeyvaultSecretsSetSecret(ctx context.Context, cfg azureConfig, vaultURL string, a azureKeyvaultSecretsArgs) (any, error) {
	client, err := newSecretsClient(cfg, vaultURL)
	if err != nil {
		return nil, err
	}
	value := a.value
	resp, err := client.SetSecret(ctx, a.name, azsecrets.SetSecretParameters{Value: &value}, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return toPlain(resp)
}

// azureKeyvaultSecretsDeleteSecret deletes a.name (all versions).
func azureKeyvaultSecretsDeleteSecret(ctx context.Context, cfg azureConfig, vaultURL string, a azureKeyvaultSecretsArgs) (any, error) {
	client, err := newSecretsClient(cfg, vaultURL)
	if err != nil {
		return nil, err
	}
	if _, err := client.DeleteSecret(ctx, a.name, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureKeyvaultSecretsExtract runs on the event loop: read + validate JS
// args. Mirrors azureBlobExtract's shape (optString for plain fields).
func azureKeyvaultSecretsExtract(call goja.FunctionCall) (azureKeyvaultSecretsArgs, error) {
	var a azureKeyvaultSecretsArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.name = optString(o, "name", "")
	a.value = optString(o, "value", "")
	return a, nil
}

// azureKeyvaultSecrets builds the object returned by
// cloud.azure(...).keyvaultSecrets(vaultUrl): one PromisifyAsync binding per
// method, all sharing azureKeyvaultSecretsExtract, cfg, and the vaultURL
// supplied by the caller. This map is built at script-run-time by the
// `keyvaultSecrets` accessor in cloud_azure.go (which reads the URL argument
// on-loop, past the engine's registration-time AsyncBinding unwrap — same
// pattern as azureBlob), so each binding's `.Func` must be unwrapped
// explicitly here.
func azureKeyvaultSecrets(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig, vaultURL string) map[string]any {
	bind := func(fn func(context.Context, azureConfig, string, azureKeyvaultSecretsArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, azureKeyvaultSecretsExtract,
			func(ctx context.Context, a azureKeyvaultSecretsArgs) (any, error) { return fn(ctx, cfg, vaultURL, a) }).Func
	}
	return map[string]any{
		"listSecrets":  bind(azureKeyvaultSecretsListSecrets),
		"getSecret":    bind(azureKeyvaultSecretsGetSecret),
		"setSecret":    bind(azureKeyvaultSecretsSetSecret),
		"deleteSecret": bind(azureKeyvaultSecretsDeleteSecret),
	}
}
