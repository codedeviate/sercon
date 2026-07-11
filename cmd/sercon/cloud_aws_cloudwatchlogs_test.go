package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSCloudWatchLogs_DescribeLogGroups(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"logGroups": [
				{"logGroupName": "/aws/lambda/my-fn", "arn": "arn:aws:logs:eu-north-1:123456789012:log-group:/aws/lambda/my-fn:*"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsDescribeLogGroups(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{prefix: "/aws/lambda"})
	if err != nil {
		t.Fatalf("describeLogGroups: %v", err)
	}
	if gotTarget != "Logs_20140328.DescribeLogGroups" {
		t.Fatalf("expected X-Amz-Target Logs_20140328.DescribeLogGroups, got %q", gotTarget)
	}
	if gotBody["logGroupNamePrefix"] != "/aws/lambda" {
		t.Fatalf("expected logGroupNamePrefix /aws/lambda, got %#v", gotBody["logGroupNamePrefix"])
	}
	m := out.(map[string]any)
	groups, ok := m["LogGroups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("expected 1 log group, got %#v", m["LogGroups"])
	}
	g := groups[0].(map[string]any)
	if g["LogGroupName"] != "/aws/lambda/my-fn" {
		t.Fatalf("expected LogGroupName /aws/lambda/my-fn, got %#v", g["LogGroupName"])
	}
}

func TestAWSCloudWatchLogs_DescribeLogStreams(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"logStreams": [
				{"logStreamName": "2026/07/11/[$LATEST]abc123"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsDescribeLogStreams(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{logGroupName: "/aws/lambda/my-fn"})
	if err != nil {
		t.Fatalf("describeLogStreams: %v", err)
	}
	if gotTarget != "Logs_20140328.DescribeLogStreams" {
		t.Fatalf("expected X-Amz-Target Logs_20140328.DescribeLogStreams, got %q", gotTarget)
	}
	if gotBody["logGroupName"] != "/aws/lambda/my-fn" {
		t.Fatalf("expected logGroupName /aws/lambda/my-fn, got %#v", gotBody["logGroupName"])
	}
	m := out.(map[string]any)
	streams, ok := m["LogStreams"].([]any)
	if !ok || len(streams) != 1 {
		t.Fatalf("expected 1 log stream, got %#v", m["LogStreams"])
	}
}

func TestAWSCloudWatchLogs_GetLogEvents(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"events": [
				{"message": "hello world", "timestamp": 1751971200000, "ingestionTime": 1751971200500}
			],
			"nextForwardToken": "f/123",
			"nextBackwardToken": "b/123"
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsGetLogEvents(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{
		logGroupName: "/aws/lambda/my-fn", logStreamName: "stream-1", limit: 5,
	})
	if err != nil {
		t.Fatalf("getLogEvents: %v", err)
	}
	if gotTarget != "Logs_20140328.GetLogEvents" {
		t.Fatalf("expected X-Amz-Target Logs_20140328.GetLogEvents, got %q", gotTarget)
	}
	if gotBody["logStreamName"] != "stream-1" {
		t.Fatalf("expected logStreamName stream-1, got %#v", gotBody["logStreamName"])
	}
	if lim, ok := gotBody["limit"].(float64); !ok || lim != 5 {
		t.Fatalf("expected limit 5, got %#v", gotBody["limit"])
	}
	m := out.(map[string]any)
	events, ok := m["Events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 event, got %#v", m["Events"])
	}
	e := events[0].(map[string]any)
	if e["Message"] != "hello world" {
		t.Fatalf("expected Message 'hello world', got %#v", e["Message"])
	}
}

// TestAWSCloudWatchLogs_GetLogEvents_NoLimit proves limit is omitted from the
// wire request when the caller doesn't pass one (mirrors how SQS's
// maxMessages/STS's durationSeconds are handled: only set when > 0).
func TestAWSCloudWatchLogs_GetLogEvents_NoLimit(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"events": []}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	_, err := awsCloudWatchLogsGetLogEvents(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{
		logGroupName: "/aws/lambda/my-fn", logStreamName: "stream-1",
	})
	if err != nil {
		t.Fatalf("getLogEvents: %v", err)
	}
	if _, present := gotBody["limit"]; present {
		t.Fatalf("expected no limit field on the wire, got %#v", gotBody["limit"])
	}
}

func TestAWSCloudWatchLogs_FilterLogEvents(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"events": [
				{"logStreamName": "stream-1", "message": "ERROR boom", "timestamp": 1751971200000}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsFilterLogEvents(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{
		logGroupName: "/aws/lambda/my-fn", filterPattern: "ERROR",
	})
	if err != nil {
		t.Fatalf("filterLogEvents: %v", err)
	}
	if gotTarget != "Logs_20140328.FilterLogEvents" {
		t.Fatalf("expected X-Amz-Target Logs_20140328.FilterLogEvents, got %q", gotTarget)
	}
	if gotBody["filterPattern"] != "ERROR" {
		t.Fatalf("expected filterPattern ERROR, got %#v", gotBody["filterPattern"])
	}
	m := out.(map[string]any)
	events, ok := m["Events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 event, got %#v", m["Events"])
	}
}

func TestAWSCloudWatchLogs_StartQuery(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"queryId": "q-abc-123"}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsStartQuery(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{
		logGroupName: "/aws/lambda/my-fn",
		queryString:  "fields @timestamp, @message | limit 20",
		startTime:    1751971200,
		endTime:      1751974800,
	})
	if err != nil {
		t.Fatalf("startQuery: %v", err)
	}
	if gotTarget != "Logs_20140328.StartQuery" {
		t.Fatalf("expected X-Amz-Target Logs_20140328.StartQuery, got %q", gotTarget)
	}
	// StartTime/EndTime on StartQueryInput are epoch SECONDS, not millis --
	// proves no unit conversion is silently applied.
	if st, ok := gotBody["startTime"].(float64); !ok || st != 1751971200 {
		t.Fatalf("expected startTime 1751971200 (seconds), got %#v", gotBody["startTime"])
	}
	if et, ok := gotBody["endTime"].(float64); !ok || et != 1751974800 {
		t.Fatalf("expected endTime 1751974800 (seconds), got %#v", gotBody["endTime"])
	}
	m := out.(map[string]any)
	if m["QueryId"] != "q-abc-123" {
		t.Fatalf("expected QueryId q-abc-123, got %#v", m["QueryId"])
	}
}

func TestAWSCloudWatchLogs_GetQueryResults(t *testing.T) {
	var gotTarget string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("X-Amz-Target")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"status": "Complete",
			"results": [
				[{"field": "@message", "value": "hello world"}]
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsGetQueryResults(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{queryId: "q-abc-123"})
	if err != nil {
		t.Fatalf("getQueryResults: %v", err)
	}
	if gotTarget != "Logs_20140328.GetQueryResults" {
		t.Fatalf("expected X-Amz-Target Logs_20140328.GetQueryResults, got %q", gotTarget)
	}
	if gotBody["queryId"] != "q-abc-123" {
		t.Fatalf("expected queryId q-abc-123, got %#v", gotBody["queryId"])
	}
	m := out.(map[string]any)
	if m["Status"] != "Complete" {
		t.Fatalf("expected Status Complete, got %#v", m["Status"])
	}
	results, ok := m["Results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 result row, got %#v", m["Results"])
	}
}

// TestAWSCloudWatchLogs_ErrorPathThrows mirrors
// TestAWSSecretsManager_ErrorPathThrows: proves an awsjson1.1 error response
// is mapped end to end (SDK response -> smithy APIError -> mapAWSError) into
// a structured awsError.
func TestAWSCloudWatchLogs_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"__type": "ResourceNotFoundException",
			"message": "The specified log group does not exist."
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchLogsDescribeLogStreams(context.Background(), awsConfig{}, awsCloudWatchLogsArgs{logGroupName: "ghost"})
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
	if ae.code != "ResourceNotFoundException" {
		t.Fatalf("expected code ResourceNotFoundException, got %q", ae.code)
	}
}

func TestAWSCloudWatchLogs_DescribeLogGroups_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{
			"logGroups": [
				{"logGroupName": "/aws/lambda/my-fn"}
			]
		}`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().describeLogGroups();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	groups, ok := m["LogGroups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("expected 1 log group, got %#v", m["LogGroups"])
	}
}
