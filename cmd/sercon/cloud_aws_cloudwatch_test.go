package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSCloudWatch_ListMetrics(t *testing.T) {
	var gotAction, gotNamespace, gotMetricName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotNamespace = r.Form.Get("Namespace")
		gotMetricName = r.Form.Get("MetricName")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListMetricsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <ListMetricsResult>
    <Metrics>
      <member>
        <Namespace>AWS/EC2</Namespace>
        <MetricName>CPUUtilization</MetricName>
        <Dimensions>
          <member>
            <Name>InstanceId</Name>
            <Value>i-1234567890abcdef0</Value>
          </member>
        </Dimensions>
      </member>
    </Metrics>
  </ListMetricsResult>
  <ResponseMetadata>
    <RequestId>req-1</RequestId>
  </ResponseMetadata>
</ListMetricsResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchListMetrics(context.Background(), awsConfig{}, awsCloudWatchArgs{
		namespace: "AWS/EC2", metricName: "CPUUtilization",
	})
	if err != nil {
		t.Fatalf("listMetrics: %v", err)
	}
	if gotAction != "ListMetrics" {
		t.Fatalf("expected Action=ListMetrics, got %q", gotAction)
	}
	if gotNamespace != "AWS/EC2" {
		t.Fatalf("expected Namespace AWS/EC2, got %q", gotNamespace)
	}
	if gotMetricName != "CPUUtilization" {
		t.Fatalf("expected MetricName CPUUtilization, got %q", gotMetricName)
	}
	m := out.(map[string]any)
	metrics, ok := m["Metrics"].([]any)
	if !ok || len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %#v", m["Metrics"])
	}
	metric := metrics[0].(map[string]any)
	if metric["MetricName"] != "CPUUtilization" {
		t.Fatalf("expected MetricName CPUUtilization, got %#v", metric["MetricName"])
	}
}

func TestAWSCloudWatch_DescribeAlarms(t *testing.T) {
	var gotAction, gotAlarmName string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotAlarmName = r.Form.Get("AlarmNames.member.1")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DescribeAlarmsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <DescribeAlarmsResult>
    <MetricAlarms>
      <member>
        <AlarmName>high-cpu</AlarmName>
        <MetricName>CPUUtilization</MetricName>
        <Namespace>AWS/EC2</Namespace>
        <StateValue>ALARM</StateValue>
      </member>
    </MetricAlarms>
  </DescribeAlarmsResult>
  <ResponseMetadata>
    <RequestId>req-2</RequestId>
  </ResponseMetadata>
</DescribeAlarmsResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchDescribeAlarms(context.Background(), awsConfig{}, awsCloudWatchArgs{alarmNames: []string{"high-cpu"}})
	if err != nil {
		t.Fatalf("describeAlarms: %v", err)
	}
	if gotAction != "DescribeAlarms" {
		t.Fatalf("expected Action=DescribeAlarms, got %q", gotAction)
	}
	if gotAlarmName != "high-cpu" {
		t.Fatalf("expected AlarmNames.member.1 high-cpu, got %q", gotAlarmName)
	}
	m := out.(map[string]any)
	alarms, ok := m["MetricAlarms"].([]any)
	if !ok || len(alarms) != 1 {
		t.Fatalf("expected 1 alarm, got %#v", m["MetricAlarms"])
	}
	alarm := alarms[0].(map[string]any)
	if alarm["AlarmName"] != "high-cpu" || alarm["StateValue"] != "ALARM" {
		t.Fatalf("expected AlarmName high-cpu / StateValue ALARM, got %#v", alarm)
	}
}

// TestAWSCloudWatch_GetMetricStatistics_PassThrough proves the JSON
// round-trip for the pass-through input: an SDK-shaped options object
// (PascalCase keys, RFC3339 timestamp strings, an enum string for
// Statistics) is JSON-marshalled then unmarshalled straight into
// cloudwatch.GetMetricStatisticsInput, and the resulting request carries
// Namespace/MetricName/StartTime/EndTime/Period/Statistics correctly onto
// the wire.
func TestAWSCloudWatch_GetMetricStatistics_PassThrough(t *testing.T) {
	var gotAction, gotNamespace, gotMetricName, gotStart, gotEnd, gotPeriod, gotStat string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotNamespace = r.Form.Get("Namespace")
		gotMetricName = r.Form.Get("MetricName")
		gotStart = r.Form.Get("StartTime")
		gotEnd = r.Form.Get("EndTime")
		gotPeriod = r.Form.Get("Period")
		gotStat = r.Form.Get("Statistics.member.1")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetMetricStatisticsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <GetMetricStatisticsResult>
    <Label>CPUUtilization</Label>
    <Datapoints>
      <member>
        <Timestamp>2024-01-15T10:30:00Z</Timestamp>
        <Average>42.5</Average>
        <Unit>Percent</Unit>
      </member>
    </Datapoints>
  </GetMetricStatisticsResult>
  <ResponseMetadata>
    <RequestId>req-3</RequestId>
  </ResponseMetadata>
</GetMetricStatisticsResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchGetMetricStatistics(context.Background(), awsConfig{}, awsCloudWatchArgs{
		raw: map[string]any{
			"Namespace":  "AWS/EC2",
			"MetricName": "CPUUtilization",
			"StartTime":  "2024-01-15T09:30:00Z",
			"EndTime":    "2024-01-15T10:30:00Z",
			"Period":     float64(300),
			"Statistics": []any{"Average"},
		},
	})
	if err != nil {
		t.Fatalf("getMetricStatistics: %v", err)
	}
	if gotAction != "GetMetricStatistics" {
		t.Fatalf("expected Action=GetMetricStatistics, got %q", gotAction)
	}
	if gotNamespace != "AWS/EC2" {
		t.Fatalf("expected Namespace AWS/EC2, got %q", gotNamespace)
	}
	if gotMetricName != "CPUUtilization" {
		t.Fatalf("expected MetricName CPUUtilization, got %q", gotMetricName)
	}
	if gotStart != "2024-01-15T09:30:00Z" {
		t.Fatalf("expected StartTime 2024-01-15T09:30:00Z (proves RFC3339 -> *time.Time round-trip), got %q", gotStart)
	}
	if gotEnd != "2024-01-15T10:30:00Z" {
		t.Fatalf("expected EndTime 2024-01-15T10:30:00Z, got %q", gotEnd)
	}
	if gotPeriod != "300" {
		t.Fatalf("expected Period 300, got %q", gotPeriod)
	}
	if gotStat != "Average" {
		t.Fatalf("expected Statistics.member.1 Average (proves string -> types.Statistic round-trip), got %q", gotStat)
	}
	m := out.(map[string]any)
	datapoints, ok := m["Datapoints"].([]any)
	if !ok || len(datapoints) != 1 {
		t.Fatalf("expected 1 datapoint, got %#v", m["Datapoints"])
	}
}

// TestAWSCloudWatch_PutMetricData_PassThrough proves the JSON round-trip for
// a nested MetricDatum (including a Dimension and an RFC3339 Timestamp).
func TestAWSCloudWatch_PutMetricData_PassThrough(t *testing.T) {
	var gotAction, gotNamespace, gotMetricName, gotValue, gotUnit, gotTimestamp, gotDimName, gotDimValue string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotNamespace = r.Form.Get("Namespace")
		gotMetricName = r.Form.Get("MetricData.member.1.MetricName")
		gotValue = r.Form.Get("MetricData.member.1.Value")
		gotUnit = r.Form.Get("MetricData.member.1.Unit")
		gotTimestamp = r.Form.Get("MetricData.member.1.Timestamp")
		gotDimName = r.Form.Get("MetricData.member.1.Dimensions.member.1.Name")
		gotDimValue = r.Form.Get("MetricData.member.1.Dimensions.member.1.Value")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<PutMetricDataResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <ResponseMetadata>
    <RequestId>req-4</RequestId>
  </ResponseMetadata>
</PutMetricDataResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchPutMetricData(context.Background(), awsConfig{}, awsCloudWatchArgs{
		raw: map[string]any{
			"Namespace": "MyApp",
			"MetricData": []any{
				map[string]any{
					"MetricName": "RequestCount",
					"Value":      float64(7),
					"Unit":       "Count",
					"Timestamp":  "2024-01-15T10:30:00Z",
					"Dimensions": []any{
						map[string]any{"Name": "Environment", "Value": "prod"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("putMetricData: %v", err)
	}
	if gotAction != "PutMetricData" {
		t.Fatalf("expected Action=PutMetricData, got %q", gotAction)
	}
	if gotNamespace != "MyApp" {
		t.Fatalf("expected Namespace MyApp, got %q", gotNamespace)
	}
	if gotMetricName != "RequestCount" {
		t.Fatalf("expected MetricData.member.1.MetricName RequestCount, got %q", gotMetricName)
	}
	if gotValue != "7" {
		t.Fatalf("expected MetricData.member.1.Value 7, got %q", gotValue)
	}
	if gotUnit != "Count" {
		t.Fatalf("expected MetricData.member.1.Unit Count (proves string -> types.StandardUnit round-trip), got %q", gotUnit)
	}
	if gotTimestamp != "2024-01-15T10:30:00Z" {
		t.Fatalf("expected MetricData.member.1.Timestamp 2024-01-15T10:30:00Z (proves RFC3339 -> *time.Time round-trip), got %q", gotTimestamp)
	}
	if gotDimName != "Environment" || gotDimValue != "prod" {
		t.Fatalf("expected Dimensions.member.1 Environment/prod, got %q/%q", gotDimName, gotDimValue)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", out)
	}
}

// TestAWSCloudWatch_GetMetricData_PassThrough proves the JSON round-trip for
// the most deeply nested pass-through input: MetricDataQueries containing a
// MetricStat -> Metric -> Dimensions chain plus RFC3339 Start/EndTime.
func TestAWSCloudWatch_GetMetricData_PassThrough(t *testing.T) {
	var gotAction, gotStart, gotEnd, gotQueryId, gotMetricName, gotPeriod, gotStat string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotStart = r.Form.Get("StartTime")
		gotEnd = r.Form.Get("EndTime")
		gotQueryId = r.Form.Get("MetricDataQueries.member.1.Id")
		gotMetricName = r.Form.Get("MetricDataQueries.member.1.MetricStat.Metric.MetricName")
		gotPeriod = r.Form.Get("MetricDataQueries.member.1.MetricStat.Period")
		gotStat = r.Form.Get("MetricDataQueries.member.1.MetricStat.Stat")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<GetMetricDataResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <GetMetricDataResult>
    <MetricDataResults>
      <member>
        <Id>q1</Id>
        <Label>CPUUtilization</Label>
        <StatusCode>Complete</StatusCode>
      </member>
    </MetricDataResults>
  </GetMetricDataResult>
  <ResponseMetadata>
    <RequestId>req-5</RequestId>
  </ResponseMetadata>
</GetMetricDataResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchGetMetricData(context.Background(), awsConfig{}, awsCloudWatchArgs{
		raw: map[string]any{
			"StartTime": "2024-01-15T09:30:00Z",
			"EndTime":   "2024-01-15T10:30:00Z",
			"MetricDataQueries": []any{
				map[string]any{
					"Id": "q1",
					"MetricStat": map[string]any{
						"Metric": map[string]any{
							"Namespace":  "AWS/EC2",
							"MetricName": "CPUUtilization",
						},
						"Period": float64(300),
						"Stat":   "Average",
					},
					"ReturnData": true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("getMetricData: %v", err)
	}
	if gotAction != "GetMetricData" {
		t.Fatalf("expected Action=GetMetricData, got %q", gotAction)
	}
	if gotStart != "2024-01-15T09:30:00Z" || gotEnd != "2024-01-15T10:30:00Z" {
		t.Fatalf("expected StartTime/EndTime round-trip, got %q/%q", gotStart, gotEnd)
	}
	if gotQueryId != "q1" {
		t.Fatalf("expected MetricDataQueries.member.1.Id q1, got %q", gotQueryId)
	}
	if gotMetricName != "CPUUtilization" {
		t.Fatalf("expected nested MetricStat.Metric.MetricName CPUUtilization, got %q", gotMetricName)
	}
	if gotPeriod != "300" {
		t.Fatalf("expected MetricStat.Period 300, got %q", gotPeriod)
	}
	if gotStat != "Average" {
		t.Fatalf("expected MetricStat.Stat Average (proves string -> types.Statistic round-trip), got %q", gotStat)
	}
	m := out.(map[string]any)
	results, ok := m["MetricDataResults"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 metric data result, got %#v", m["MetricDataResults"])
	}
}

// TestAWSCloudWatch_ErrorPathThrows mirrors TestAWSIAM_ErrorPathThrows: proves
// a CloudWatch query-protocol error response is mapped end to end (SDK
// response -> smithy APIError -> mapAWSError) into a structured awsError.
func TestAWSCloudWatch_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <Error>
    <Type>Sender</Type>
    <Code>InvalidParameterValue</Code>
    <Message>The parameter MetricName is not valid.</Message>
  </Error>
  <RequestId>req-err</RequestId>
</ErrorResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsCloudWatchListMetrics(context.Background(), awsConfig{}, awsCloudWatchArgs{})
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
	if ae.code != "InvalidParameterValue" {
		t.Fatalf("expected code InvalidParameterValue, got %q", ae.code)
	}
}

func TestAWSCloudWatch_ListMetrics_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListMetricsResponse xmlns="http://monitoring.amazonaws.com/doc/2010-08-01/">
  <ListMetricsResult>
    <Metrics>
      <member>
        <Namespace>AWS/EC2</Namespace>
        <MetricName>CPUUtilization</MetricName>
      </member>
    </Metrics>
  </ListMetricsResult>
  <ResponseMetadata>
    <RequestId>req-js-1</RequestId>
  </ResponseMetadata>
</ListMetricsResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).cloudwatch().listMetrics({ namespace: "AWS/EC2" });
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	metrics, ok := m["Metrics"].([]any)
	if !ok || len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %#v", m["Metrics"])
	}
}
