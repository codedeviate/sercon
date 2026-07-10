package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsEC2Args is the plain-Go carrier for every cloud.aws(...).ec2() method:
// extracted on-loop by awsEC2Extract, consumed off-loop by the awsEC2Xxx work
// funcs.
type awsEC2Args struct {
	instanceIds, volumeIds []string
	imageId, instanceType  string
	minCount, maxCount     int
}

// newEC2Client builds an ec2.Client for cfg. Unlike S3, EC2 needs no
// UsePathStyle option — its query/XML wire protocol addresses everything by
// action + BaseEndpoint, not by URL host/path.
func newEC2Client(ctx context.Context, cfg awsConfig) (*ec2.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return ec2.NewFromConfig(base, func(o *ec2.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsEC2DescribeInstances(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &ec2.DescribeInstancesInput{}
	if len(a.instanceIds) > 0 {
		in.InstanceIds = a.instanceIds
	}
	out, err := c.DescribeInstances(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsEC2RunInstances(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId:      aws.String(a.imageId),
		InstanceType: types.InstanceType(a.instanceType),
		MinCount:     aws.Int32(int32(a.minCount)),
		MaxCount:     aws.Int32(int32(a.maxCount)),
	})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsEC2TerminateInstances(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: a.instanceIds})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsEC2StartInstances(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: a.instanceIds})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsEC2StopInstances(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: a.instanceIds})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsEC2DescribeVolumes(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &ec2.DescribeVolumesInput{}
	if len(a.volumeIds) > 0 {
		in.VolumeIds = a.volumeIds
	}
	out, err := c.DescribeVolumes(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsEC2DescribeAvailabilityZones(ctx context.Context, cfg awsConfig, a awsEC2Args) (any, error) {
	c, err := newEC2Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsEC2Extract reads the single options object on the event loop. minCount
// and maxCount default to 1 (matching the EC2 API's own RunInstances
// defaults) when omitted.
func awsEC2Extract(call goja.FunctionCall) (awsEC2Args, error) {
	var a awsEC2Args
	a.minCount, a.maxCount = 1, 1
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.instanceIds = optStringSlice(o, "instanceIds")
	a.volumeIds = optStringSlice(o, "volumeIds")
	a.imageId = optString(o, "imageId", "")
	a.instanceType = optString(o, "instanceType", "")
	a.minCount = optInt(o, "minCount", 1)
	a.maxCount = optInt(o, "maxCount", 1)
	return a, nil
}

// awsEC2 builds the object returned by cloud.aws(...).ec2(): one
// PromisifyAsync binding per method, all sharing awsEC2Extract and cfg.
//
// This map is built at script-run time (inside the ec2() accessor call in
// cloud_aws.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// awsS3).
func awsEC2(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsEC2Args) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsEC2Extract,
			func(ctx context.Context, a awsEC2Args) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"describeInstances":         bind(awsEC2DescribeInstances),
		"runInstances":              bind(awsEC2RunInstances),
		"terminateInstances":        bind(awsEC2TerminateInstances),
		"startInstances":            bind(awsEC2StartInstances),
		"stopInstances":             bind(awsEC2StopInstances),
		"describeVolumes":           bind(awsEC2DescribeVolumes),
		"describeAvailabilityZones": bind(awsEC2DescribeAvailabilityZones),
	}
}
