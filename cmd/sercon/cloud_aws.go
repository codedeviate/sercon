package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// awsConfig is the resolved config for a cloud.aws(...) handle. Credential
// fields are NEVER logged. Empty fields mean "use the default chain".
type awsConfig struct {
	region, profile                            string
	accessKeyID, secretAccessKey, sessionToken string
}

// awsTestSeam, when set (tests only), routes every service client at an httptest
// server with static creds. nil in production.
type awsTestSeam struct {
	endpoint   string
	httpClient aws.HTTPClient
}

var awsTestOptions *awsTestSeam

// awsBaseEndpoint returns the per-service BaseEndpoint override (test-only), or
// nil in production. Every service's NewFromConfig applies it.
func awsBaseEndpoint() *string {
	if awsTestOptions != nil && awsTestOptions.endpoint != "" {
		return aws.String(awsTestOptions.endpoint)
	}
	return nil
}

func parseAWSConfig(vm *goja.Runtime, call goja.FunctionCall) (awsConfig, error) {
	var cfg awsConfig
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return cfg, nil
	}
	obj, ok := arg.(*goja.Object)
	if !ok {
		return cfg, errors.New("cloud.aws: options must be an object")
	}
	opts, ok := obj.Export().(map[string]any)
	if !ok {
		return cfg, errors.New("cloud.aws: options must be an object")
	}
	cfg.region = optString(opts, "region", "")
	cfg.profile = optString(opts, "profile", "")
	if raw, present := opts["credentials"]; present && raw != nil {
		cm, ok := raw.(map[string]any)
		if !ok {
			return cfg, errors.New("cloud.aws: credentials must be an object { accessKeyId, secretAccessKey, sessionToken? }")
		}
		cfg.accessKeyID = optString(cm, "accessKeyId", "")
		cfg.secretAccessKey = optString(cm, "secretAccessKey", "")
		cfg.sessionToken = optString(cm, "sessionToken", "")
	}
	return cfg, nil
}

// load builds the aws.Config: the test seam wins (static creds + httptest
// client); otherwise the default chain plus any explicit region/profile/creds.
func (c awsConfig) load(ctx context.Context) (aws.Config, error) {
	if awsTestOptions != nil {
		return awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion("us-east-1"),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
			awsconfig.WithHTTPClient(awsTestOptions.httpClient),
		)
	}
	var loadOpts []func(*awsconfig.LoadOptions) error
	if c.region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(c.region))
	}
	if c.profile != "" {
		loadOpts = append(loadOpts, awsconfig.WithSharedConfigProfile(c.profile))
	}
	if c.accessKeyID != "" || c.secretAccessKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.accessKeyID, c.secretAccessKey, c.sessionToken)))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return aws.Config{}, mapAWSError(err)
	}
	return cfg, nil
}

func (c awsConfig) String() string {
	creds := "chain"
	if c.accessKeyID != "" || c.secretAccessKey != "" {
		creds = "explicit(redacted)"
	}
	return fmt.Sprintf("awsConfig{region:%q profile:%q creds:%s}", c.region, c.profile, creds)
}

// awsHandle builds the object returned by cloud.aws(...): one accessor per
// AWS service, each lazily building its own typed service map (so, e.g.,
// cloud.aws(cfg).s3() only pays for an s3.Client if the script calls it).
func awsHandle(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	return map[string]any{
		"s3":             func(goja.FunctionCall) goja.Value { return vm.ToValue(awsS3(vm, loop, cfg)) },
		"ec2":            func(goja.FunctionCall) goja.Value { return vm.ToValue(awsEC2(vm, loop, cfg)) },
		"iam":            func(goja.FunctionCall) goja.Value { return vm.ToValue(awsIAM(vm, loop, cfg)) },
		"secretsmanager": func(goja.FunctionCall) goja.Value { return vm.ToValue(awsSecretsManager(vm, loop, cfg)) },
		"sts":            func(goja.FunctionCall) goja.Value { return vm.ToValue(awsSTS(vm, loop, cfg)) },
		"lambda":         func(goja.FunctionCall) goja.Value { return vm.ToValue(awsLambda(vm, loop, cfg)) },
		"sqs":            func(goja.FunctionCall) goja.Value { return vm.ToValue(awsSQS(vm, loop, cfg)) },
		"cloudwatch":     func(goja.FunctionCall) goja.Value { return vm.ToValue(awsCloudWatch(vm, loop, cfg)) },
		"cloudwatchlogs": func(goja.FunctionCall) goja.Value { return vm.ToValue(awsCloudWatchLogs(vm, loop, cfg)) },
	}
}

type awsError struct {
	code    string
	status  int
	message string
	details any
}

func (e awsError) Error() string {
	return fmt.Sprintf("cloud.aws: %s (%d): %s", e.code, e.status, e.message)
}

func (e awsError) ErrorFields() map[string]any {
	return map[string]any{"code": e.code, "status": e.status, "message": e.message, "details": e.details}
}

// mapAWSError normalises a smithy error into a structured awsError. Non-API
// errors (DNS/TLS/timeout) map to code "" / status 0, with the raw error text
// as the message.
func mapAWSError(err error) error {
	if err == nil {
		return nil
	}
	var out awsError
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		out.code = apiErr.ErrorCode()
		out.message = apiErr.ErrorMessage()
	} else {
		out.message = err.Error()
	}
	var re *smithyhttp.ResponseError
	if errors.As(err, &re) && re.Response != nil {
		out.status = re.HTTPStatusCode()
	}
	return out
}
