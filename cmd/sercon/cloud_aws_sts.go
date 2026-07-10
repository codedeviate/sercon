package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsSTSArgs is the plain-Go carrier for every cloud.aws(...).sts() method:
// extracted on-loop by awsSTSExtract, consumed off-loop by the awsSTSXxx work
// funcs. durationSeconds is 0 when not provided by the caller — the AWS
// minimum is 900, so 0 is a safe "omit" sentinel.
type awsSTSArgs struct {
	roleArn, roleSessionName string
	durationSeconds          int
}

// newSTSClient builds an sts.Client for cfg. Like IAM/EC2, STS's query/XML
// wire protocol addresses everything by action + BaseEndpoint — no
// UsePathStyle option applies here.
func newSTSClient(ctx context.Context, cfg awsConfig) (*sts.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return sts.NewFromConfig(base, func(o *sts.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

// awsSTSGetCallerIdentity returns the identity of the credentials in use.
// The response contains no secrets (Account/Arn/UserId only).
func awsSTSGetCallerIdentity(ctx context.Context, cfg awsConfig, a awsSTSArgs) (any, error) {
	c, err := newSTSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsSTSAssumeRole returns temporary credentials for the assumed role. The
// output (AccessKeyId/SecretAccessKey/SessionToken) is sensitive: it is
// returned to the caller via toPlain by design, but must never be logged
// here.
func awsSTSAssumeRole(ctx context.Context, cfg awsConfig, a awsSTSArgs) (any, error) {
	c, err := newSTSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &sts.AssumeRoleInput{
		RoleArn:         aws.String(a.roleArn),
		RoleSessionName: aws.String(a.roleSessionName),
	}
	if a.durationSeconds > 0 {
		in.DurationSeconds = aws.Int32(int32(a.durationSeconds))
	}
	out, err := c.AssumeRole(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsSTSGetSessionToken returns temporary credentials for the current
// principal. Same sensitivity note as awsSTSAssumeRole applies.
func awsSTSGetSessionToken(ctx context.Context, cfg awsConfig, a awsSTSArgs) (any, error) {
	c, err := newSTSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &sts.GetSessionTokenInput{}
	if a.durationSeconds > 0 {
		in.DurationSeconds = aws.Int32(int32(a.durationSeconds))
	}
	out, err := c.GetSessionToken(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsSTSExtract reads the single options object on the event loop.
func awsSTSExtract(call goja.FunctionCall) (awsSTSArgs, error) {
	var a awsSTSArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.roleArn = optString(o, "roleArn", "")
	a.roleSessionName = optString(o, "roleSessionName", "")
	a.durationSeconds = optInt(o, "durationSeconds", 0)
	return a, nil
}

// awsSTS builds the object returned by cloud.aws(...).sts(): one
// PromisifyAsync binding per method, all sharing awsSTSExtract and cfg.
//
// This map is built at script-run time (inside the sts() accessor call in
// cloud_aws.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// awsS3/awsEC2/awsIAM).
func awsSTS(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsSTSArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsSTSExtract,
			func(ctx context.Context, a awsSTSArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"getCallerIdentity": bind(awsSTSGetCallerIdentity),
		"assumeRole":        bind(awsSTSAssumeRole),
		"getSessionToken":   bind(awsSTSGetSessionToken),
	}
}
