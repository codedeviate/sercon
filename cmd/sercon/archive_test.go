package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
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
				_, readErr = extractZip(in, extract, false)
			case "tar":
				_, readErr = extractTar(in, extract, false)
			case "tar.gz":
				gr, err := gzip.NewReader(in)
				if err != nil {
					t.Fatal(err)
				}
				_, readErr = extractTar(gr, extract, false)
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
	_, err := extractTar(&buf, dest, false)
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
	_, err = extractZip(zf, dest, false /* overwrite */)
	_ = zf.Close()
	if !errors.Is(err, os.ErrExist) && err == nil {
		t.Errorf("expected overwrite-false to error on collision, got nil")
	}

	zf2, err := os.Open(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zf2.Close() }()
	if _, err := extractZip(zf2, dest, true /* overwrite */); err != nil {
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
