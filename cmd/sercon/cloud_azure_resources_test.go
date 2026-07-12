package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAzureResources_ListByResourceGroup(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"res1","type":"Microsoft.Storage/storageAccounts"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureResourcesListByResourceGroup(context.Background(), azureConfig{subscriptionID: "sub"}, azureResourcesArgs{resourceGroup: "rg1"})
	if err != nil {
		t.Fatalf("azureResourcesListByResourceGroup error: %v", err)
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
	if !ok || first["name"] != "res1" || first["type"] != "Microsoft.Storage/storageAccounts" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

func TestAzureResources_GetById(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"res1","type":"Microsoft.Storage/storageAccounts","location":"eastus"}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureResourcesGetById(context.Background(), azureConfig{subscriptionID: "sub"}, azureResourcesArgs{
		resourceId: "/subscriptions/sub/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/res1",
		apiVersion: "2021-04-01",
	})
	if err != nil {
		t.Fatalf("azureResourcesGetById error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	if m["name"] != "res1" || m["type"] != "Microsoft.Storage/storageAccounts" || m["location"] != "eastus" {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
}

func TestAzureResources_ListByResourceGroup_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"name":"res1","type":"Microsoft.Storage/storageAccounts"}]}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	got := runCloudAzureScript(t, `
		const az = cloud.azure({ subscriptionId: "sub-guid" });
		const __result = await az.resources().listByResourceGroup({ resourceGroup: "rg1" });
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
	if !ok || first["name"] != "res1" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

func TestAzureResources_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ResourceNotFound","message":"Resource 'missing' could not be found."}}`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	_, err := azureResourcesGetById(context.Background(), azureConfig{subscriptionID: "sub"}, azureResourcesArgs{
		resourceId: "/subscriptions/sub/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/missing",
		apiVersion: "2021-04-01",
	})
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
