package main

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// azureRGArgs is the plain-Go carrier for every cloud.azure(...).resourceGroups()
// method: extracted on-loop by azureRGExtract, consumed off-loop by the
// azureRGXxx work funcs. This is the ARM-service pattern Tasks 5-6 copy.
type azureRGArgs struct {
	name     string
	location string
}

// newResourceGroupsClient resolves the subscription + credential + ARM client
// options for cfg and builds an armresources.ResourceGroupsClient. A missing
// subscription (no explicit subscriptionId and no AZURE_SUBSCRIPTION_ID) must
// reject, so cfg.subscription()'s error is propagated rather than swallowed.
func newResourceGroupsClient(ctx context.Context, cfg azureConfig) (*armresources.ResourceGroupsClient, error) {
	subscription, err := cfg.subscription()
	if err != nil {
		return nil, err
	}
	cred, err := cfg.credential()
	if err != nil {
		return nil, err
	}
	client, err := armresources.NewResourceGroupsClient(subscription, cred, cfg.armClientOptions())
	if err != nil {
		return nil, mapAzureError(err)
	}
	return client, nil
}

// azureRGList lists all resource groups in the subscription, paging through
// every page the SDK's pager returns.
func azureRGList(ctx context.Context, cfg azureConfig, a azureRGArgs) (any, error) {
	client, err := newResourceGroupsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var all []*armresources.ResourceGroup
	pager := client.NewListPager(nil)
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

// azureRGGet fetches a single resource group by name.
func azureRGGet(ctx context.Context, cfg azureConfig, a azureRGArgs) (any, error) {
	client, err := newResourceGroupsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(ctx, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return toPlain(resp.ResourceGroup)
}

// azureRGCreate creates or updates a resource group at the given location.
func azureRGCreate(ctx context.Context, cfg azureConfig, a azureRGArgs) (any, error) {
	client, err := newResourceGroupsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := client.CreateOrUpdate(ctx, a.name, armresources.ResourceGroup{Location: &a.location}, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return toPlain(resp.ResourceGroup)
}

// azureRGDelete deletes a resource group. BeginDelete starts the long-running
// operation; PollUntilDone blocks until it completes (the mock server
// completes it immediately with a 200/202).
func azureRGDelete(ctx context.Context, cfg azureConfig, a azureRGArgs) (any, error) {
	client, err := newResourceGroupsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	poller, err := client.BeginDelete(ctx, a.name, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	if _, err := poller.PollUntilDone(ctx, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureRGExtract runs on the event loop: read + validate JS args. name is
// required for get/create/delete; list ignores the options object entirely.
func azureRGExtract(call goja.FunctionCall) (azureRGArgs, error) {
	var a azureRGArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.name = optString(o, "name", "")
	a.location = optString(o, "location", "")
	return a, nil
}

// azureResourceGroups builds the object returned by
// cloud.azure(...).resourceGroups(): one PromisifyAsync binding per method,
// all sharing azureRGExtract and cfg. This map is built at script-run time
// (inside the resourceGroups() accessor call in cloud_azure.go), past the
// engine's registration-time AsyncBinding unwrap — so each binding's `.Func`
// must be unwrapped explicitly here (same pattern as awsS3/googleStorage).
func azureResourceGroups(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig) map[string]any {
	bind := func(fn func(context.Context, azureConfig, azureRGArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, azureRGExtract,
			func(ctx context.Context, a azureRGArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"list":   bind(azureRGList),
		"get":    bind(azureRGGet),
		"create": bind(azureRGCreate),
		"delete": bind(azureRGDelete),
	}
}
