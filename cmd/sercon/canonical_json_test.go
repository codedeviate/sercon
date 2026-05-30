package main

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// cjOrder has fields in a deliberately non-alphabetical order to prove goja
// (with the engine's json TagFieldNameMapper) enumerates struct fields in
// DECLARATION order, not sorted and not Go-map-randomized. This is the
// invariant the canonical-JSON sweep relies on: returning a json-tagged
// struct instead of a map[string]any yields byte-stable JSON.stringify output.
type cjOrder struct {
	Zebra string `json:"zebra"`
	Alpha int    `json:"alpha"`
	Mid   bool   `json:"mid"`
}

func TestStructResult_StableKeyOrder(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.Register("probe", func() cjOrder { return cjOrder{"z", 1, true} }); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := eng.Register("__rec", func(call goja.FunctionCall) goja.Value {
		got = call.Argument(0).String()
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `__rec(JSON.stringify(probe()))`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := `{"zebra":"z","alpha":1,"mid":true}`; got != want {
		t.Fatalf("struct key order (want declaration order, json-tag names):\n got: %s\nwant: %s", got, want)
	}
}

// TestNetProbeTCP_KeyOrder drives the real net.probe.tcp binding against a
// local listener and asserts its result object's keys are in a fixed order —
// the canonical-JSON guarantee for the (now struct-backed) probe result.
func TestNetProbeTCP_KeyOrder(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := eng.Register("__rec", func(call goja.FunctionCall) goja.Value {
		got = call.Argument(0).String()
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`const r = await net.probe.tcp(%q); __rec(Object.keys(r).join(","));`, ln.Addr().String())
	if _, err := eng.Run(context.Background(), "probe.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "host,port,ip,latencyMs"; got != want {
		t.Fatalf("net.probe.tcp key order:\n got: %s\nwant: %s", got, want)
	}
}

// TestSQLRows_StableColumnOrder proves a db.sqlite query row (built via the
// shared Ordered-backed scanRows) keeps the result-set column order — the
// dynamic-key half of the canonical-JSON sweep. Uses an in-memory DB (no
// network), so it's deterministic.
func TestSQLRows_StableColumnOrder(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := eng.Register("__rec", func(call goja.FunctionCall) goja.Value {
		got = call.Argument(0).String()
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "rows.ts", `
		const d = await db.sqlite.open(":memory:");
		await d.exec("CREATE TABLE t (zebra TEXT, alpha INTEGER, mid TEXT)");
		await d.exec("INSERT INTO t VALUES ('z', 1, 'm')");
		const rows = await d.query("SELECT zebra, alpha, mid FROM t");
		await d.close();
		__rec(JSON.stringify(rows[0]));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := `{"zebra":"z","alpha":1,"mid":"m"}`; got != want {
		t.Fatalf("sqlite row key order:\n got: %s\nwant: %s", got, want)
	}
}
