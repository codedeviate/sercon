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
