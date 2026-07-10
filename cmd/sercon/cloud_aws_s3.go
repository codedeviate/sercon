package main

import (
	"bytes"
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsS3Args is the plain-Go carrier for every cloud.aws(...).s3() method:
// extracted on-loop by awsS3Extract, consumed off-loop by the awsS3Xxx work
// funcs.
type awsS3Args struct {
	bucket, key, prefix string
	body                []byte
}

// newS3Client builds an s3.Client for cfg. The test seam (awsBaseEndpoint,
// set only via withMockAWS) overrides the endpoint and forces path-style
// addressing — required against an httptest server, since S3's default
// virtual-hosted-style addressing needs a real DNS-resolvable bucket
// subdomain. S3 is the only AWS service that needs UsePathStyle; the other 8
// services (Tasks 4-11) only set BaseEndpoint.
func newS3Client(ctx context.Context, cfg awsConfig) (*s3.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(base, func(o *s3.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
			o.UsePathStyle = true // required for httptest / non-virtual-host endpoints
		}
	}), nil
}

func awsS3ListBuckets(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsS3CreateBucket(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(a.bucket)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsS3DeleteBucket(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(a.bucket)}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

func awsS3ListObjects(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &s3.ListObjectsV2Input{Bucket: aws.String(a.bucket)}
	if a.prefix != "" {
		in.Prefix = aws.String(a.prefix)
	}
	out, err := c.ListObjectsV2(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsS3HeadObject(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(a.key)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsS3GetObject(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(a.key)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	defer out.Body.Close()
	raw, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{"bytes": raw}, nil
}

func awsS3PutObject(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(a.key),
		Body:   bytes.NewReader(a.body),
	})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsS3DeleteObject(ctx context.Context, cfg awsConfig, a awsS3Args) (any, error) {
	c, err := newS3Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(a.key)}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

// awsS3Extract reads the single options object on the event loop. body
// accepts a string (UTF-8 bytes) or Uint8Array/ArrayBuffer, via the same
// coercion googleStorage's storageExtract uses (bytesFromExported).
func awsS3Extract(call goja.FunctionCall) (awsS3Args, error) {
	var a awsS3Args
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
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

// awsS3 builds the object returned by cloud.aws(...).s3(): one PromisifyAsync
// binding per method, all sharing awsS3Extract and cfg.
//
// This map is built at script-run time (inside the s3() accessor call in
// cloud_aws.go), past the engine's registration-time AsyncBinding unwrap —
// so each binding's `.Func` must be unwrapped explicitly here (same pattern
// as googleStorage and sqlHandle).
func awsS3(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsS3Args) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsS3Extract,
			func(ctx context.Context, a awsS3Args) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listBuckets":  bind(awsS3ListBuckets),
		"createBucket": bind(awsS3CreateBucket),
		"deleteBucket": bind(awsS3DeleteBucket),
		"listObjects":  bind(awsS3ListObjects),
		"headObject":   bind(awsS3HeadObject),
		"getObject":    bind(awsS3GetObject),
		"putObject":    bind(awsS3PutObject),
		"deleteObject": bind(awsS3DeleteObject),
	}
}
