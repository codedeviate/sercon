package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompute_ListInstances(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"vm-a"},{"name":"vm-b"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeListInstances(context.Background(), googleConfig{}, computeArgs{project: "p", zone: "z"})
	if err != nil {
		t.Fatalf("listInstances: %v", err)
	}
	m := out.(map[string]any)
	items, ok := m["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 instances, got %#v", m["items"])
	}
}

func TestCompute_GetInstance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"vm-a","status":"RUNNING"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeGetInstance(context.Background(), googleConfig{}, computeArgs{project: "p", zone: "z", name: "vm-a"})
	if err != nil {
		t.Fatalf("getInstance: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "vm-a" {
		t.Fatalf("expected name vm-a, got %#v", m["name"])
	}
}

func TestCompute_CreateInstance(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"op-1","status":"RUNNING"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeCreateInstance(context.Background(), googleConfig{}, computeArgs{
		project: "p", zone: "z",
		instance: map[string]any{"name": "vm-a", "machineType": "n1-standard-1"},
	})
	if err != nil {
		t.Fatalf("createInstance: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "op-1" {
		t.Fatalf("expected name op-1, got %#v", m["name"])
	}
	if gotBody["name"] != "vm-a" {
		t.Fatalf("expected uploaded instance name vm-a, got %#v", gotBody["name"])
	}
}

func TestCompute_DeleteInstance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"op-2","status":"DONE"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeDeleteInstance(context.Background(), googleConfig{}, computeArgs{project: "p", zone: "z", name: "vm-a"})
	if err != nil {
		t.Fatalf("deleteInstance: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "op-2" {
		t.Fatalf("expected name op-2, got %#v", m["name"])
	}
}

func TestCompute_StartInstance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"op-3","status":"RUNNING"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeStartInstance(context.Background(), googleConfig{}, computeArgs{project: "p", zone: "z", name: "vm-a"})
	if err != nil {
		t.Fatalf("startInstance: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "op-3" {
		t.Fatalf("expected name op-3, got %#v", m["name"])
	}
}

func TestCompute_StopInstance(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"op-4","status":"DONE"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeStopInstance(context.Background(), googleConfig{}, computeArgs{project: "p", zone: "z", name: "vm-a"})
	if err != nil {
		t.Fatalf("stopInstance: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "op-4" {
		t.Fatalf("expected name op-4, got %#v", m["name"])
	}
}

func TestCompute_ListZones(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"europe-north1-a"},{"name":"europe-north1-b"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeListZones(context.Background(), googleConfig{}, computeArgs{project: "p"})
	if err != nil {
		t.Fatalf("listZones: %v", err)
	}
	m := out.(map[string]any)
	items, ok := m["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 zones, got %#v", m["items"])
	}
}

func TestCompute_ListDisks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"disk-a"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := computeListDisks(context.Background(), googleConfig{}, computeArgs{project: "p", zone: "z"})
	if err != nil {
		t.Fatalf("listDisks: %v", err)
	}
	m := out.(map[string]any)
	items, ok := m["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 disk, got %#v", m["items"])
	}
}

// TestCompute_ListInstances_ViaJS exercises the full script path (cloud
// namespace registration, the compute() accessor's .Func unwrap,
// computeExtract's goja .Export of the options object, and Promise
// resolution via await) — the same pattern TestStorage_ListBuckets_ViaJS
// established for Task 5.
func TestCompute_ListInstances_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"vm-a"},{"name":"vm-b"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	got := runCloudScript(t, `
		const c = await cloud.google({ project: "p" }).compute().listInstances({ project: "p", zone: "z" });
		const __result = c;
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T (%#v)", got, got)
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 instances, got %#v", m["items"])
	}
}
