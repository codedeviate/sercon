package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runDumpScript(t *testing.T, script string) string {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := eng.Register("__record", func(call goja.FunctionCall) goja.Value {
		got = call.Argument(0).String()
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "dump.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
	return got
}

func TestCodecPHP_EndToEnd(t *testing.T) {
	got := runDumpScript(t, `
		const v = { name: "Al", age: 30, tags: ["x","y"] };
		const s = codec.php.serialize(v);
		const back = codec.php.unserialize(s);
		__record(s + "|" + JSON.stringify(back));
	`)
	want := `a:3:{s:4:"name";s:2:"Al";s:3:"age";i:30;s:4:"tags";a:2:{i:0;s:1:"x";i:1;s:1:"y";}}` +
		`|{"name":"Al","age":30,"tags":["x","y"]}`
	if got != want {
		t.Fatalf("php round-trip:\n got: %s\nwant: %s", got, want)
	}
}

func TestCodecPHP_ClassSentinel(t *testing.T) {
	got := runDumpScript(t, `
		const s = codec.php.serialize({ __class: "Point", x: 1, y: 2 });
		__record(s + "|" + JSON.stringify(codec.php.unserialize(s)));
	`)
	want := `O:5:"Point":2:{s:1:"x";i:1;s:1:"y";i:2;}|{"__class":"Point","x":1,"y":2}`
	if got != want {
		t.Fatalf("php object:\n got: %s\nwant: %s", got, want)
	}
}

func TestCodecPHP_VarExportAndVarDump(t *testing.T) {
	// Both directions reachable from a script; round-trip via the engine.
	got := runDumpScript(t, `
		const v = { a: 1, b: [2,3] };
		const ve = codec.php.parseVarExport(codec.php.varExport(v));
		const vd = codec.php.parseVarDump(codec.php.varDump(v));
		__record(JSON.stringify(ve) + "|" + JSON.stringify(vd));
	`)
	want := `{"a":1,"b":[2,3]}|{"a":1,"b":[2,3]}`
	if got != want {
		t.Fatalf("php export/dump:\n got: %s\nwant: %s", got, want)
	}
}

func TestCodecPerl_BoolEndToEnd(t *testing.T) {
	got := runDumpScript(t, `
		const s = codec.perl.dumper(true);
		__record(s + "|" + codec.perl.parseDumper(s));
	`)
	want := `$VAR1 = bless( do{\(my $o = 1)}, 'JSON::XS::Boolean' );|true`
	if got != want {
		t.Fatalf("perl bool:\n got: %s\nwant: %s", got, want)
	}
}

func TestCodecPerl_RoundTrip(t *testing.T) {
	got := runDumpScript(t, `
		const v = { name: "Al", nums: [1,2,3] };
		__record(JSON.stringify(codec.perl.parseDumper(codec.perl.dumper(v))));
	`)
	if want := `{"name":"Al","nums":[1,2,3]}`; got != want {
		t.Fatalf("perl round-trip:\n got: %s\nwant: %s", got, want)
	}
}

func TestCodecClassKeyOption(t *testing.T) {
	// opts.classKey overrides the sentinel property name.
	got := runDumpScript(t, `
		const s = codec.php.serialize({ "@type": "Point", x: 1 }, { classKey: "@type" });
		__record(s);
	`)
	if want := `O:5:"Point":1:{s:1:"x";i:1;}`; got != want {
		t.Fatalf("classKey option:\n got: %s\nwant: %s", got, want)
	}
}

func TestCodec_CycleThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "c.ts", `
		const a = {}; a.self = a; codec.php.serialize(a);
	`)
	if err == nil {
		t.Fatal("expected thrown circular-reference error")
	}
}
