package main

import (
	"context"
	"os"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func grepEng(t *testing.T) *scriptengine.Engine {
	t.Helper()
	root := searchFixture(t) // a.txt "alpha", b.go "package main", sub/c.go "package sub"
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

func TestFsGrep_Matches(t *testing.T) {
	eng := grepEng(t)
	_, err := eng.Run(context.Background(), "grep.ts", `
		const hits = await fs.grep({ pattern: "package", glob: "**/*.go" });
		if (hits.length !== 2) throw new Error("len " + hits.length);
		const h = hits.find(x => x.path === "b.go");
		if (!h || h.line !== 1 || h.match !== "package") throw new Error(JSON.stringify(h));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestFsGrepFilesAndCount(t *testing.T) {
	eng := grepEng(t)
	_, err := eng.Run(context.Background(), "grep.ts", `
		const files = await fs.grepFiles({ pattern: "package", glob: "**/*.go" });
		if (files.slice().sort().join(",") !== "b.go,sub/c.go") throw new Error(files.join(","));
		const counts = await fs.grepCount({ pattern: "package", glob: "**/*.go" });
		if (counts.length !== 2 || counts.some(c => c.count !== 1)) throw new Error(JSON.stringify(counts));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}
