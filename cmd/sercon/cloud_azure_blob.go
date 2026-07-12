package main

import (
	"context"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/service"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// azureBlobArgs is the plain-Go carrier for every cloud.azure(...).blob(url)
// method: extracted on-loop by azureBlobExtract, consumed off-loop by the
// azureBlobXxx work funcs. This is the DATA-PLANE pattern Task 8
// (keyvaultSecrets) copies: unlike the ARM services (Tasks 4-6), the
// accountURL is threaded through explicitly rather than resolved from cfg's
// subscription — data-plane services have no subscription/ARM host of their
// own, the caller-supplied endpoint URL *is* the service.
type azureBlobArgs struct {
	container string
	blob      string
	body      []byte
}

// newBlobClient builds an azblob.Client from the caller-supplied accountURL.
// No subscription is involved (data-plane, not ARM) and no Cloud/endpoint
// override is needed either: the accountURL passed in *is* the endpoint, so
// in tests it already points at the mock server directly. Only the transport
// (and, when the seam is active, permission to attach a bearer token to a
// plain-HTTP request) come from cfg.coreClientOptions().
func newBlobClient(cfg azureConfig, accountURL string) (*azblob.Client, error) {
	cred, err := cfg.credential()
	if err != nil {
		return nil, err
	}
	client, err := azblob.NewClient(accountURL, cred, &azblob.ClientOptions{ClientOptions: cfg.coreClientOptions()})
	if err != nil {
		return nil, mapAzureError(err)
	}
	return client, nil
}

// azureBlobListContainers lists every container in the storage account,
// paging through every page the SDK's pager returns.
func azureBlobListContainers(ctx context.Context, cfg azureConfig, accountURL string, a azureBlobArgs) (any, error) {
	client, err := newBlobClient(cfg, accountURL)
	if err != nil {
		return nil, err
	}
	var all []*service.ContainerItem
	pager := client.NewListContainersPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, mapAzureError(err)
		}
		all = append(all, page.ContainerItems...)
	}
	plain, err := toPlain(all)
	if err != nil {
		return nil, err
	}
	return map[string]any{"value": plain}, nil
}

// azureBlobListBlobs lists every blob in a.container (flat listing, no
// hierarchy/delimiter), paging through every page the SDK's pager returns.
func azureBlobListBlobs(ctx context.Context, cfg azureConfig, accountURL string, a azureBlobArgs) (any, error) {
	client, err := newBlobClient(cfg, accountURL)
	if err != nil {
		return nil, err
	}
	var all []*container.BlobItem
	pager := client.NewListBlobsFlatPager(a.container, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, mapAzureError(err)
		}
		if page.Segment != nil {
			all = append(all, page.Segment.BlobItems...)
		}
	}
	plain, err := toPlain(all)
	if err != nil {
		return nil, err
	}
	return map[string]any{"value": plain}, nil
}

// azureBlobDownload downloads a.blob from a.container and reads the entire
// body into memory, returning it as raw bytes (goja renders a []byte as a
// Uint8Array).
func azureBlobDownload(ctx context.Context, cfg azureConfig, accountURL string, a azureBlobArgs) (any, error) {
	client, err := newBlobClient(cfg, accountURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.DownloadStream(ctx, a.container, a.blob, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{"bytes": raw}, nil
}

// azureBlobUpload uploads a.body as a.blob in a.container in a single
// request (block blob "Put Blob"); a.body is populated by
// azureBlobExtract via bytesFromExported.
func azureBlobUpload(ctx context.Context, cfg azureConfig, accountURL string, a azureBlobArgs) (any, error) {
	client, err := newBlobClient(cfg, accountURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.UploadBuffer(ctx, a.container, a.blob, a.body, nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return toPlain(resp)
}

// azureBlobDeleteBlob deletes a.blob from a.container.
func azureBlobDeleteBlob(ctx context.Context, cfg azureConfig, accountURL string, a azureBlobArgs) (any, error) {
	client, err := newBlobClient(cfg, accountURL)
	if err != nil {
		return nil, err
	}
	if _, err := client.DeleteBlob(ctx, a.container, a.blob, nil); err != nil {
		return nil, mapAzureError(err)
	}
	return map[string]any{}, nil
}

// azureBlobExtract runs on the event loop: read + validate JS args. body
// accepts a string (UTF-8 bytes) or Uint8Array/ArrayBuffer, via the same
// coercion cloud_google_storage.go's storageExtract uses (bytesFromExported).
func azureBlobExtract(call goja.FunctionCall) (azureBlobArgs, error) {
	var a azureBlobArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.container = optString(o, "container", "")
	a.blob = optString(o, "blob", "")
	if raw, present := o["body"]; present && raw != nil {
		b, err := bytesFromExported(raw)
		if err != nil {
			return a, err
		}
		a.body = b
	}
	return a, nil
}

// azureBlob builds the object returned by cloud.azure(...).blob(accountURL):
// one PromisifyAsync binding per method, all sharing azureBlobExtract, cfg,
// and the accountURL supplied by the caller. This map is built at
// script-run-time call of the `blob` accessor in cloud_azure.go (which reads
// the URL argument on-loop, past the engine's registration-time
// AsyncBinding unwrap — same pattern as azureResourceGroups/azureCompute),
// so each binding's `.Func` must be unwrapped explicitly here.
//
// Sets the data-plane pattern Task 8 (keyvaultSecrets) copies: the accessor
// takes an endpoint URL argument and the client is built from it, rather
// than from cfg's subscription (there is none for data-plane services).
func azureBlob(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig, accountURL string) map[string]any {
	bind := func(fn func(context.Context, azureConfig, string, azureBlobArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, azureBlobExtract,
			func(ctx context.Context, a azureBlobArgs) (any, error) { return fn(ctx, cfg, accountURL, a) }).Func
	}
	return map[string]any{
		"listContainers": bind(azureBlobListContainers),
		"listBlobs":      bind(azureBlobListBlobs),
		"download":       bind(azureBlobDownload),
		"upload":         bind(azureBlobUpload),
		"deleteBlob":     bind(azureBlobDeleteBlob),
	}
}
