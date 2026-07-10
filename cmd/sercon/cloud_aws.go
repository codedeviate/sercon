package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
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

// awsHandle — temporary stub; service accessors implemented in Tasks 3-11.
func awsHandle(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	noop := func(goja.FunctionCall) goja.Value { return goja.Undefined() }
	return map[string]any{
		"s3": noop, "ec2": noop, "iam": noop, "secretsmanager": noop, "sts": noop,
		"lambda": noop, "sqs": noop, "cloudwatch": noop, "cloudwatchlogs": noop,
	}
}

// mapAWSError is a temporary passthrough shim; Task 2 replaces this with a
// mapper that converts AWS SDK errors into structured, script-facing errors.
func mapAWSError(err error) error { return err }
