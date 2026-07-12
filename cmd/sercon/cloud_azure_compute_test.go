package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAzureCompute_List(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"vm1","location":"eastus"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputeList(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1"})
	if err != nil {
		t.Fatalf("azureComputeList error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["name"] != "vm1" || first["location"] != "eastus" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

func TestAzureCompute_List_AllSubscription(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"vm1","location":"eastus"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputeList(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{})
	if err != nil {
		t.Fatalf("azureComputeList error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	if _, ok := m["value"].([]any); !ok {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	if gotPath == "" {
		t.Fatal("expected a request to have been made")
	}
}

func TestAzureCompute_Get(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"vm1","location":"eastus","id":"/subscriptions/sub/resourceGroups/rg1/providers/Microsoft.Compute/virtualMachines/vm1"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputeGet(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1", name: "vm1"})
	if err != nil {
		t.Fatalf("azureComputeGet error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	if m["name"] != "vm1" || m["location"] != "eastus" {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
}

func TestAzureCompute_Start(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputeStart(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1", name: "vm1"})
	if err != nil {
		t.Fatalf("azureComputeStart error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

func TestAzureCompute_PowerOff(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputePowerOff(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1", name: "vm1"})
	if err != nil {
		t.Fatalf("azureComputePowerOff error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

func TestAzureCompute_Deallocate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputeDeallocate(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1", name: "vm1"})
	if err != nil {
		t.Fatalf("azureComputeDeallocate error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

func TestAzureCompute_Delete(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureComputeDelete(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1", name: "vm1"})
	if err != nil {
		t.Fatalf("azureComputeDelete error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

func TestAzureCompute_ListVirtualMachines_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"vm1","location":"eastus"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	got := runCloudAzureScript(t, `
		const az = cloud.azure({ subscriptionId: "sub-guid" });
		const __result = await az.compute().listVirtualMachines({ resourceGroup: "rg1" });
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
	if !ok || first["name"] != "vm1" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

func TestAzureCompute_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ResourceNotFound","message":"VM 'missing' could not be found."}}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	_, err := azureComputeGet(context.Background(), azureConfig{subscriptionID: "sub"}, azureComputeArgs{resourceGroup: "rg1", name: "missing"})
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
	if ae.code != "ResourceNotFound" {
		t.Fatalf("expected code ResourceNotFound, got %q", ae.code)
	}
}
