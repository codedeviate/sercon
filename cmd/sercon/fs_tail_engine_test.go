package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func tailEng(t *testing.T) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true, Timeout: 10 * time.Second})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	return eng
}

// fs.tail follows appended lines; a Go goroutine appends after the script starts
// following, and the script collects two lines then breaks.
func TestFsTail_FollowsAppends(t *testing.T) {
	eng := tailEng(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		for _, s := range []string{"alpha\n", "beta\n"} {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
			_, _ = f.WriteString(s)
			_ = f.Close()
			time.Sleep(30 * time.Millisecond)
		}
	}()
	_, err := eng.Run(context.Background(), "tail.ts", `
		const got = [];
		for await (const line of fs.tail(`+"`"+path+"`"+`)) {
			got.push(line);
			if (got.length === 2) break;
		}
		if (got.join(",") !== "alpha,beta") throw new Error("got: " + got.join(","));
	`)
	_ = err
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// fs.grepStream follows a file and yields only matching lines (Go-side match).
func TestFsGrepStream_MatchesOnly(t *testing.T) {
	eng := tailEng(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		for _, s := range []string{"info: ok\n", "ERROR: boom\n", "info: fine\n", "ERROR: again\n"} {
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
			_, _ = f.WriteString(s)
			_ = f.Close()
			time.Sleep(30 * time.Millisecond)
		}
	}()
	_, err := eng.Run(context.Background(), "gs.ts", `
		const hits = [];
		for await (const m of fs.grepStream(`+"`"+path+"`"+`, { pattern: "ERROR", fixed: true })) {
			hits.push({ line: m.line, column: m.column, text: m.text });
			if (hits.length === 2) break;
		}
		const got = hits.map(h => h.line + ":" + h.column + ":" + h.text).join("|");
		// Session-relative line counter (all 4 lines observed; matches are #2 and #4)
		// and 1-based rune column ("ERROR" starts each line, so column 1).
		if (got !== "2:1:ERROR: boom|4:1:ERROR: again") throw new Error("got: " + got);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// fs.grepStream throws synchronously on a missing pattern and on a missing file.
func TestFsGrepStream_ThrowsOnMissingPatternAndFile(t *testing.T) {
	eng := tailEng(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "real.log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "gs.ts", `
		let noPattern = false;
		// A real file, but no pattern → the pattern check must fire.
		try { fs.grepStream(`+"`"+path+"`"+`, {}); } catch (e) { noPattern = true; }
		if (!noPattern) throw new Error("expected fs.grepStream to throw when pattern is missing");
		let noFile = false;
		try { fs.grepStream("/no/such/file/here.log", { pattern: "x" }); } catch (e) { noFile = true; }
		if (!noFile) throw new Error("expected fs.grepStream to throw for a missing file");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// A missing file throws synchronously (spec: no wait-for-create in v1).
func TestFsTail_MissingFileThrows(t *testing.T) {
	eng := tailEng(t)
	_, err := eng.Run(context.Background(), "tail.ts", `
		let threw = false;
		try { fs.tail("/no/such/file/here.log"); } catch (e) { threw = true; }
		if (!threw) throw new Error("expected fs.tail to throw for a missing file");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}
