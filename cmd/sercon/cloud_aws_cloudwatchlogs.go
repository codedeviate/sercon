package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsCloudWatchLogsArgs is the plain-Go carrier for every
// cloud.aws(...).cloudwatchlogs() method: extracted on-loop by
// awsCloudWatchLogsExtract, consumed off-loop by the awsCloudWatchLogsXxx work
// funcs.
type awsCloudWatchLogsArgs struct {
	prefix, logGroupName, logStreamName, filterPattern, queryString, queryId string
	limit                                                                    int
	startTime, endTime                                                       int64
}

// newCloudWatchLogsClient builds a cloudwatchlogs.Client for cfg. Unlike its
// sibling cloudwatch (metrics) module — pinned below v1.53.0 because that
// module migrated to the Smithy RPC v2 CBOR protocol — cloudwatchlogs is
// still on the classic awsjson1.1 protocol at @latest (v1.79.0, checked by
// inspecting the resolved module's generated serializers.go for
// awsAwsjson11_serializeOp* symbols and the absence of any cbor/rpcv2
// symbols), so no version pin is needed here. Addressed by BaseEndpoint
// alone — like Secrets Manager/EC2/IAM/SQS/STS, no UsePathStyle option
// applies here.
func newCloudWatchLogsClient(ctx context.Context, cfg awsConfig) (*cloudwatchlogs.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return cloudwatchlogs.NewFromConfig(base, func(o *cloudwatchlogs.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsCloudWatchLogsDescribeLogGroups(ctx context.Context, cfg awsConfig, a awsCloudWatchLogsArgs) (any, error) {
	c, err := newCloudWatchLogsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &cloudwatchlogs.DescribeLogGroupsInput{}
	if a.prefix != "" {
		in.LogGroupNamePrefix = aws.String(a.prefix)
	}
	out, err := c.DescribeLogGroups(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsCloudWatchLogsDescribeLogStreams(ctx context.Context, cfg awsConfig, a awsCloudWatchLogsArgs) (any, error) {
	c, err := newCloudWatchLogsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(a.logGroupName),
	})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsCloudWatchLogsGetLogEvents(ctx context.Context, cfg awsConfig, a awsCloudWatchLogsArgs) (any, error) {
	c, err := newCloudWatchLogsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(a.logGroupName),
		LogStreamName: aws.String(a.logStreamName),
	}
	if a.limit > 0 {
		in.Limit = aws.Int32(int32(a.limit))
	}
	out, err := c.GetLogEvents(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsCloudWatchLogsFilterLogEvents(ctx context.Context, cfg awsConfig, a awsCloudWatchLogsArgs) (any, error) {
	c, err := newCloudWatchLogsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &cloudwatchlogs.FilterLogEventsInput{LogGroupName: aws.String(a.logGroupName)}
	if a.filterPattern != "" {
		in.FilterPattern = aws.String(a.filterPattern)
	}
	out, err := c.FilterLogEvents(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsCloudWatchLogsStartQuery's StartTime/EndTime are epoch seconds (unlike
// GetLogEvents/FilterLogEvents, whose StartTime/EndTime are epoch
// milliseconds) — per the StartQueryInput doc comment ("Specified as epoch
// time, the number of seconds since January 1, 1970").
func awsCloudWatchLogsStartQuery(ctx context.Context, cfg awsConfig, a awsCloudWatchLogsArgs) (any, error) {
	c, err := newCloudWatchLogsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.StartQuery(ctx, &cloudwatchlogs.StartQueryInput{
		LogGroupName: aws.String(a.logGroupName),
		QueryString:  aws.String(a.queryString),
		StartTime:    aws.Int64(a.startTime),
		EndTime:      aws.Int64(a.endTime),
	})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsCloudWatchLogsGetQueryResults(ctx context.Context, cfg awsConfig, a awsCloudWatchLogsArgs) (any, error) {
	c, err := newCloudWatchLogsClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.GetQueryResults(ctx, &cloudwatchlogs.GetQueryResultsInput{QueryId: aws.String(a.queryId)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsCloudWatchLogsExtract reads the single options object on the event loop.
func awsCloudWatchLogsExtract(call goja.FunctionCall) (awsCloudWatchLogsArgs, error) {
	var a awsCloudWatchLogsArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.prefix = optString(o, "prefix", "")
	a.logGroupName = optString(o, "logGroupName", "")
	a.logStreamName = optString(o, "logStreamName", "")
	a.filterPattern = optString(o, "filterPattern", "")
	a.queryString = optString(o, "queryString", "")
	a.queryId = optString(o, "queryId", "")
	a.limit = optInt(o, "limit", 0)
	a.startTime = int64(optInt(o, "startTime", 0))
	a.endTime = int64(optInt(o, "endTime", 0))
	return a, nil
}

// awsCloudWatchLogs builds the object returned by
// cloud.aws(...).cloudwatchlogs(): one PromisifyAsync binding per method, all
// sharing awsCloudWatchLogsExtract and cfg.
//
// This map is built at script-run time (inside the cloudwatchlogs() accessor
// call in cloud_aws.go), past the engine's registration-time AsyncBinding
// unwrap — so each binding's `.Func` must be unwrapped explicitly here (same
// pattern as awsS3/awsSecretsManager/awsEC2/awsIAM/awsLambda/awsSQS/
// awsCloudWatch).
func awsCloudWatchLogs(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsCloudWatchLogsArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsCloudWatchLogsExtract,
			func(ctx context.Context, a awsCloudWatchLogsArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"describeLogGroups":  bind(awsCloudWatchLogsDescribeLogGroups),
		"describeLogStreams": bind(awsCloudWatchLogsDescribeLogStreams),
		"getLogEvents":       bind(awsCloudWatchLogsGetLogEvents),
		"filterLogEvents":    bind(awsCloudWatchLogsFilterLogEvents),
		"startQuery":         bind(awsCloudWatchLogsStartQuery),
		"getQueryResults":    bind(awsCloudWatchLogsGetQueryResults),
	}
}
