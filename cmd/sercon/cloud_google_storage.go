package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"google.golang.org/api/storage/v1"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// gcsArgs is the plain-Go carrier for every cloud.google(...).storage()
// method: extracted on-loop by storageExtract, consumed off-loop by the
// storageXxx functions.
type gcsArgs struct {
	project, bucket, key, prefix string
	body                         []byte
}

// newStorageService builds a storage/v1 client for cfg. googleTestOptions
// (set only by tests via withMockGoogle) is appended last so it can override
// auth/endpoint/http-client for httptest servers.
func newStorageService(ctx context.Context, cfg googleConfig) (*storage.Service, error) {
	svc, err := storage.NewService(ctx, cfg.clientOptions(googleTestOptions...)...)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return svc, nil
}

// toPlain round-trips an SDK struct through JSON into a plain map/slice/etc so
// goja emits a clean object (SDK structs carry unexported/server-only fields
// and custom marshalling we don't want leaking into the JS-facing shape).
func toPlain(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, mapGoogleError(err)
	}
	return out, nil
}

func storageListBuckets(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Buckets.List(a.project).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func storageGetBucket(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Buckets.Get(a.bucket).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func storageCreateBucket(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Buckets.Insert(a.project, &storage.Bucket{Name: a.bucket}).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func storageDeleteBucket(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := svc.Buckets.Delete(a.bucket).Context(ctx).Do(); err != nil {
		return nil, mapGoogleError(err)
	}
	return map[string]any{}, nil
}

func storageListObjects(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Objects.List(a.bucket).Prefix(a.prefix).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func storageStatObject(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Objects.Get(a.bucket, a.key).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func storageReadObject(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	resp, err := svc.Objects.Get(a.bucket, a.key).Context(ctx).Download()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return map[string]any{"bytes": raw}, nil
}

func storagePutObject(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	res, err := svc.Objects.Insert(a.bucket, &storage.Object{Name: a.key}).
		Media(bytes.NewReader(a.body)).Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return toPlain(res)
}

func storageDeleteObject(ctx context.Context, cfg googleConfig, a gcsArgs) (any, error) {
	svc, err := newStorageService(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := svc.Objects.Delete(a.bucket, a.key).Context(ctx).Do(); err != nil {
		return nil, mapGoogleError(err)
	}
	return map[string]any{}, nil
}

// storageExtract reads the single options object on the event loop. body
// accepts a string (UTF-8 bytes) or Uint8Array/ArrayBuffer, via the same
// coercion http.go's multipart path uses (bytesFromExported).
func storageExtract(call goja.FunctionCall) (gcsArgs, error) {
	a := gcsArgs{}
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.project = optString(o, "project", "")
	a.bucket = optString(o, "bucket", "")
	a.key = optString(o, "key", "")
	a.prefix = optString(o, "prefix", "")
	if raw, present := o["body"]; present && raw != nil {
		b, err := bytesFromExported(raw)
		if err != nil {
			return a, err
		}
		a.body = b
	}
	return a, nil
}

// googleStorage builds the object returned by cloud.google(...).storage():
// one PromisifyAsync binding per method, all sharing storageExtract and cfg.
//
// This map is built at script-run time (inside the storage() accessor call in
// cloud.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// googleHandle's `call` in cloud.go and sqlHandle in db_sql.go).
func googleStorage(vm *goja.Runtime, loop *eventloop.EventLoop, cfg googleConfig) map[string]any {
	bind := func(fn func(context.Context, googleConfig, gcsArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, storageExtract,
			func(ctx context.Context, a gcsArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listBuckets":  bind(storageListBuckets),
		"getBucket":    bind(storageGetBucket),
		"createBucket": bind(storageCreateBucket),
		"deleteBucket": bind(storageDeleteBucket),
		"listObjects":  bind(storageListObjects),
		"statObject":   bind(storageStatObject),
		"readObject":   bind(storageReadObject),
		"putObject":    bind(storagePutObject),
		"deleteObject": bind(storageDeleteObject),
	}
}
