package main

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// azureResourcesArgs is the plain-Go carrier for every
// cloud.azure(...).resources() method: extracted on-loop by
// azureResourcesExtract, consumed off-loop by the azureResourcesXxx work
// funcs. This is the ARM-service pattern Tasks 4-6 share.
type azureResourcesArgs struct {
	resourceGroup string
	resourceId    string
	apiVersion    string
}

// newResourcesClient resolves the subscription + credential + ARM client
// options for cfg and builds an armresources.Client (the generic resources
// client — distinct from armresources.ResourceGroupsClient built by Task 4's
// newResourceGroupsClient). A missing subscription (no explicit
// subscriptionId and no AZURE_SUBSCRIPTION_ID) must reject, so
// cfg.subscription()'s error is propagated rather than swallowed.
func newResourcesClient(ctx context.Context, cfg azureConfig) (*armresources.Client, error) {
	subscription, err := cfg.subscription()
	if err != nil {
		return nil, err
	}
	cred, err := cfg.credential()
	if err != nil {
		return nil, err
	}
	client, err := armresources.NewClient(subscription, cred, cfg.armClientOptions())
	if err != nil {
		return nil, mapAzureError(err)
	}
	return client, nil
}

// azureResourcesListByResourceGroup lists all resources in a resource group,
// paging through every page the SDK's pager returns.
func azureResourcesListByResourceGroup(ctx context.Context, cfg azureConfig, a azureResourcesArgs) (any, error) {
	client, err := newResourcesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var all []*armresources.GenericResourceExpanded
	pager := client.NewListByResourceGroupPager(a.resourceGroup, nil)
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

// azureResourcesGetById fetches a single resource by its fully qualified
// resource ID and an explicit api-version (the generic resources API has no
// per-resource-type default, so the caller must supply one).
func azureResourcesGetById(ctx context.Context, cfg azureConfig, a azureResourcesArgs) (any, error) {
	client, err := newResourcesClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetByID(ctx, a.resourceId, a.apiVersion, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return toPlain(resp.GenericResource)
}

// azureResourcesExtract runs on the event loop: read + validate JS args.
// resourceGroup is required for listByResourceGroup; resourceId/apiVersion
// are required for getById. Field-level validation is left to the ARM API's
// own errors, mirroring azureRGExtract/azureComputeExtract.
func azureResourcesExtract(call goja.FunctionCall) (azureResourcesArgs, error) {
	var a azureResourcesArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.resourceGroup = optString(o, "resourceGroup", "")
	a.resourceId = optString(o, "resourceId", "")
	a.apiVersion = optString(o, "apiVersion", "")
	return a, nil
}

// azureResources builds the object returned by cloud.azure(...).resources():
// one PromisifyAsync binding per method, all sharing azureResourcesExtract
// and cfg. This map is built at script-run time (inside the resources()
// accessor call in cloud_azure.go), past the engine's registration-time
// AsyncBinding unwrap — so each binding's `.Func` must be unwrapped
// explicitly here (same pattern as azureResourceGroups/azureCompute).
// Replaces the temporary stub previously defined in cloud_azure.go.
func azureResources(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig) map[string]any {
	bind := func(fn func(context.Context, azureConfig, azureResourcesArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, azureResourcesExtract,
			func(ctx context.Context, a azureResourcesArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listByResourceGroup": bind(azureResourcesListByResourceGroup),
		"getById":             bind(azureResourcesGetById),
	}
}
