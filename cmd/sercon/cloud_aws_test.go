package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	smithy "github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
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

func TestMapAWSError(t *testing.T) {
	apiErr := &smithy.GenericAPIError{Code: "NoSuchBucket", Message: "The bucket does not exist"}
	wrapped := &smithyhttp.ResponseError{Response: &smithyhttp.Response{Response: &http.Response{StatusCode: 404}}, Err: apiErr}
	ae, ok := mapAWSError(wrapped).(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T", mapAWSError(wrapped))
	}
	f := ae.ErrorFields()
	if f["code"] != "NoSuchBucket" || f["status"] != 404 {
		t.Fatalf("bad fields: %#v", f)
	}
	if !strings.Contains(ae.Error(), "The bucket does not exist") {
		t.Fatalf("message should include the API message, got %q", ae.Error())
	}
}

func TestCloudAWS_RejectsIncompleteCredentials(t *testing.T) {
	got := runCloudAWSScript(t, `
		let msg = "";
		try { cloud.aws({ credentials: { sessionToken: "x" } }); } catch (e) { msg = e.message; }
		const __result = { msg };
	`)
	m := got.(map[string]any)
	if s, _ := m["msg"].(string); !strings.Contains(s, "accessKeyId and secretAccessKey") {
		t.Fatalf("expected a credentials-validation error, got %q", s)
	}
}

func TestMapAWSError_Nil(t *testing.T) {
	if got := mapAWSError(nil); got != nil {
		t.Fatalf("mapAWSError(nil) should be nil, got %v", got)
	}
}

func TestMapAWSError_TransportError(t *testing.T) {
	// A non-API (transport) error: no smithy.APIError, no ResponseError.
	ae, ok := mapAWSError(errors.New("dial tcp 10.0.0.1:443: i/o timeout")).(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T", mapAWSError(errors.New("x")))
	}
	f := ae.ErrorFields()
	if f["code"] != "" || f["status"] != 0 {
		t.Fatalf("transport error should have empty code / status 0, got %#v", f)
	}
	if !strings.Contains(ae.Error(), "i/o timeout") {
		t.Fatalf("message should carry the raw transport error, got %q", ae.Error())
	}
	if f["details"] != nil {
		t.Fatalf("transport error should have nil details, got %#v", f["details"])
	}
}

func TestMapAWSError_PopulatesFaultDetails(t *testing.T) {
	apiErr := &smithy.GenericAPIError{Code: "AccessDenied", Message: "no", Fault: smithy.FaultClient}
	ae := mapAWSError(apiErr).(awsError)
	d, ok := ae.ErrorFields()["details"].(map[string]any)
	if !ok {
		t.Fatalf("API error should populate details, got %#v", ae.ErrorFields()["details"])
	}
	if d["fault"] != "client" {
		t.Fatalf("expected fault 'client', got %#v", d["fault"])
	}
}
