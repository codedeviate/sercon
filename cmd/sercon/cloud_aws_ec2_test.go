package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAWSEC2_DescribeInstances(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-1</requestId>
  <reservationSet>
    <item>
      <reservationId>r-1234567890abcdef0</reservationId>
      <ownerId>123456789012</ownerId>
      <instancesSet>
        <item>
          <instanceId>i-1234567890abcdef0</instanceId>
          <imageId>ami-0abcdef1234567890</imageId>
          <instanceType>t3.micro</instanceType>
          <instanceState>
            <code>16</code>
            <name>running</name>
          </instanceState>
        </item>
      </instancesSet>
    </item>
  </reservationSet>
</DescribeInstancesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2DescribeInstances(context.Background(), awsConfig{}, awsEC2Args{})
	if err != nil {
		t.Fatalf("describeInstances: %v", err)
	}
	if gotAction != "DescribeInstances" {
		t.Fatalf("expected Action=DescribeInstances, got %q", gotAction)
	}
	m := out.(map[string]any)
	reservations, ok := m["Reservations"].([]any)
	if !ok || len(reservations) != 1 {
		t.Fatalf("expected 1 reservation, got %#v", m["Reservations"])
	}
	rsv := reservations[0].(map[string]any)
	instances, ok := rsv["Instances"].([]any)
	if !ok || len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %#v", rsv["Instances"])
	}
	inst := instances[0].(map[string]any)
	if inst["InstanceId"] != "i-1234567890abcdef0" {
		t.Fatalf("expected InstanceId i-1234567890abcdef0, got %#v", inst["InstanceId"])
	}
}

func TestAWSEC2_RunInstances(t *testing.T) {
	var gotAction, gotImageId, gotInstanceType, gotMinCount, gotMaxCount string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotImageId = r.Form.Get("ImageId")
		gotInstanceType = r.Form.Get("InstanceType")
		gotMinCount = r.Form.Get("MinCount")
		gotMaxCount = r.Form.Get("MaxCount")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<RunInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-2</requestId>
  <reservationId>r-abcdef1234567890a</reservationId>
  <ownerId>123456789012</ownerId>
  <instancesSet>
    <item>
      <instanceId>i-0abcdef1234567890</instanceId>
      <imageId>ami-0abcdef1234567890</imageId>
      <instanceType>t3.micro</instanceType>
      <instanceState>
        <code>0</code>
        <name>pending</name>
      </instanceState>
    </item>
  </instancesSet>
</RunInstancesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2RunInstances(context.Background(), awsConfig{}, awsEC2Args{
		imageId: "ami-0abcdef1234567890", instanceType: "t3.micro", minCount: 1, maxCount: 1,
	})
	if err != nil {
		t.Fatalf("runInstances: %v", err)
	}
	if gotAction != "RunInstances" {
		t.Fatalf("expected Action=RunInstances, got %q", gotAction)
	}
	if gotImageId != "ami-0abcdef1234567890" {
		t.Fatalf("expected ImageId ami-0abcdef1234567890, got %q", gotImageId)
	}
	if gotInstanceType != "t3.micro" {
		t.Fatalf("expected InstanceType t3.micro, got %q", gotInstanceType)
	}
	if gotMinCount != "1" || gotMaxCount != "1" {
		t.Fatalf("expected MinCount/MaxCount 1/1, got %q/%q", gotMinCount, gotMaxCount)
	}
	m := out.(map[string]any)
	instances, ok := m["Instances"].([]any)
	if !ok || len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %#v", m["Instances"])
	}
}

func TestAWSEC2_TerminateInstances(t *testing.T) {
	var gotAction, gotInstanceId string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotInstanceId = r.Form.Get("InstanceId.1")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<TerminateInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-3</requestId>
  <instancesSet>
    <item>
      <instanceId>i-1234567890abcdef0</instanceId>
      <currentState><code>32</code><name>shutting-down</name></currentState>
      <previousState><code>16</code><name>running</name></previousState>
    </item>
  </instancesSet>
</TerminateInstancesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2TerminateInstances(context.Background(), awsConfig{}, awsEC2Args{instanceIds: []string{"i-1234567890abcdef0"}})
	if err != nil {
		t.Fatalf("terminateInstances: %v", err)
	}
	if gotAction != "TerminateInstances" {
		t.Fatalf("expected Action=TerminateInstances, got %q", gotAction)
	}
	if gotInstanceId != "i-1234567890abcdef0" {
		t.Fatalf("expected InstanceId.1 i-1234567890abcdef0, got %q", gotInstanceId)
	}
	m := out.(map[string]any)
	changes, ok := m["TerminatingInstances"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 state change, got %#v", m["TerminatingInstances"])
	}
}

func TestAWSEC2_StartInstances(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<StartInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-4</requestId>
  <instancesSet>
    <item>
      <instanceId>i-1234567890abcdef0</instanceId>
      <currentState><code>0</code><name>pending</name></currentState>
      <previousState><code>80</code><name>stopped</name></previousState>
    </item>
  </instancesSet>
</StartInstancesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2StartInstances(context.Background(), awsConfig{}, awsEC2Args{instanceIds: []string{"i-1234567890abcdef0"}})
	if err != nil {
		t.Fatalf("startInstances: %v", err)
	}
	if gotAction != "StartInstances" {
		t.Fatalf("expected Action=StartInstances, got %q", gotAction)
	}
	m := out.(map[string]any)
	changes, ok := m["StartingInstances"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 state change, got %#v", m["StartingInstances"])
	}
}

func TestAWSEC2_StopInstances(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<StopInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-5</requestId>
  <instancesSet>
    <item>
      <instanceId>i-1234567890abcdef0</instanceId>
      <currentState><code>64</code><name>stopping</name></currentState>
      <previousState><code>16</code><name>running</name></previousState>
    </item>
  </instancesSet>
</StopInstancesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2StopInstances(context.Background(), awsConfig{}, awsEC2Args{instanceIds: []string{"i-1234567890abcdef0"}})
	if err != nil {
		t.Fatalf("stopInstances: %v", err)
	}
	if gotAction != "StopInstances" {
		t.Fatalf("expected Action=StopInstances, got %q", gotAction)
	}
	m := out.(map[string]any)
	changes, ok := m["StoppingInstances"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("expected 1 state change, got %#v", m["StoppingInstances"])
	}
}

func TestAWSEC2_DescribeVolumes(t *testing.T) {
	var gotAction, gotVolumeId string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		gotVolumeId = r.Form.Get("VolumeId.1")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DescribeVolumesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-6</requestId>
  <volumeSet>
    <item>
      <volumeId>vol-1234567890abcdef0</volumeId>
      <size>8</size>
      <availabilityZone>eu-north-1a</availabilityZone>
      <status>in-use</status>
    </item>
  </volumeSet>
</DescribeVolumesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2DescribeVolumes(context.Background(), awsConfig{}, awsEC2Args{volumeIds: []string{"vol-1234567890abcdef0"}})
	if err != nil {
		t.Fatalf("describeVolumes: %v", err)
	}
	if gotAction != "DescribeVolumes" {
		t.Fatalf("expected Action=DescribeVolumes, got %q", gotAction)
	}
	if gotVolumeId != "vol-1234567890abcdef0" {
		t.Fatalf("expected VolumeId.1 vol-1234567890abcdef0, got %q", gotVolumeId)
	}
	m := out.(map[string]any)
	volumes, ok := m["Volumes"].([]any)
	if !ok || len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %#v", m["Volumes"])
	}
	vol := volumes[0].(map[string]any)
	if size, ok := vol["Size"].(float64); !ok || size != 8 {
		t.Fatalf("expected Size 8, got %#v", vol["Size"])
	}
}

func TestAWSEC2_DescribeAvailabilityZones(t *testing.T) {
	var gotAction string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotAction = r.Form.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DescribeAvailabilityZonesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-7</requestId>
  <availabilityZoneInfo>
    <item>
      <zoneName>eu-north-1a</zoneName>
      <zoneState>available</zoneState>
      <regionName>eu-north-1</regionName>
    </item>
  </availabilityZoneInfo>
</DescribeAvailabilityZonesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2DescribeAvailabilityZones(context.Background(), awsConfig{}, awsEC2Args{})
	if err != nil {
		t.Fatalf("describeAvailabilityZones: %v", err)
	}
	if gotAction != "DescribeAvailabilityZones" {
		t.Fatalf("expected Action=DescribeAvailabilityZones, got %q", gotAction)
	}
	m := out.(map[string]any)
	zones, ok := m["AvailabilityZones"].([]any)
	if !ok || len(zones) != 1 {
		t.Fatalf("expected 1 zone, got %#v", m["AvailabilityZones"])
	}
	zone := zones[0].(map[string]any)
	if zone["ZoneName"] != "eu-north-1a" {
		t.Fatalf("expected ZoneName eu-north-1a, got %#v", zone["ZoneName"])
	}
}

// TestAWSEC2_ErrorPathThrows mirrors TestAWSS3_ErrorPathThrows: proves an EC2
// query-protocol error response is mapped end to end (SDK response -> smithy
// APIError -> mapAWSError) into a structured awsError.
func TestAWSEC2_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Errors>
    <Error>
      <Code>InvalidInstanceID.NotFound</Code>
      <Message>The instance ID 'i-0000000000000000' does not exist</Message>
    </Error>
  </Errors>
  <RequestID>req-err</RequestID>
</Response>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsEC2DescribeInstances(context.Background(), awsConfig{}, awsEC2Args{instanceIds: []string{"i-0000000000000000"}})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.code == "" {
		t.Fatalf("expected non-empty error code, got %q (message=%q)", ae.code, ae.message)
	}
	if ae.status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", ae.status)
	}
	if ae.code != "InvalidInstanceID.NotFound" {
		t.Fatalf("expected code InvalidInstanceID.NotFound, got %q", ae.code)
	}
}

func TestAWSEC2_DescribeInstances_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">
  <requestId>req-js-1</requestId>
  <reservationSet>
    <item>
      <reservationId>r-1234567890abcdef0</reservationId>
      <instancesSet>
        <item>
          <instanceId>i-1234567890abcdef0</instanceId>
          <imageId>ami-0abcdef1234567890</imageId>
          <instanceType>t3.micro</instanceType>
          <instanceState><code>16</code><name>running</name></instanceState>
        </item>
      </instancesSet>
    </item>
  </reservationSet>
</DescribeInstancesResponse>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).ec2().describeInstances();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	reservations, ok := m["Reservations"].([]any)
	if !ok || len(reservations) != 1 {
		t.Fatalf("expected 1 reservation, got %#v", m["Reservations"])
	}
}
