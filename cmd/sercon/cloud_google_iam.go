package main

import (
	"context"
	"encoding/json"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"google.golang.org/api/iam/v1"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// iamArgs is the plain-Go carrier for every cloud.google(...).iam() method:
// extracted on-loop by iamExtract, consumed off-loop by the iamXxx functions.
type iamArgs struct {
	project, email, accountId, displayName, resource string
	policy                                           map[string]any
}

// newIAMService builds an iam/v1 client for cfg. googleTestOptions (set only
// by tests via withMockGoogle) is appended last so it can override
// auth/endpoint/http-client for httptest servers.
func newIAMService(ctx context.Context, cfg googleConfig) (*iam.Service, error) {
	svc, err := iam.NewService(ctx, cfg.clientOptions(googleTestOptions...)...)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return svc, nil
}

func iamListServiceAccounts(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Projects.ServiceAccounts.List("projects/" + a.project).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func iamGetServiceAccount(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := "projects/" + a.project + "/serviceAccounts/" + a.email
	res, err := svc.Projects.ServiceAccounts.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func iamCreateServiceAccount(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	req := &iam.CreateServiceAccountRequest{
		AccountId:      a.accountId,
		ServiceAccount: &iam.ServiceAccount{DisplayName: a.displayName},
	}
	res, err := svc.Projects.ServiceAccounts.Create("projects/"+a.project, req).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func iamDeleteServiceAccount(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := "projects/" + a.project + "/serviceAccounts/" + a.email
	if _, err := svc.Projects.ServiceAccounts.Delete(name).Context(ctx).Do(); err != nil {
		return nil, mapGoogleError(err)
	}
	return map[string]any{}, nil
}

func iamListKeys(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := "projects/" + a.project + "/serviceAccounts/" + a.email
	res, err := svc.Projects.ServiceAccounts.Keys.List(name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func iamCreateKey(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	name := "projects/" + a.project + "/serviceAccounts/" + a.email
	res, err := svc.Projects.ServiceAccounts.Keys.Create(name, &iam.CreateServiceAccountKeyRequest{}).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func iamGetIamPolicy(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Projects.ServiceAccounts.GetIamPolicy(a.resource).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func iamSetIamPolicy(ctx context.Context, cfg googleConfig, a iamArgs) (any, error) {
	svc, err := newIAMService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(a.policy)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	policy := &iam.Policy{}
	if err := json.Unmarshal(b, policy); err != nil {
		return nil, mapGoogleError(err)
	}
	res, err := svc.Projects.ServiceAccounts.SetIamPolicy(a.resource, &iam.SetIamPolicyRequest{Policy: policy}).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

// iamExtract reads the single options object on the event loop.
func iamExtract(call goja.FunctionCall) (iamArgs, error) {
	a := iamArgs{}
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.project = optString(o, "project", "")
	a.email = optString(o, "email", "")
	a.accountId = optString(o, "accountId", "")
	a.displayName = optString(o, "displayName", "")
	a.resource = optString(o, "resource", "")
	if raw, present := o["policy"]; present && raw != nil {
		if m, ok := raw.(map[string]any); ok {
			a.policy = m
		}
	}
	return a, nil
}

// googleIAM builds the object returned by cloud.google(...).iam(): one
// PromisifyAsync binding per method, all sharing iamExtract and cfg.
//
// This map is built at script-run time (inside the iam() accessor call in
// cloud.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// googleStorage in cloud_google_storage.go and googleCompute in
// cloud_google_compute.go).
func googleIAM(vm *goja.Runtime, loop *eventloop.EventLoop, cfg googleConfig) map[string]any {
	bind := func(fn func(context.Context, googleConfig, iamArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, iamExtract,
			func(ctx context.Context, a iamArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listServiceAccounts":  bind(iamListServiceAccounts),
		"getServiceAccount":    bind(iamGetServiceAccount),
		"createServiceAccount": bind(iamCreateServiceAccount),
		"deleteServiceAccount": bind(iamDeleteServiceAccount),
		"listKeys":             bind(iamListKeys),
		"createKey":            bind(iamCreateKey),
		"getIamPolicy":         bind(iamGetIamPolicy),
		"setIamPolicy":         bind(iamSetIamPolicy),
	}
}
