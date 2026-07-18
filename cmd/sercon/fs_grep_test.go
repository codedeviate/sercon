package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestFsGrep_TypePreset(t *testing.T) {
	eng := grepEng(t)
	_, err := eng.Run(context.Background(), "grep.ts", `
		const hits = await fs.grep({ pattern: "package", type: "go" });
		if (hits.length !== 2) throw new Error("type:go should match b.go + sub/c.go, got " + hits.length);
		const paths = hits.map(h => h.path).sort().join(",");
		if (paths !== "b.go,sub/c.go") throw new Error("paths " + paths);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestFsGrep_ExplicitPaths_Relative covers the ordinary case: a relative
// `paths:` entry read and displayed correctly (default absolute:false).
func TestFsGrep_ExplicitPaths_Relative(t *testing.T) {
	eng := grepEng(t) // cwd == fixture root; b.go contains "package main\n"
	_, err := eng.Run(context.Background(), "grep.ts", `
		const hits = await fs.grep({ pattern: "package", paths: ["b.go"] });
		if (hits.length !== 1) throw new Error("len " + hits.length);
		if (hits[0].path !== "b.go") throw new Error("path " + hits[0].path);
		if (hits[0].match !== "package") throw new Error("match " + hits[0].match);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestFsGrep_ExplicitPaths_AbsoluteOption falsifies the absPath() no-op this
// test guards against: with `absolute: true`, a *relative* `paths:` entry
// must be reported back as an absolute path. The old absPath(p) simply
// returned p unchanged, so relDisplay(abs, true) — which trusts its input is
// already absolute and returns it verbatim — would have leaked the relative
// "b.go" straight through instead of the resolved absolute path.
func TestFsGrep_ExplicitPaths_AbsoluteOption(t *testing.T) {
	eng := grepEng(t) // cwd == fixture root; b.go contains "package main\n"
	// Resolve via os.Getwd() (not the raw t.TempDir() value) so the
	// expectation survives any symlink normalization (e.g. macOS /tmp ->
	// /private/tmp) the engine's own os.Getwd()-based resolution applies too.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, "b.go")
	script := fmt.Sprintf(`
		const hits = await fs.grep({ pattern: "package", paths: ["b.go"], absolute: true });
		if (hits.length !== 1) throw new Error("len " + hits.length);
		if (hits[0].path !== %q) throw new Error("path " + hits[0].path);
	`, want)
	if _, err := eng.Run(context.Background(), "grep.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestFsGrep_ExplicitPaths_Absolute passes an absolute `paths:` entry from a
// script whose cwd is NOT that file's directory, so a correct read requires
// absPath to resolve the (already-absolute) path rather than depend on cwd.
func TestFsGrep_ExplicitPaths_Absolute(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "sub", "hit.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("package hit\n// TODO: fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir() // deliberately different cwd from `target`'s dir
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: elsewhere, DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	script := fmt.Sprintf(`
		const hits = await fs.grep({ pattern: "TODO", fixed: true, paths: [%q] });
		if (hits.length !== 1) throw new Error("len " + hits.length);
		if (hits[0].line !== 2) throw new Error("line " + hits[0].line);
	`, target)
	if _, err := eng.Run(context.Background(), "grep.ts", script); err != nil {
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
