package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// fcall builds a goja.FunctionCall from already-constructed values, for
// driving the extract halves. The work halves take plain Go values and are
// called directly.
func fcall(args ...goja.Value) goja.FunctionCall { return goja.FunctionCall{Arguments: args} }

func TestFileWriteReadText(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "note.txt")

	res, err := fileWriteText(ctx, fileWriteTextArgs{path: p, data: []byte("héllo ✨")})
	if err != nil {
		t.Fatalf("writeText: %v", err)
	}
	if res["bytes"].(int) != len([]byte("héllo ✨")) {
		t.Fatalf("bytes = %v", res["bytes"])
	}
	got, err := fileReadText(ctx, p)
	if err != nil || got != "héllo ✨" {
		t.Fatalf("readText got %q err %v", got, err)
	}
}

func TestFileWriteReadBytes(t *testing.T) {
	vm := goja.New()
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "blob.bin")
	ua, err := vm.RunString("new Uint8Array([1,2,3,4,255])")
	if err != nil {
		t.Fatal(err)
	}
	// The Uint8Array → []byte coercion lives in the extract half; run it
	// for real, then hand the plain-Go bundle to the work half.
	args, err := fileWriteBytesExtract(fcall(vm.ToValue(p), ua))
	if err != nil {
		t.Fatalf("writeBytes extract: %v", err)
	}
	if _, err := fileWriteBytes(ctx, args); err != nil {
		t.Fatalf("writeBytes: %v", err)
	}
	got, err := fileReadBytes(ctx, p)
	if err != nil {
		t.Fatalf("readBytes: %v", err)
	}
	want := []byte{1, 2, 3, 4, 255}
	if len(got) != len(want) {
		t.Fatalf("readBytes len %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("byte %d = %d want %d", i, got[i], want[i])
		}
	}
}

func TestFileWriteBytesRejectsNonUint8(t *testing.T) {
	vm := goja.New()
	p := filepath.Join(t.TempDir(), "x.bin")
	if _, err := fileWriteBytesExtract(fcall(vm.ToValue(p), vm.ToValue("not bytes"))); err == nil {
		t.Fatal("expected error for non-Uint8Array data")
	}
}

func TestFileWriteFailsMissingParent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope", "deep", "x.txt")
	if _, err := fileWriteText(context.Background(), fileWriteTextArgs{path: p, data: []byte("x")}); err == nil {
		t.Fatal("writeText should fail when parent dir is missing")
	}
}

func TestFileMkdirNestedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "a", "b", "c")
	if _, err := fileMkdir(ctx, p); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := fileMkdir(ctx, p); err != nil {
		t.Fatalf("mkdir idempotent: %v", err)
	}
	if _, err := fileWriteText(ctx, fileWriteTextArgs{path: filepath.Join(p, "f.txt"), data: []byte("ok")}); err != nil {
		t.Fatalf("write under mkdir: %v", err)
	}
}

func TestFileExists(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	p := filepath.Join(dir, "here.txt")
	if ok, err := fileExists(ctx, p); err != nil || ok {
		t.Fatalf("absent path should be (false, nil): ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := fileExists(ctx, p); err != nil || !ok {
		t.Fatalf("should exist: ok=%v err=%v", ok, err)
	}
}

func TestFileRemove(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f := filepath.Join(dir, "f.txt")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	if _, err := fileRemove(ctx, f); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("file not removed")
	}
	sub := filepath.Join(dir, "tree", "deep")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "g.txt"), []byte("y"), 0o644)
	if _, err := fileRemove(ctx, filepath.Join(dir, "tree")); err != nil {
		t.Fatalf("remove tree: %v", err)
	}
	if _, err := fileRemove(ctx, filepath.Join(dir, "ghost")); err != nil {
		t.Fatalf("remove absent should be no-op: %v", err)
	}
}

func TestFileStat(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	p := filepath.Join(dir, "s.txt")
	_ = os.WriteFile(p, []byte("12345"), 0o644)
	st, err := fileStat(ctx, p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st["size"].(int64) != 5 || st["isDir"].(bool) {
		t.Fatalf("stat = %v", st)
	}
	if _, ok := st["modifiedMs"].(int64); !ok {
		t.Fatalf("modifiedMs missing/typed wrong: %v", st["modifiedMs"])
	}
	if _, err := fileStat(ctx, filepath.Join(dir, "absent")); err == nil {
		t.Fatal("stat of absent should error")
	}
}

func TestFilePathRequired(t *testing.T) {
	// The path check lives in the extract half (filePathExtract).
	if _, err := filePathExtract("readText")(fcall()); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected 'path is required', got %v", err)
	}
}
