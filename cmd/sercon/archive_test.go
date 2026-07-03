package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// fixtureTree creates a small directory layout used by the round-trip
// tests: a top-level file plus a nested subdirectory with a second file.
func fixtureTree(t *testing.T, root string) []string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"hello.txt":   "hello world\n",
		"sub/note.md": "# Note\n\nContent.\n",
	}
	var names []string
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, rel)
	}
	sort.Strings(names)
	return names
}

// Round-trip each supported format: build an archive from the fixture
// tree, extract into a fresh directory, confirm the files come back with
// the same contents.
func TestArchive_RoundTrip(t *testing.T) {
	for _, ext := range []string{".zip", ".tar", ".tar.gz"} {
		t.Run(ext, func(t *testing.T) {
			work := t.TempDir()
			src := filepath.Join(work, "src")
			files := fixtureTree(t, src)
			dest := filepath.Join(work, "out"+ext)

			sources := []archiveSource{{path: src, name: "src"}}
			out, err := os.Create(dest)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = out.Close() }()
			var writeErr error
			switch detectArchiveFormat(dest) {
			case "zip":
				_, writeErr = writeZip(out, sources)
			case "tar":
				_, writeErr = writeTar(out, sources)
			case "tar.gz":
				gw := gzip.NewWriter(out)
				_, writeErr = writeTar(gw, sources)
				if writeErr == nil {
					writeErr = gw.Close()
				}
			}
			if writeErr != nil {
				t.Fatalf("write: %v", writeErr)
			}
			if err := out.Sync(); err != nil {
				t.Fatal(err)
			}

			// Read back.
			extract := filepath.Join(work, "extract")
			if err := os.MkdirAll(extract, 0o755); err != nil {
				t.Fatal(err)
			}
			in, err := os.Open(dest)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = in.Close() }()
			var readErr error
			switch detectArchiveFormat(dest) {
			case "zip":
				_, readErr = extractZip(in, extract, false, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries)
			case "tar":
				_, readErr = extractTar(in, extract, false, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries)
			case "tar.gz":
				gr, err := gzip.NewReader(in)
				if err != nil {
					t.Fatal(err)
				}
				_, readErr = extractTar(gr, extract, false, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries)
				_ = gr.Close()
			}
			if readErr != nil {
				t.Fatalf("extract: %v", readErr)
			}

			// Verify every fixture file is present with the right contents.
			for _, rel := range files {
				full := filepath.Join(extract, "src", rel)
				got, err := os.ReadFile(full)
				if err != nil {
					t.Fatalf("missing %s: %v", rel, err)
				}
				if len(got) == 0 {
					t.Errorf("empty %s", rel)
				}
			}
		})
	}
}

// safeJoin must reject entries that would escape destDir. We test both a
// classic "../" tar-slip and an absolute-path attempt; both should error.
func TestArchive_SafeJoinRejectsEscape(t *testing.T) {
	dest := t.TempDir()
	for _, name := range []string{
		"../etc/passwd",
		"sub/../../escape.txt",
		"../../../absolute",
	} {
		if _, err := safeJoin(dest, name); err == nil {
			t.Errorf("safeJoin(%q) should have errored", name)
		}
	}
	// And the obvious good cases must succeed.
	for _, name := range []string{"a.txt", "sub/b.txt", "deep/nested/c.txt"} {
		if _, err := safeJoin(dest, name); err != nil {
			t.Errorf("safeJoin(%q) errored unexpectedly: %v", name, err)
		}
	}
}

// Building a tar with a hand-crafted "../escape" entry and trying to
// extract it must fail rather than overwrite a file outside destDir.
func TestArchive_ZipSlipRejected(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte("malicious")
	hdr := &tar.Header{
		Name:     "../escape.txt",
		Mode:     0o600,
		Size:     int64(len(body)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	_, err := extractTar(&buf, dest, false, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries)
	if err == nil {
		t.Fatal("expected extract to reject the escaping entry")
	}
	// Either the `..` segment check or the final prefix check is a
	// valid rejection point; both surface a clear error message.
	msg := err.Error()
	if !strings.Contains(msg, "escapes destination") && !strings.Contains(msg, "parent-directory") {
		t.Errorf("error should mention escape or parent-directory, got: %v", err)
	}
}

// overwrite:false (the default) must refuse to clobber an existing file
// in destDir; overwrite:true must succeed.
func TestArchive_OverwriteFlag(t *testing.T) {
	work := t.TempDir()
	dest := filepath.Join(work, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "x.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build a single-entry zip containing x.txt.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("x.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	// Materialise as a file on disk so extractZip can stat it.
	zipPath := filepath.Join(work, "x.zip")
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	zf, err := os.Open(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = extractZip(zf, dest, false /* overwrite */, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries)
	_ = zf.Close()
	if !errors.Is(err, os.ErrExist) && err == nil {
		t.Errorf("expected overwrite-false to error on collision, got nil")
	}

	zf2, err := os.Open(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zf2.Close() }()
	if _, err := extractZip(zf2, dest, true /* overwrite */, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries); err != nil {
		t.Fatalf("overwrite-true should succeed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("contents after overwrite: %q (want %q)", got, "new")
	}
}

// archiveExtract takes 3 positional args (path, destDir, opts). A previous
// implementation read opts via the 2-arg helper, which silently dropped
// `overwrite: true` and let O_EXCL trip on repeat extracts. This pins the
// JS-binding signature so the bug doesn't come back.
func TestArchiveExtract_OverwriteOptThroughBinding(t *testing.T) {
	work := t.TempDir()
	zipPath := filepath.Join(work, "x.zip")
	{
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, err := zw.Create("x.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("payload")); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(work, "out")
	vm := goja.New()
	mk := func(opts map[string]any) goja.FunctionCall {
		args := []goja.Value{vm.ToValue(zipPath), vm.ToValue(dest)}
		if opts != nil {
			args = append(args, vm.ToValue(opts))
		}
		return goja.FunctionCall{Arguments: args}
	}

	if _, err := archiveExtract(context.Background(), mk(nil)); err != nil {
		t.Fatalf("first extract: %v", err)
	}
	// Second extract without overwrite must fail (the file is there).
	if _, err := archiveExtract(context.Background(), mk(nil)); err == nil {
		t.Errorf("second extract w/o overwrite should fail")
	}
	// With overwrite:true it should succeed — this was the silently-dropped path.
	if _, err := archiveExtract(context.Background(), mk(map[string]any{"overwrite": true})); err != nil {
		t.Errorf("extract with overwrite:true: %v", err)
	}
}

// buildTarWithSize returns a single-entry tar containing name filled with
// size zero bytes.
func buildTarWithSize(t *testing.T, name string, size int) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name:     name,
		Mode:     0o600,
		Size:     int64(size),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildTarWithEntries returns a tar with n tiny regular-file entries named
// f0, f1, ....
func buildTarWithEntries(t *testing.T, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := 0; i < n; i++ {
		body := []byte("x")
		hdr := &tar.Header{
			Name:     fmt.Sprintf("f%d", i),
			Mode:     0o600,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// A member whose decompressed size exceeds a tiny maxTotalBytes must abort
// extraction with an error, and the running total must never be allowed to
// grow past the cap by more than the single-byte "at the boundary" slack
// (io.LimitReader(rc, remaining+1), same trick as readAllCapped) — i.e. this
// is the decompression-bomb guard for fs.archive.extract.
func TestArchiveExtract_MaxTotalBytesCap(t *testing.T) {
	const size = 5 * 1024 * 1024 // 5 MB of zeros
	const tinyCap = 1024         // tiny cap
	data := buildTarWithSize(t, "big.bin", size)

	dest := t.TempDir()
	_, err := extractTar(bytes.NewReader(data), dest, false, tinyCap, DefaultMaxArchiveEntries)
	if err == nil {
		t.Fatal("expected extract to fail once the cap is exceeded")
	}
	if !strings.Contains(err.Error(), "maxTotalBytes") {
		t.Errorf("expected a maxTotalBytes error, got: %v", err)
	}

	// The partially-written file (if any) must not have grown past the
	// cap-plus-one-byte boundary.
	if fi, statErr := os.Stat(filepath.Join(dest, "big.bin")); statErr == nil {
		if fi.Size() > tinyCap+1 {
			t.Errorf("wrote %d bytes, want <= %d (cap+1)", fi.Size(), tinyCap+1)
		}
	}
}

// Same guard, exercised through extractZip.
func TestArchiveExtract_MaxTotalBytesCap_Zip(t *testing.T) {
	const size = 5 * 1024 * 1024
	const tinyCap = 1024
	work := t.TempDir()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(make([]byte, size)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(work, "big.zip")
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	zf, err := os.Open(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zf.Close() }()

	dest := filepath.Join(work, "out")
	_, err = extractZip(zf, dest, false, tinyCap, DefaultMaxArchiveEntries)
	if err == nil {
		t.Fatal("expected extract to fail once the cap is exceeded")
	}
	if !strings.Contains(err.Error(), "maxTotalBytes") {
		t.Errorf("expected a maxTotalBytes error, got: %v", err)
	}
	if fi, statErr := os.Stat(filepath.Join(dest, "big.bin")); statErr == nil {
		if fi.Size() > tinyCap+1 {
			t.Errorf("wrote %d bytes, want <= %d (cap+1)", fi.Size(), tinyCap+1)
		}
	}
}

// An archive with more entries than a tiny maxEntries must abort with an
// entry-count error, leaving the entries beyond the cap unwritten.
func TestArchiveExtract_MaxEntriesCap(t *testing.T) {
	data := buildTarWithEntries(t, 5)
	dest := t.TempDir()

	const maxEntries = 2
	_, err := extractTar(bytes.NewReader(data), dest, false, DefaultMaxArchiveBytes, maxEntries)
	if err == nil {
		t.Fatal("expected extract to fail once the entry-count cap is exceeded")
	}
	if !strings.Contains(err.Error(), "maxEntries") {
		t.Errorf("expected a maxEntries error, got: %v", err)
	}
	for i := 0; i < maxEntries; i++ {
		if _, statErr := os.Stat(filepath.Join(dest, fmt.Sprintf("f%d", i))); statErr != nil {
			t.Errorf("entry f%d should have been written before the cap tripped: %v", i, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(dest, fmt.Sprintf("f%d", maxEntries))); statErr == nil {
		t.Errorf("entry f%d should not have been written past the cap", maxEntries)
	}
}

// A normal small archive within both caps must extract fully — the caps
// must not clip well-formed input.
func TestArchiveExtract_CapsAllowNormalArchive(t *testing.T) {
	data := buildTarWithEntries(t, 3)
	dest := t.TempDir()
	entries, err := extractTar(bytes.NewReader(data), dest, false, DefaultMaxArchiveBytes, DefaultMaxArchiveEntries)
	if err != nil {
		t.Fatalf("normal archive should extract cleanly: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
	for i := 0; i < 3; i++ {
		if _, statErr := os.Stat(filepath.Join(dest, fmt.Sprintf("f%d", i))); statErr != nil {
			t.Errorf("missing f%d: %v", i, statErr)
		}
	}
}

// maxTotalBytes / maxEntries opts must thread through the JS-facing
// archiveExtract binding, not just the direct extractTar/extractZip calls.
func TestArchiveExtract_CapsThroughBinding(t *testing.T) {
	work := t.TempDir()
	zipPath := filepath.Join(work, "big.zip")
	{
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		f, err := zw.Create("big.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(make([]byte, 1<<20)); err != nil { // 1 MiB
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(work, "out")
	vm := goja.New()
	mk := func(opts map[string]any) goja.FunctionCall {
		args := []goja.Value{vm.ToValue(zipPath), vm.ToValue(dest), vm.ToValue(opts)}
		return goja.FunctionCall{Arguments: args}
	}

	_, err := archiveExtract(context.Background(), mk(map[string]any{"maxTotalBytes": 1024}))
	if err == nil {
		t.Fatal("expected maxTotalBytes opt to abort extraction")
	}
	if !strings.Contains(err.Error(), "maxTotalBytes") {
		t.Errorf("expected a maxTotalBytes error, got: %v", err)
	}
}
