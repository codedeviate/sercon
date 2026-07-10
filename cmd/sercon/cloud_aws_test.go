package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runCloudAWSScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("cloud", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return cloudNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "c.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestCloudAWS_HandleShape(t *testing.T) {
	got := runCloudAWSScript(t, `
		const aws = cloud.aws({ region: "eu-north-1" });
		const __result = {
			isFn: typeof cloud.aws === "function",
			s3: typeof aws.s3, ec2: typeof aws.ec2, iam: typeof aws.iam,
			secretsmanager: typeof aws.secretsmanager, sts: typeof aws.sts,
			lambda: typeof aws.lambda, sqs: typeof aws.sqs,
			cloudwatch: typeof aws.cloudwatch, cloudwatchlogs: typeof aws.cloudwatchlogs,
		};
	`)
	m := got.(map[string]any)
	if m["isFn"] != true {
		t.Fatal("cloud.aws must be callable")
	}
	for _, k := range []string{"s3", "ec2", "iam", "secretsmanager", "sts", "lambda", "sqs", "cloudwatch", "cloudwatchlogs"} {
		if m[k] != "function" {
			t.Fatalf("expected aws.%s to be a function, got %v", k, m[k])
		}
	}
}

func TestAWSConfig_CredsNeverLogged(t *testing.T) {
	c := awsConfig{region: "eu-north-1", accessKeyID: "AKIASECRET", secretAccessKey: "topsecret", sessionToken: "tok"}
	s := c.String()
	for _, leak := range []string{"AKIASECRET", "topsecret", "tok"} {
		if strings.Contains(s, leak) {
			t.Fatalf("awsConfig.String() leaked credential material: %q", s)
		}
	}
}
