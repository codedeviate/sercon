package scriptengine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestEntry_ImportAfterHelperPreamble: an entry that imports AND declares a
// top-level function makes esbuild emit a `var __name`/`var __defProp`
// preamble before the import. The rewriter must still convert the import
// (previously it broke at the preamble, leaking a raw ESM import → goja
// SyntaxError). Combined with export-default capture, Run returns f()'s value.
func TestEntry_ImportAfterHelperPreamble(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.ts"),
		[]byte("export const x = 7;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	val, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"),
		"import { x } from \"./m.ts\";\nfunction f() { return x; }\nexport default f();\n")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if val == nil || val.ToInteger() != 7 {
		t.Errorf("expected Run to return 7, got %v", val)
	}
}

// TestExportDefault_Number: export default of a number is captured as Run's value.
func TestExportDefault_Number(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	val, err := eng.Run(context.Background(), "main.ts", "export default 6 * 7;\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == nil || val.ToInteger() != 42 {
		t.Errorf("expected 42, got %v", val)
	}
}

// TestExportDefault_Object: export default of an object exports to a Go map.
func TestExportDefault_Object(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	val, err := eng.Run(context.Background(), "main.ts", "export default { a: 1, b: 2 };\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := val.Export().(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T (%v)", val.Export(), val.Export())
	}
	if m["a"] != int64(1) && m["a"] != float64(1) {
		t.Errorf("a: got %v", m["a"])
	}
}

// TestExportConst_NoDefault_NoCrash: a named export with no default no longer
// crashes (the export block is stripped); Run resolves undefined.
func TestExportConst_NoDefault_NoCrash(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	val, err := eng.Run(context.Background(), "main.ts", "export const y = 1;\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil && !goja.IsUndefined(val) && val.Export() != nil {
		t.Errorf("expected no captured value, got %v", val.Export())
	}
}

// TestExportDefault_TemplateLooksLikeExport is a regression guard: a script
// whose multi-line template literal contains a line that trims to "export {"
// must not confuse the export-block stripper (which scans from the end, since
// esbuild always emits the real export block as the trailing statement). The
// real default export must still be captured and the template preserved.
func TestExportDefault_TemplateLooksLikeExport(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	src := "const s = `\nexport {\n`;\nexport default s.trim();\n"
	val, err := eng.Run(context.Background(), "main.ts", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val == nil || val.String() != "export {" {
		t.Errorf("expected template value \"export {\", got %v", val)
	}
}
