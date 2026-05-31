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
