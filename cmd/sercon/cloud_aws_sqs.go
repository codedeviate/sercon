package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// awsSQSArgs is the plain-Go carrier for every cloud.aws(...).sqs() method:
// extracted on-loop by awsSQSExtract, consumed off-loop by the awsSQSXxx work
// funcs.
type awsSQSArgs struct {
	prefix, queueName, queueUrl, messageBody, receiptHandle string
	maxMessages                                             int
	attributeNames                                          []string
}

// newSQSClient builds an sqs.Client for cfg. SQS's wire protocol is JSON
// (awsjson1.0), addressed by BaseEndpoint alone — like Secrets Manager/EC2/IAM,
// no UsePathStyle option applies here.
func newSQSClient(ctx context.Context, cfg awsConfig) (*sqs.Client, error) {
	base, err := cfg.load(ctx)
	if err != nil {
		return nil, err
	}
	return sqs.NewFromConfig(base, func(o *sqs.Options) {
		if ep := awsBaseEndpoint(); ep != nil {
			o.BaseEndpoint = ep
		}
	}), nil
}

func awsSQSListQueues(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &sqs.ListQueuesInput{}
	if a.prefix != "" {
		in.QueueNamePrefix = aws.String(a.prefix)
	}
	out, err := c.ListQueues(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSQSCreateQueue(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(a.queueName)})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSQSDeleteQueue(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(a.queueUrl)}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

func awsSQSSendMessage(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	out, err := c.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(a.queueUrl),
		MessageBody: aws.String(a.messageBody),
	})
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSQSReceiveMessage(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &sqs.ReceiveMessageInput{QueueUrl: aws.String(a.queueUrl)}
	if a.maxMessages > 0 {
		in.MaxNumberOfMessages = int32(a.maxMessages)
	}
	out, err := c.ReceiveMessage(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

func awsSQSDeleteMessage(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := c.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(a.queueUrl),
		ReceiptHandle: aws.String(a.receiptHandle),
	}); err != nil {
		return nil, mapAWSError(err)
	}
	return map[string]any{}, nil
}

func awsSQSGetQueueAttributes(ctx context.Context, cfg awsConfig, a awsSQSArgs) (any, error) {
	c, err := newSQSClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	in := &sqs.GetQueueAttributesInput{QueueUrl: aws.String(a.queueUrl)}
	if len(a.attributeNames) > 0 {
		names := make([]types.QueueAttributeName, len(a.attributeNames))
		for i, n := range a.attributeNames {
			names[i] = types.QueueAttributeName(n)
		}
		in.AttributeNames = names
	}
	out, err := c.GetQueueAttributes(ctx, in)
	if err != nil {
		return nil, mapAWSError(err)
	}
	return toPlain(out)
}

// awsSQSExtract reads the single options object on the event loop.
func awsSQSExtract(call goja.FunctionCall) (awsSQSArgs, error) {
	var a awsSQSArgs
	obj, ok := call.Argument(0).(*goja.Object)
	if !ok {
		return a, nil
	}
	o, ok := obj.Export().(map[string]any)
	if !ok {
		return a, nil
	}
	a.prefix = optString(o, "prefix", "")
	a.queueName = optString(o, "queueName", "")
	a.queueUrl = optString(o, "queueUrl", "")
	a.messageBody = optString(o, "messageBody", "")
	a.receiptHandle = optString(o, "receiptHandle", "")
	a.maxMessages = optInt(o, "maxMessages", 0)
	a.attributeNames = optStringSlice(o, "attributeNames")
	return a, nil
}

// awsSQS builds the object returned by cloud.aws(...).sqs(): one
// PromisifyAsync binding per method, all sharing awsSQSExtract and cfg.
//
// This map is built at script-run time (inside the sqs() accessor call in
// cloud_aws.go), past the engine's registration-time AsyncBinding unwrap — so
// each binding's `.Func` must be unwrapped explicitly here (same pattern as
// awsS3/awsSecretsManager).
func awsSQS(vm *goja.Runtime, loop *eventloop.EventLoop, cfg awsConfig) map[string]any {
	bind := func(fn func(context.Context, awsConfig, awsSQSArgs) (any, error)) func(goja.FunctionCall) goja.Value {
		return scriptengine.PromisifyAsync(vm, loop, awsSQSExtract,
			func(ctx context.Context, a awsSQSArgs) (any, error) { return fn(ctx, cfg, a) }).Func
	}
	return map[string]any{
		"listQueues":         bind(awsSQSListQueues),
		"createQueue":        bind(awsSQSCreateQueue),
		"deleteQueue":        bind(awsSQSDeleteQueue),
		"sendMessage":        bind(awsSQSSendMessage),
		"receiveMessage":     bind(awsSQSReceiveMessage),
		"deleteMessage":      bind(awsSQSDeleteMessage),
		"getQueueAttributes": bind(awsSQSGetQueueAttributes),
	}
}
