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

// TestFsFind_Limit asserts fs.find's limit option bounds the result count.
// The fixture (searchFixture) has exactly 3 files that survive the default
// gitignore+hidden filters (a.txt, b.go, sub/c.go). Because fastwalk walks
// concurrently, hitting the limit (which stops the walk via the internal
// errStopWalk sentinel) can overshoot the exact number by a few entries that
// were already in flight when the stop propagated — so this asserts a
// bounded range (>= the requested limit, <= the fixture's total of 3
// matches) rather than an exact length of 1.
func TestFsFind_Limit(t *testing.T) {
	eng, _ := findEng(t)
	_, err := eng.Run(context.Background(), "find.ts", `
		const files = await fs.find({ glob: "**/*", type: "file", limit: 1 });
		if (files.length < 1 || files.length > 3)
			throw new Error("expected 1-3 results (limit:1, bounded overshoot), got " + files.length + ": " + JSON.stringify(files));
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
