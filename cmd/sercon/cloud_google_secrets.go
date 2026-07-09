package main

import (
	"context"
	"encoding/base64"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"google.golang.org/api/secretmanager/v1"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// secretsArgs is the plain-Go carrier for every cloud.google(...).secrets()
// method: extracted on-loop by secretsExtract, consumed off-loop by the
// secretsXxx functions.
type secretsArgs struct {
	project, name, version, payload string
}

// newSecretsService builds a secretmanager/v1 client for cfg. googleTestOptions
// (set only by tests via withMockGoogle) is appended last so it can override
// auth/endpoint/http-client for httptest servers.
func newSecretsService(ctx context.Context, cfg googleConfig) (*secretmanager.Service, error) {
	svc, err := secretmanager.NewService(ctx, cfg.clientOptions(googleTestOptions...)...)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return svc, nil
}

func secretsListSecrets(ctx context.Context, cfg googleConfig, a secretsArgs) (any, error) {
	svc, err := newSecretsService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Projects.Secrets.List("projects/" + a.project).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func secretsGetSecret(ctx context.Context, cfg googleConfig, a secretsArgs) (any, error) {
	svc, err := newSecretsService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := "projects/" + a.project + "/secrets/" + a.name
	res, err := svc.Projects.Secrets.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func secretsCreateSecret(ctx context.Context, cfg googleConfig, a secretsArgs) (any, error) {
	svc, err := newSecretsService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	secret := &secretmanager.Secret{
		Replication: &secretmanager.Replication{Automatic: &secretmanager.Automatic{}},
	}
	res, err := svc.Projects.Secrets.Create("projects/"+a.project, secret).SecretId(a.name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

// secretsAddSecretVersion base64-encodes a.payload before sending it — the
// Secret Manager API requires SecretPayload.Data to be base64.
func secretsAddSecretVersion(ctx context.Context, cfg googleConfig, a secretsArgs) (any, error) {
	svc, err := newSecretsService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	parent := "projects/" + a.project + "/secrets/" + a.name
	req := &secretmanager.AddSecretVersionRequest{
		Payload: &secretmanager.SecretPayload{Data: base64.StdEncoding.EncodeToString([]byte(a.payload))},
	}
	res, err := svc.Projects.Secrets.AddVersion(parent, req).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

// secretsAccessSecretVersion base64-decodes the response payload and returns
// the decoded plaintext — callers must never see the raw base64 wire format.
func secretsAccessSecretVersion(ctx context.Context, cfg googleConfig, a secretsArgs) (any, error) {
	svc, err := newSecretsService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	version := a.version
	if version == "" {
		version = "latest"
	}
	name := "projects/" + a.project + "/secrets/" + a.name + "/versions/" + version
	res, err := svc.Projects.Secrets.Versions.Access(name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	if res.Payload == nil {
		return map[string]any{"value": ""}, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(res.Payload.Data)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return map[string]any{"value": string(decoded)}, nil
}

func secretsDeleteSecret(ctx context.Context, cfg googleConfig, a secretsArgs) (any, error) {
	svc, err := newSecretsService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := "projects/" + a.project + "/secrets/" + a.name
	if _, err := svc.Projects.Secrets.Delete(name).Context(ctx).Do(); err != nil {
		return nil, mapGoogleError(err)
	}
	return map[string]any{}, nil
}

// secretsExtract reads the single options object on the event loop.
func secretsExtract(call goja.FunctionCall) (secretsArgs, error) {
	a := secretsArgs{}
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.project = optString(o, "project", "")
	a.name = optString(o, "name", "")
	a.version = optString(o, "version", "")
	a.payload = optString(o, "payload", "")
	return a, nil
}

// googleSecrets builds the object returned by cloud.google(...).secrets():
// one PromisifyAsync binding per method, all sharing secretsExtract and cfg.
//
// This map is built at script-run time (inside the secrets() accessor call in
// cloud.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// googleStorage in cloud_google_storage.go, googleCompute in
// cloud_google_compute.go, and googleIAM in cloud_google_iam.go).
func googleSecrets(vm *goja.Runtime, loop *eventloop.EventLoop, cfg googleConfig) map[string]any {
	bind := func(fn func(context.Context, googleConfig, secretsArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, secretsExtract,
			func(ctx context.Context, a secretsArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listSecrets":         bind(secretsListSecrets),
		"getSecret":           bind(secretsGetSecret),
		"createSecret":        bind(secretsCreateSecret),
		"addSecretVersion":    bind(secretsAddSecretVersion),
		"accessSecretVersion": bind(secretsAccessSecretVersion),
		"deleteSecret":        bind(secretsDeleteSecret),
	}
}
