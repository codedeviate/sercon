package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// A grep over a large synthetic tree must abort promptly when the Run times out.
func TestFsGrep_ContextCancel(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 2000; i++ {
		d := filepath.Join(root, "d"+strconv.Itoa(i%50))
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, strconv.Itoa(i)+".txt"), []byte("no match here\n"), 0o644)
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: root, DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	_ = os.Chdir(root)
	t.Cleanup(func() { _ = os.Chdir(old) })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = eng.Run(ctx, "c.ts", `await fs.grep({ pattern: "zzz-never" });`)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("grep did not honor cancellation promptly: %v", elapsed)
	}
}
