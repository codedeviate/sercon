package main

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v8"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// azureComputeArgs is the plain-Go carrier for every cloud.azure(...).compute()
// method: extracted on-loop by azureComputeExtract, consumed off-loop by the
// azureCompute<Xxx> work funcs. Named azureComputeArgs (not computeArgs) to
// avoid colliding with cloud_google_compute.go's computeArgs in this package.
type azureComputeArgs struct {
	resourceGroup string
	name          string
}

// newVirtualMachinesClient resolves the subscription + credential + ARM client
// options for cfg and builds an armcompute.VirtualMachinesClient. A missing
// subscription (no explicit subscriptionId and no AZURE_SUBSCRIPTION_ID) must
// reject, so cfg.subscription()'s error is propagated rather than swallowed.
func newVirtualMachinesClient(ctx context.Context, cfg azureConfig) (*armcompute.VirtualMachinesClient, error) {
	subscription, err := cfg.subscription()
	if err != nil {
		return nil, err
	}
	cred, err := cfg.credential()
	if err != nil {
		return nil, err
	}
	client, err := armcompute.NewVirtualMachinesClient(subscription, cred, cfg.armClientOptions())
	if err != nil {
		return nil, mapAzureError(err)
	}
	return client, nil
}

// azureComputeList lists virtual machines: scoped to a resource group when one
// is given, else subscription-wide via NewListAllPager.
func azureComputeList(ctx context.Context, cfg azureConfig, a azureComputeArgs) (any, error) {
	client, err := newVirtualMachinesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var all []*armcompute.VirtualMachine
	if a.resourceGroup != "" {
		pager := client.NewListPager(a.resourceGroup, nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, mapAzureError(err)
			}
			all = append(all, page.Value...)
		}
	} else {
		pager := client.NewListAllPager(nil)
		for pager.More() {
			page, err := pager.NextPage(ctx)
			if err != nil {
				return nil, mapAzureError(err)
			}
			all = append(all, page.Value...)
		}
	}
	plain, err := toPlain(all)
	if err != nil {
		return nil, err
	}
	return map[string]any{"value": plain}, nil
}

// azureComputeGet fetches a single virtual machine by resource group + name.
func azureComputeGet(ctx context.Context, cfg azureConfig, a azureComputeArgs) (any, error) {
	client, err := newVirtualMachinesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return toPlain(resp.VirtualMachine)
}

// azureComputeStart starts a virtual machine. BeginStart starts the
// long-running operation; PollUntilDone blocks until it completes (the mock
// server completes it immediately with a 200/202).
func azureComputeStart(ctx context.Context, cfg azureConfig, a azureComputeArgs) (any, error) {
	client, err := newVirtualMachinesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	poller, err := client.BeginStart(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureComputePowerOff powers off a virtual machine (like BeginStart, a
// long-running operation polled to completion).
func azureComputePowerOff(ctx context.Context, cfg azureConfig, a azureComputeArgs) (any, error) {
	client, err := newVirtualMachinesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	poller, err := client.BeginPowerOff(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureComputeDeallocate deallocates a virtual machine (releases the compute
// resources while retaining disks/config).
func azureComputeDeallocate(ctx context.Context, cfg azureConfig, a azureComputeArgs) (any, error) {
	client, err := newVirtualMachinesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	poller, err := client.BeginDeallocate(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureComputeDelete deletes a virtual machine.
func azureComputeDelete(ctx context.Context, cfg azureConfig, a azureComputeArgs) (any, error) {
	client, err := newVirtualMachinesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	poller, err := client.BeginDelete(ctx, a.resourceGroup, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureComputeExtract runs on the event loop: read + validate JS args.
// resourceGroup is optional for list (empty ⇒ subscription-wide); name is
// required for get/start/powerOff/deallocate/delete but that requirement is
// left to the ARM API's own validation, mirroring azureRGExtract.
func azureComputeExtract(call goja.FunctionCall) (azureComputeArgs, error) {
	var a azureComputeArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.resourceGroup = optString(o, "resourceGroup", "")
	a.name = optString(o, "name", "")
	return a, nil
}

// azureCompute builds the object returned by cloud.azure(...).compute(): one
// PromisifyAsync binding per method, all sharing azureComputeExtract and cfg.
// This map is built at script-run time (inside the compute() accessor call in
// cloud_azure.go), past the engine's registration-time AsyncBinding unwrap —
// so each binding's `.Func` must be unwrapped explicitly here (same pattern as
// azureResourceGroups). Replaces the temporary stub previously defined in
// cloud_azure.go.
func azureCompute(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig) map[string]any {
	bind := func(fn func(context.Context, azureConfig, azureComputeArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, azureComputeExtract,
			func(ctx context.Context, a azureComputeArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listVirtualMachines": bind(azureComputeList),
		"getVirtualMachine":   bind(azureComputeGet),
		"start":               bind(azureComputeStart),
		"powerOff":            bind(azureComputePowerOff),
		"deallocate":          bind(azureComputeDeallocate),
		"delete":              bind(azureComputeDelete),
	}
}
