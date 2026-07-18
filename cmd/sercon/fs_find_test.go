package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// findEng builds an engine with the CLI surface and a fixture cwd.
func findEng(t *testing.T) (*scriptengine.Engine, string) {
	t.Helper()
	root := searchFixture(t)
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: root, DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	// Run from the fixture dir so relative results are stable.
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return eng, root
}

func TestFsFind_GlobDefaults(t *testing.T) {
	eng, _ := findEng(t)
	_, err := eng.Run(context.Background(), "find.ts", `
		const files = await fs.find({ glob: "**/*.go" });
		const joined = files.slice().sort().join(",");
		if (joined !== "b.go,sub/c.go") throw new Error("got: " + joined);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestFsFind_StatShape(t *testing.T) {
	eng, root := findEng(t)
	_ = filepath.Join(root, "a.txt")
	_, err := eng.Run(context.Background(), "find.ts", `
		const rows = await fs.find({ glob: "a.txt", stat: true });
		if (rows.length !== 1) throw new Error("len " + rows.length);
		const r = rows[0];
		if (r.type !== "file" || typeof r.size !== "number" || typeof r.mtimeMs !== "number")
			throw new Error("shape " + JSON.stringify(r));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	_ = strings.TrimSpace
}
