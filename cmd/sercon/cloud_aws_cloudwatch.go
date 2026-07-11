package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsCloudWatchArgs is the plain-Go carrier for every
// cloud.aws(...).cloudwatch() method: extracted on-loop by
// awsCloudWatchExtract, consumed off-loop by the awsCloudWatchXxx work funcs.
type awsCloudWatchArgs struct {
	namespace, metricName string
	alarmNames            []string
	// raw is the whole options object (map[string]any), exported as-is, for
	// the pass-through methods (getMetricData/getMetricStatistics/
	// putMetricData): each JSON round-trips it straight into the matching SDK
	// Input struct.
	raw map[string]any
}

// newCloudWatchClient builds a cloudwatch.Client for cfg. Like EC2/IAM/SQS/
// STS, CloudWatch's wire protocol is classic query/XML, addressed by
// BaseEndpoint alone — no UsePathStyle option applies here.
//
// Pinned at v1.52.0 (not @latest): AWS SDK for Go v2 migrated the cloudwatch
// module to the Smithy RPC v2 CBOR protocol starting at v1.53.0. Pinning
// below that boundary keeps CloudWatch on the query/XML wire protocol used
// by every sibling service in this file's family (EC2/IAM/S3/SQS/STS), so the
// mock pattern here (httptest + XML bodies, <ErrorResponse><Error> envelope)
// stays consistent across cloud_aws_*.go. See go.mod for the pin.
func newCloudWatchClient(ctx context.Context, cfg awsConfig) (*cloudwatch.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return cloudwatch.NewFromConfig(base, func(o *cloudwatch.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsCloudWatchListMetrics(ctx context.Context, cfg awsConfig, a awsCloudWatchArgs) (any, error) {
	c, err := newCloudWatchClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &cloudwatch.ListMetricsInput{}
	if a.namespace != "" {
		in.Namespace = aws.String(a.namespace)
	}
	if a.metricName != "" {
		in.MetricName = aws.String(a.metricName)
	}
	out, err := c.ListMetrics(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsCloudWatchDescribeAlarms(ctx context.Context, cfg awsConfig, a awsCloudWatchArgs) (any, error) {
	c, err := newCloudWatchClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &cloudwatch.DescribeAlarmsInput{}
	if len(a.alarmNames) > 0 {
		in.AlarmNames = a.alarmNames
	}
	out, err := c.DescribeAlarms(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsCloudWatchGetMetricData is a pass-through method: GetMetricDataInput has
// nested MetricDataQuery/MetricStat/Metric/Dimension slices plus *time.Time
// Start/EndTime fields that don't earn a hand-mapped struct literal. The
// caller supplies an SDK-shaped object (PascalCase keys matching the Go
// struct field names) which is JSON round-tripped straight into the input
// struct: RFC3339 timestamp strings unmarshal into *time.Time (via
// time.Time's own UnmarshalJSON), plain strings into the enum string-alias
// types (types.Statistic, types.StandardUnit, types.ScanBy) since those are
// themselves just named string types with no custom (un)marshaller.
func awsCloudWatchGetMetricData(ctx context.Context, cfg awsConfig, a awsCloudWatchArgs) (any, error) {
	c, err := newCloudWatchClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(a.raw)
	if err != nil {
		return nil, mapAWSError(err)
	}
	var in cloudwatch.GetMetricDataInput
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, mapAWSError(err)
	}
	out, err := c.GetMetricData(ctx, &in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsCloudWatchGetMetricStatistics is a pass-through method; see
// awsCloudWatchGetMetricData for the JSON round-trip rationale.
func awsCloudWatchGetMetricStatistics(ctx context.Context, cfg awsConfig, a awsCloudWatchArgs) (any, error) {
	c, err := newCloudWatchClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(a.raw)
	if err != nil {
		return nil, mapAWSError(err)
	}
	var in cloudwatch.GetMetricStatisticsInput
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, mapAWSError(err)
	}
	out, err := c.GetMetricStatistics(ctx, &in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsCloudWatchPutMetricData is a pass-through method; see
// awsCloudWatchGetMetricData for the JSON round-trip rationale. Per the
// pattern rules, its result is always the empty object (PutMetricData's
// response carries no payload beyond request metadata).
func awsCloudWatchPutMetricData(ctx context.Context, cfg awsConfig, a awsCloudWatchArgs) (any, error) {
	c, err := newCloudWatchClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(a.raw)
	if err != nil {
		return nil, mapAWSError(err)
	}
	var in cloudwatch.PutMetricDataInput
	if err := json.Unmarshal(b, &in); err != nil {
		return nil, mapAWSError(err)
	}
	if _, err := c.PutMetricData(ctx, &in); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

// awsCloudWatchExtract reads the single options object on the event loop.
// namespace/metricName/alarmNames are hand-mapped for listMetrics/
// describeAlarms; raw carries the whole object through for the pass-through
// methods (getMetricData/getMetricStatistics/putMetricData).
func awsCloudWatchExtract(call goja.FunctionCall) (awsCloudWatchArgs, error) {
	var a awsCloudWatchArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.namespace = optString(o, "namespace", "")
	a.metricName = optString(o, "metricName", "")
	a.alarmNames = optStringSlice(o, "alarmNames")
	a.raw = o
	return a, nil
}

// awsCloudWatch builds the object returned by cloud.aws(...).cloudwatch():
// one PromisifyAsync binding per method, all sharing awsCloudWatchExtract and
// cfg.
//
// This map is built at script-run time (inside the cloudwatch() accessor
// call in cloud_aws.go), past the engine's registration-time AsyncBinding
// unwrap — so each binding's `.Func` must be unwrapped explicitly here (same
// pattern as awsS3/awsSecretsManager/awsEC2/awsIAM/awsLambda/awsSQS).
func awsCloudWatch(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsCloudWatchArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsCloudWatchExtract,
			func(ctx context.Context, a awsCloudWatchArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listMetrics":         bind(awsCloudWatchListMetrics),
		"getMetricData":       bind(awsCloudWatchGetMetricData),
		"getMetricStatistics": bind(awsCloudWatchGetMetricStatistics),
		"describeAlarms":      bind(awsCloudWatchDescribeAlarms),
		"putMetricData":       bind(awsCloudWatchPutMetricData),
	}
}
