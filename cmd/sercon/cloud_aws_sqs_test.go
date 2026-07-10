package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSSQS_ListQueues(t *testing.T) {
	var gotTarget string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{
			"QueueUrls": ["https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue"]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSListQueues(context.Background(), awsConfig{}, awsSQSArgs{})
	if err != nil {
		t.Fatalf("listQueues: %v", err)
	}
	if gotTarget != "AmazonSQS.ListQueues" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.ListQueues, got %q", gotTarget)
	}
	m := out.(map[string]any)
	urls, ok := m["QueueUrls"].([]any)
	if !ok || len(urls) != 1 {
		t.Fatalf("expected 1 queue url, got %#v", m["QueueUrls"])
	}
}

func TestAWSSQS_CreateQueue(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{
			"QueueUrl": "https://sqs.eu-north-1.amazonaws.com/123456789012/new-queue"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSCreateQueue(context.Background(), awsConfig{}, awsSQSArgs{queueName: "new-queue"})
	if err != nil {
		t.Fatalf("createQueue: %v", err)
	}
	if gotTarget != "AmazonSQS.CreateQueue" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.CreateQueue, got %q", gotTarget)
	}
	if gotBody["QueueName"] != "new-queue" {
		t.Fatalf("expected QueueName new-queue, got %#v", gotBody["QueueName"])
	}
	m := out.(map[string]any)
	if m["QueueUrl"] != "https://sqs.eu-north-1.amazonaws.com/123456789012/new-queue" {
		t.Fatalf("expected QueueUrl to round-trip, got %#v", m["QueueUrl"])
	}
}

func TestAWSSQS_DeleteQueue(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSDeleteQueue(context.Background(), awsConfig{}, awsSQSArgs{queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue"})
	if err != nil {
		t.Fatalf("deleteQueue: %v", err)
	}
	if gotTarget != "AmazonSQS.DeleteQueue" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.DeleteQueue, got %q", gotTarget)
	}
	if gotBody["QueueUrl"] != "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue" {
		t.Fatalf("expected QueueUrl to round-trip, got %#v", gotBody["QueueUrl"])
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

func TestAWSSQS_SendMessage(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{
			"MD5OfMessageBody": "5d41402abc4b2a76b9719d911017c592",
			"MessageId": "msg-1"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSSendMessage(context.Background(), awsConfig{}, awsSQSArgs{
		queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", messageBody: "hello",
	})
	if err != nil {
		t.Fatalf("sendMessage: %v", err)
	}
	if gotTarget != "AmazonSQS.SendMessage" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.SendMessage, got %q", gotTarget)
	}
	if gotBody["MessageBody"] != "hello" {
		t.Fatalf("expected MessageBody to round-trip, got %#v", gotBody["MessageBody"])
	}
	m := out.(map[string]any)
	if m["MessageId"] != "msg-1" {
		t.Fatalf("expected MessageId msg-1, got %#v", m["MessageId"])
	}
}

func TestAWSSQS_ReceiveMessage(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{
			"Messages": [
				{"MessageId": "msg-1", "ReceiptHandle": "rh-1", "Body": "hello"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSReceiveMessage(context.Background(), awsConfig{}, awsSQSArgs{
		queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", maxMessages: 5,
	})
	if err != nil {
		t.Fatalf("receiveMessage: %v", err)
	}
	if gotTarget != "AmazonSQS.ReceiveMessage" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.ReceiveMessage, got %q", gotTarget)
	}
	if n, ok := gotBody["MaxNumberOfMessages"].(float64); !ok || int(n) != 5 {
		t.Fatalf("expected MaxNumberOfMessages 5 to round-trip, got %#v", gotBody["MaxNumberOfMessages"])
	}
	m := out.(map[string]any)
	msgs, ok := m["Messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %#v", m["Messages"])
	}
	msg := msgs[0].(map[string]any)
	if msg["Body"] != "hello" {
		t.Fatalf("expected Body hello, got %#v", msg["Body"])
	}
}

func TestAWSSQS_DeleteMessage(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSDeleteMessage(context.Background(), awsConfig{}, awsSQSArgs{
		queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", receiptHandle: "rh-1",
	})
	if err != nil {
		t.Fatalf("deleteMessage: %v", err)
	}
	if gotTarget != "AmazonSQS.DeleteMessage" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.DeleteMessage, got %q", gotTarget)
	}
	if gotBody["ReceiptHandle"] != "rh-1" {
		t.Fatalf("expected ReceiptHandle to round-trip, got %#v", gotBody["ReceiptHandle"])
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

func TestAWSSQS_GetQueueAttributes(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{
			"Attributes": {
				"QueueArn": "arn:aws:sqs:eu-north-1:123456789012:my-queue",
				"ApproximateNumberOfMessages": "3"
			}
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSGetQueueAttributes(context.Background(), awsConfig{}, awsSQSArgs{
		queueUrl:       "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue",
		attributeNames: []string{"QueueArn", "ApproximateNumberOfMessages"},
	})
	if err != nil {
		t.Fatalf("getQueueAttributes: %v", err)
	}
	if gotTarget != "AmazonSQS.GetQueueAttributes" {
		t.Fatalf("expected X-Amz-Target AmazonSQS.GetQueueAttributes, got %q", gotTarget)
	}
	names, ok := gotBody["AttributeNames"].([]any)
	if !ok || len(names) != 2 {
		t.Fatalf("expected 2 attribute names to round-trip, got %#v", gotBody["AttributeNames"])
	}
	m := out.(map[string]any)
	attrs, ok := m["Attributes"].(map[string]any)
	if !ok || attrs["QueueArn"] != "arn:aws:sqs:eu-north-1:123456789012:my-queue" {
		t.Fatalf("expected Attributes map, got %#v", m["Attributes"])
	}
}

// TestAWSSQS_ErrorPathThrows mirrors TestAWSSecretsManager_ErrorPathThrows:
// proves an awsjson1.0 error response is mapped end to end (SDK response ->
// smithy APIError -> mapAWSError) into a structured awsError.
func TestAWSSQS_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"__type": "QueueDoesNotExist",
			"message": "The specified queue does not exist."
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsSQSDeleteQueue(context.Background(), awsConfig{}, awsSQSArgs{queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/ghost"})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", ae.status)
	}
	if ae.code != "QueueDoesNotExist" {
		t.Fatalf("expected code QueueDoesNotExist, got %q", ae.code)
	}
}

func TestAWSSQS_ListQueues_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{
			"QueueUrls": ["https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue"]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).sqs().listQueues();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	urls, ok := m["QueueUrls"].([]any)
	if !ok || len(urls) != 1 {
		t.Fatalf("expected 1 queue url, got %#v", m["QueueUrls"])
	}
}
