package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsLambdaArgs is the plain-Go carrier for every cloud.aws(...).lambda()
// method: extracted on-loop by awsLambdaExtract, consumed off-loop by the
// awsLambdaXxx work funcs.
type awsLambdaArgs struct {
	functionName           string
	payload                any // raw exported value: string, map[string]any, or nil
	role, runtime, handler string
	s3Bucket, s3Key        string
	zipFile                []byte
}

// newLambdaClient builds a lambda.Client for cfg. Lambda's wire protocol is
// REST-JSON (restjson1), addressed by BaseEndpoint alone — like
// SecretsManager/EC2/IAM, no UsePathStyle option applies here.
func newLambdaClient(ctx context.Context, cfg awsConfig) (*lambda.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return lambda.NewFromConfig(base, func(o *lambda.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsLambdaListFunctions(ctx context.Context, cfg awsConfig, a awsLambdaArgs) (any, error) {
	c, err := newLambdaClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.ListFunctions(ctx, &lambda.ListFunctionsInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsLambdaGetFunction(ctx context.Context, cfg awsConfig, a awsLambdaArgs) (any, error) {
	c, err := newLambdaClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.GetFunction(ctx, &lambda.GetFunctionInput{FunctionName: aws.String(a.functionName)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsLambdaInvoke hand-builds its result rather than returning toPlain(out):
// out.Payload is raw []byte (the invoked function's JSON response), and
// toPlain would JSON-round-trip it into a numeric byte array instead of the
// string callers expect.
func awsLambdaInvoke(ctx context.Context, cfg awsConfig, a awsLambdaArgs) (any, error) {
	c, err := newLambdaClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var payload []byte
	switch p := a.payload.(type) {
	case nil:
		// no payload
	case string:
		payload = []byte(p)
	default:
		b, err := json.Marshal(p)
		if err != nil {
			return nil, mapAWSError(err)
		}
		payload = b
	}
	in := &lambda.InvokeInput{FunctionName: aws.String(a.functionName)}
	if payload != nil {
		in.Payload = payload
	}
	out, err := c.Invoke(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	res := map[string]any{
		"statusCode": out.StatusCode,
		"payload":    string(out.Payload),
	}
	if out.FunctionError != nil {
		res["functionError"] = *out.FunctionError
	}
	if out.ExecutedVersion != nil {
		res["executedVersion"] = *out.ExecutedVersion
	}
	return res, nil
}

// awsLambdaCreateFunction hand-maps the key CreateFunctionInput fields
// (rather than JSON-unmarshalling opts directly into the SDK struct): the
// input has a nested pointer struct (Code) and an enum (Runtime), neither of
// which round-trips cleanly through encoding/json into SDK types. Only the
// fields the caller provides are set.
func awsLambdaCreateFunction(ctx context.Context, cfg awsConfig, a awsLambdaArgs) (any, error) {
	c, err := newLambdaClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &lambda.CreateFunctionInput{
		FunctionName: aws.String(a.functionName),
		Role:         aws.String(a.role),
		Runtime:      types.Runtime(a.runtime),
		Handler:      aws.String(a.handler),
		Code:         &types.FunctionCode{},
	}
	if len(a.zipFile) > 0 {
		in.Code.ZipFile = a.zipFile
	}
	if a.s3Bucket != "" {
		in.Code.S3Bucket = aws.String(a.s3Bucket)
	}
	if a.s3Key != "" {
		in.Code.S3Key = aws.String(a.s3Key)
	}
	out, err := c.CreateFunction(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsLambdaDeleteFunction(ctx context.Context, cfg awsConfig, a awsLambdaArgs) (any, error) {
	c, err := newLambdaClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteFunction(ctx, &lambda.DeleteFunctionInput{FunctionName: aws.String(a.functionName)}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

// awsLambdaExtract reads the single options object on the event loop.
func awsLambdaExtract(call goja.FunctionCall) (awsLambdaArgs, error) {
	var a awsLambdaArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.functionName = optString(o, "functionName", "")
	if raw, present := o["payload"]; present && raw != nil {
		a.payload = raw
	}
	a.role = optString(o, "role", "")
	a.runtime = optString(o, "runtime", "")
	a.handler = optString(o, "handler", "")
	a.s3Bucket = optString(o, "s3Bucket", "")
	a.s3Key = optString(o, "s3Key", "")
	if raw, present := o["zipFile"]; present && raw != nil {
		b, err := bytesFromExported(raw)
		if err != nil {
			return a, err
		}
		a.zipFile = b
	}
	return a, nil
}

// awsLambda builds the object returned by cloud.aws(...).lambda(): one
// PromisifyAsync binding per method, all sharing awsLambdaExtract and cfg.
//
// This map is built at script-run time (inside the lambda() accessor call in
// cloud_aws.go), past the engine's registration-time AsyncBinding unwrap —
// so each binding's `.Func` must be unwrapped explicitly here (same pattern
// as awsS3/awsSecretsManager/awsEC2/awsIAM).
func awsLambda(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsLambdaArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsLambdaExtract,
			func(ctx context.Context, a awsLambdaArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listFunctions":  bind(awsLambdaListFunctions),
		"getFunction":    bind(awsLambdaGetFunction),
		"invoke":         bind(awsLambdaInvoke),
		"createFunction": bind(awsLambdaCreateFunction),
		"deleteFunction": bind(awsLambdaDeleteFunction),
	}
}
