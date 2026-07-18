package main

import (
	"context"
	"os"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func streamEng(t *testing.T) *scriptengine.Engine {
	t.Helper()
	root := searchFixture(t)
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: root, DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return eng
}

func TestFsFind_Stream(t *testing.T) {
	eng := streamEng(t)
	_, err := eng.Run(context.Background(), "s.ts", `
		const got = [];
		for await (const p of fs.find({ glob: "**/*.go", stream: true })) got.push(p);
		if (got.slice().sort().join(",") !== "b.go,sub/c.go") throw new Error(got.join(","));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestFsGrep_Stream(t *testing.T) {
	eng := streamEng(t)
	_, err := eng.Run(context.Background(), "s.ts", `
		const got = [];
		for await (const m of fs.grep({ pattern: "package", glob: "**/*.go", stream: true })) got.push(m.path);
		if (got.slice().sort().join(",") !== "b.go,sub/c.go") throw new Error(got.join(","));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestFsFind_Stream_BreakEarly verifies that breaking out of a for-await loop
// early (before the producer drains) does not deadlock or leak the producer
// goroutine — the iterator's return() must cancel the walk and release the
// HoldRun sentinel.
func TestFsFind_Stream_BreakEarly(t *testing.T) {
	eng := streamEng(t)
	_, err := eng.Run(context.Background(), "s.ts", `
		for await (const p of fs.find({ glob: "**/*.go", stream: true })) {
			break;
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}
