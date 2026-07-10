package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsSecretsManagerArgs is the plain-Go carrier for every
// cloud.aws(...).secretsmanager() method: extracted on-loop by
// awsSecretsManagerExtract, consumed off-loop by the awsSecretsManagerXxx work
// funcs.
type awsSecretsManagerArgs struct {
	secretId, name, secretString string
}

// newSecretsManagerClient builds a secretsmanager.Client for cfg. Secrets
// Manager's wire protocol is JSON (awsjson1.1), addressed by BaseEndpoint
// alone — like EC2/IAM, no UsePathStyle option applies here.
func newSecretsManagerClient(ctx context.Context, cfg awsConfig) (*secretsmanager.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return secretsmanager.NewFromConfig(base, func(o *secretsmanager.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsSecretsManagerListSecrets(ctx context.Context, cfg awsConfig, a awsSecretsManagerArgs) (any, error) {
	c, err := newSecretsManagerClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSecretsManagerDescribeSecret(ctx context.Context, cfg awsConfig, a awsSecretsManagerArgs) (any, error) {
	c, err := newSecretsManagerClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(a.secretId)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSecretsManagerCreateSecret(ctx context.Context, cfg awsConfig, a awsSecretsManagerArgs) (any, error) {
	c, err := newSecretsManagerClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &secretsmanager.CreateSecretInput{Name: aws.String(a.name)}
	if a.secretString != "" {
		in.SecretString = aws.String(a.secretString)
	}
	out, err := c.CreateSecret(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsSecretsManagerGetSecretValue returns the decoded secret value as
// { value: string }, never the raw SDK struct — the raw struct would also
// carry ARN/VersionId metadata, but callers of this binding only want the
// value. SecretBinary in aws-sdk-go-v2 is already raw decoded bytes (the SDK
// base64-decodes the awsjson1.1 blob on the wire), so it is converted to a
// string as-is — no further base64 decoding.
func awsSecretsManagerGetSecretValue(ctx context.Context, cfg awsConfig, a awsSecretsManagerArgs) (any, error) {
	c, err := newSecretsManagerClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(a.secretId)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	var value string
	switch {
	case out.SecretString != nil:
		value = *out.SecretString
	case out.SecretBinary != nil:
		value = string(out.SecretBinary)
	}
	return map[string]any{"value": value}, nil
}

func awsSecretsManagerPutSecretValue(ctx context.Context, cfg awsConfig, a awsSecretsManagerArgs) (any, error) {
	c, err := newSecretsManagerClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(a.secretId),
		SecretString: aws.String(a.secretString),
	})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSecretsManagerDeleteSecret(ctx context.Context, cfg awsConfig, a awsSecretsManagerArgs) (any, error) {
	c, err := newSecretsManagerClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String(a.secretId)}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

// awsSecretsManagerExtract reads the single options object on the event loop.
func awsSecretsManagerExtract(call goja.FunctionCall) (awsSecretsManagerArgs, error) {
	var a awsSecretsManagerArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.secretId = optString(o, "secretId", "")
	a.name = optString(o, "name", "")
	a.secretString = optString(o, "secretString", "")
	return a, nil
}

// awsSecretsManager builds the object returned by
// cloud.aws(...).secretsmanager(): one PromisifyAsync binding per method, all
// sharing awsSecretsManagerExtract and cfg.
//
// This map is built at script-run time (inside the secretsmanager() accessor
// call in cloud_aws.go), past the engine's registration-time AsyncBinding
// unwrap — so each binding's `.Func` must be unwrapped explicitly here (same
// pattern as awsS3/awsEC2/awsIAM).
func awsSecretsManager(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsSecretsManagerArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsSecretsManagerExtract,
			func(ctx context.Context, a awsSecretsManagerArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listSecrets":    bind(awsSecretsManagerListSecrets),
		"describeSecret": bind(awsSecretsManagerDescribeSecret),
		"createSecret":   bind(awsSecretsManagerCreateSecret),
		"getSecretValue": bind(awsSecretsManagerGetSecretValue),
		"putSecretValue": bind(awsSecretsManagerPutSecretValue),
		"deleteSecret":   bind(awsSecretsManagerDeleteSecret),
	}
}
