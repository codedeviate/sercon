package main

import (
	"context"
	"encoding/json"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"google.golang.org/api/compute/v1"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// computeArgs is the plain-Go carrier for every cloud.google(...).compute()
// method: extracted on-loop by computeExtract, consumed off-loop by the
// computeXxx functions.
type computeArgs struct {
	project, zone, name string
	instance            map[string]any
}

// newComputeService builds a compute/v1 client for cfg. googleTestOptions
// (set only by tests via withMockGoogle) is appended last so it can override
// auth/endpoint/http-client for httptest servers.
func newComputeService(ctx context.Context, cfg googleConfig) (*compute.Service, error) {
	svc, err := compute.NewService(ctx, cfg.clientOptions(googleTestOptions...)...)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return svc, nil
}

func computeListInstances(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Instances.List(a.project, a.zone).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeGetInstance(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Instances.Get(a.project, a.zone, a.name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeCreateInstance(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(a.instance)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	inst := &compute.Instance{}
	if err := json.Unmarshal(b, inst); err != nil {
		return nil, mapGoogleError(err)
	}
	res, err := svc.Instances.Insert(a.project, a.zone, inst).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeDeleteInstance(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Instances.Delete(a.project, a.zone, a.name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeStartInstance(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Instances.Start(a.project, a.zone, a.name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeStopInstance(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Instances.Stop(a.project, a.zone, a.name).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeListZones(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Zones.List(a.project).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func computeListDisks(ctx context.Context, cfg googleConfig, a computeArgs) (any, error) {
	svc, err := newComputeService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Disks.List(a.project, a.zone).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

// computeExtract reads the single options object on the event loop.
func computeExtract(call goja.FunctionCall) (computeArgs, error) {
	a := computeArgs{}
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.project = optString(o, "project", "")
	a.zone = optString(o, "zone", "")
	a.name = optString(o, "name", "")
	if raw, present := o["instance"]; present && raw != nil {
		if m, ok := raw.(map[string]any); ok {
			a.instance = m
		}
	}
	return a, nil
}

// googleCompute builds the object returned by cloud.google(...).compute():
// one PromisifyAsync binding per method, all sharing computeExtract and cfg.
//
// This map is built at script-run time (inside the compute() accessor call in
// cloud.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// googleStorage in cloud_google_storage.go).
func googleCompute(vm *goja.Runtime, loop *eventloop.EventLoop, cfg googleConfig) map[string]any {
	bind := func(fn func(context.Context, googleConfig, computeArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, computeExtract,
			func(ctx context.Context, a computeArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listInstances":  bind(computeListInstances),
		"getInstance":    bind(computeGetInstance),
		"createInstance": bind(computeCreateInstance),
		"deleteInstance": bind(computeDeleteInstance),
		"startInstance":  bind(computeStartInstance),
		"stopInstance":   bind(computeStopInstance),
		"listZones":      bind(computeListZones),
		"listDisks":      bind(computeListDisks),
	}
}
