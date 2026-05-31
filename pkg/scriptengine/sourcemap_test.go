package scriptengine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestSourceMap_ModuleThrowMapsToTSLine: an imported module throws from a
// function defined BELOW an `enum` (which esbuild lowers into several JS
// lines, so the JS line != the TS line). The mapped stack frame must show the
// module's TS line, proving real source mapping rather than coincidence.
//
// boom.ts layout (1-based TS lines):
//
//	1: enum E { A, B }
//	2: export function boom(): void { throw new Error("mod-boom"); }
func TestSourceMap_ModuleThrowMapsToTSLine(t *testing.T) {
	dir := t.TempDir()
	mod := "enum E { A, B }\nexport function boom(): void { throw new Error(\"mod-boom\"); }\n"
	if err := os.WriteFile(filepath.Join(dir, "boom.ts"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"),
		`import { boom } from "./boom.ts";
boom();`)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom.ts:2") {
		t.Errorf("expected mapped module frame boom.ts:2, got:\n%s", err.Error())
	}
}

// TestSourceMap_EntryThrowMapsToTSLine: the entry script throws from a line
// BELOW an enum (esbuild lowers it, shifting JS lines). The mapped frame must
// show the entry's TS line.
//
// main.ts layout (1-based):
//
//	1: enum E { A, B, C }
//	2: const x: E = E.B;
//	3: if (x === E.B) throw new Error("entry-boom");
func TestSourceMap_EntryThrowMapsToTSLine(t *testing.T) {
	dir := t.TempDir()
	src := "enum E { A, B, C }\nconst x: E = E.B;\nif (x === E.B) throw new Error(\"entry-boom\");\n"
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"), src)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "entry-boom") {
		t.Errorf("missing message: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "main.ts:3") {
		t.Errorf("expected mapped entry frame main.ts:3, got:\n%s", err.Error())
	}
}

// TestSourceMap_NoImportEntryMapped exercises the shift>0 path: with no imports
// the prefix is just the IIFE opener, so the body is pushed down relative to
// esbuild's output and the map must be shifted to compensate.
func TestSourceMap_NoImportEntryMapped(t *testing.T) {
	dir := t.TempDir()
	src := "enum E { A, B }\nthrow new Error(\"noimp-boom\");\n" // throw on TS line 2
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"), src)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "main.ts:2") {
		t.Errorf("expected mapped frame main.ts:2, got:\n%s", err.Error())
	}
}
