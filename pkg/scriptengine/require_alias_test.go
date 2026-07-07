package scriptengine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// A module required via different specifiers that resolve to the SAME file
// (bare "./counter" vs explicit "./counter.ts") must yield one shared
// module instance — its top-level state must not fork. goja_nodejs keys its
// module cache on the pre-extension-fallback path, so without path
// canonicalization the two specifiers produce two instances and the counter
// resets. MANUAL §8.3/§11 promise a single instance per Run.
func TestRun_RequireExtensionAliasSharesInstance(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("counter.ts", "let n = 0;\nexport function inc(): number { return ++n; }\n")

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	var first, second int64
	if err := eng.Register("record", func(a, b int64) { first, second = a, b }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"), `
const a = require("./counter").inc();       // 1
const b = require("./counter.ts").inc();    // 2 iff the module is shared
record(a, b);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if first != 1 {
		t.Fatalf("first inc() = %d, want 1", first)
	}
	if second != 2 {
		t.Fatalf("second inc() = %d, want 2 (module forked: counter.ts executed twice)", second)
	}
}

// The directory form ("./mod") and its explicit index file
// ("./mod/index.ts") must likewise share one instance.
func TestRun_RequireDirIndexAliasSharesInstance(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mod", "index.ts"),
		[]byte("let n = 0;\nexport function inc(): number { return ++n; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	var first, second int64
	if err := eng.Register("record", func(a, b int64) { first, second = a, b }); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"), `
const a = require("./mod").inc();            // 1
const b = require("./mod/index.ts").inc();   // 2 iff shared
record(a, b);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if first != 1 || second != 2 {
		t.Fatalf("dir/index alias: got (%d,%d), want (1,2)", first, second)
	}
}
