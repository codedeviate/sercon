package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAzureRG_List(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"rg1","location":"eastus"},{"name":"rg2","location":"westus"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureRGList(context.Background(), azureConfig{subscriptionID: "sub"}, azureRGArgs{})
	if err != nil {
		t.Fatalf("azureRGList error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 2 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["name"] != "rg1" || first["location"] != "eastus" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
	second, ok := vals[1].(map[string]any)
	if !ok || second["name"] != "rg2" || second["location"] != "westus" {
		t.Fatalf("unexpected second element: %#v", vals[1])
	}
}

func TestAzureRG_Get(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"rg1","location":"eastus","id":"/subscriptions/sub/resourceGroups/rg1"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureRGGet(context.Background(), azureConfig{subscriptionID: "sub"}, azureRGArgs{name: "rg1"})
	if err != nil {
		t.Fatalf("azureRGGet error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	if m["name"] != "rg1" || m["location"] != "eastus" {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
}

func TestAzureRG_Create(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"rg1","location":"eastus"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureRGCreate(context.Background(), azureConfig{subscriptionID: "sub"}, azureRGArgs{name: "rg1", location: "eastus"})
	if err != nil {
		t.Fatalf("azureRGCreate error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	if m["name"] != "rg1" || m["location"] != "eastus" {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	if gotBody == "" {
		t.Fatal("expected a request body to have been sent")
	}
}

func TestAzureRG_Delete(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureRGDelete(context.Background(), azureConfig{subscriptionID: "sub"}, azureRGArgs{name: "rg1"})
	if err != nil {
		t.Fatalf("azureRGDelete error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

func TestAzureResourceGroups_List_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"rg1","location":"eastus"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	got := runCloudAzureScript(t, `
		const az = cloud.azure({ subscriptionId: "sub-guid" });
		const __result = await az.resourceGroups().list();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T: %#v", got, got)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["name"] != "rg1" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

func TestAzureResourceGroups_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ResourceGroupNotFound","message":"Resource group 'missing' could not be found."}}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	_, err := azureRGGet(context.Background(), azureConfig{subscriptionID: "sub"}, azureRGArgs{name: "missing"})
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	ae, ok := err.(azureError)
	if !ok {
		t.Fatalf("expected azureError, got %T: %v", err, err)
	}
	if ae.status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", ae.status)
	}
	if ae.code != "ResourceGroupNotFound" {
		t.Fatalf("expected code ResourceGroupNotFound, got %q", ae.code)
	}
}
