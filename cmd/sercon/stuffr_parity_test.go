//go:build unix

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Parity tests compare the embedded (WASI) stuffr against a native build of the
// same commit, named by STUFFR_ORACLE.

func oracleBin(t *testing.T) string {
	t.Helper()
	p := os.Getenv("STUFFR_ORACLE")
	if p == "" {
		t.Skip("STUFFR_ORACLE not set")
	}
	return p
}

type tarEnt struct {
	name, link string
	mode       int64
	body       string
	typ        byte
}

func makeTar(t *testing.T, path string, ents []tarEnt) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range ents {
		h := &tar.Header{Name: e.name, Mode: e.mode, Typeflag: e.typ, Linkname: e.link}
		if e.typ == 0 {
			h.Typeflag = tar.TypeReg
		}
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wasiRun(t *testing.T, cwd string, argv ...string) (int, string) {
	t.Helper()
	res, err := testStuffr(t).run(context.Background(), stuffrRunReq{Argv: argv, Cwd: cwd})
	if err != nil {
		t.Fatalf("wasi run %v: %v", argv, err)
	}
	return res.Code, string(res.Stderr)
}

func oracleRun(t *testing.T, cwd string, argv ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(oracleBin(t), argv...)
	cmd.Dir = cwd
	var eb bytes.Buffer
	cmd.Stderr = &eb
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), eb.String()
		}
		t.Fatalf("oracle %v: %v", argv, err)
	}
	return 0, eb.String()
}

func listTree(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && p != root {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// safetyCase runs one hostile archive through both builds. setup prepares a
// fresh scenario (dst, outside) and returns the tar entries. With strict set,
// both builds must refuse (non-zero) with equal exit codes; otherwise only the
// "nothing outside dst" invariant is enforced and each outcome is logged.
// Layout: base/{dst,outside,a.tar,work/cwd}; the stuffr cwd is nested inside
// the scanned tree so a cwd-relative escape is also caught.
func safetyCase(t *testing.T, strict bool, setup func(dst, outside string) []tarEnt) {
	oracleBin(t)
	run := func(name string, fn func(t *testing.T, cwd string, argv ...string) (int, string)) int {
		base := t.TempDir()
		dst := filepath.Join(base, "dst")
		outside := filepath.Join(base, "outside")
		cwd := filepath.Join(base, "work", "cwd")
		for _, d := range []string{dst, outside, cwd} {
			os.MkdirAll(d, 0o755)
		}
		arc := filepath.Join(base, "a.tar")
		makeTar(t, arc, setup(dst, outside))
		code, stderr := fn(t, cwd, "unpack", "--directory", dst, "--", arc)
		t.Logf("%s: code=%d stderr=%q", name, code, strings.TrimSpace(stderr))
		if strict && code == 0 {
			t.Errorf("%s: hostile archive was accepted (exit 0)", name)
		}
		if got := listTree(t, outside); len(got) != 0 {
			t.Errorf("%s: files written outside destination: %v", name, got)
		}
		sep := string(filepath.Separator)
		for _, e := range listTree(t, base) {
			switch {
			case e == "dst", e == "outside", e == "a.tar", e == "work", e == "work"+sep+"cwd":
			case strings.HasPrefix(e, "dst"+sep):
			default:
				t.Errorf("%s: stray path in tree: %s", name, e)
			}
		}
		return code
	}
	w := run("wasi", wasiRun)
	o := run("oracle", oracleRun)
	if strict && w != o {
		t.Errorf("exit code mismatch: wasi=%d oracle=%d", w, o)
	}
}

func TestStuffrParity_EscapeRefused(t *testing.T) {
	// One entry per case: a single archive would abort at the first entry and
	// never evaluate the second.
	t.Run("DotDot", func(t *testing.T) {
		safetyCase(t, true, func(dst, outside string) []tarEnt {
			return []tarEnt{{name: "../evil", mode: 0o644, body: "x"}}
		})
	})
	t.Run("Absolute", func(t *testing.T) {
		safetyCase(t, true, func(dst, outside string) []tarEnt {
			return []tarEnt{{name: filepath.Join(outside, "abs-evil"), mode: 0o644, body: "x"}}
		})
	})
}

func TestStuffrParity_SymlinkAncestorRefused(t *testing.T) {
	safetyCase(t, true, func(dst, outside string) []tarEnt {
		if err := os.Symlink(outside, filepath.Join(dst, "sub")); err != nil {
			t.Fatal(err)
		}
		return []tarEnt{{name: "sub/file", mode: 0o644, body: "x"}}
	})
}

func TestStuffrParity_ArchiveSymlinkThenFile(t *testing.T) {
	for name, link := range map[string]func(outside string) string{
		"UnsafeAbsoluteSymlinkTargetRefused": func(o string) string { return o },
		"UnsafeClimbingSymlinkTargetRefused": func(string) string { return "../outside" },
	} {
		t.Run(name, func(t *testing.T) {
			safetyCase(t, true, func(dst, outside string) []tarEnt {
				return []tarEnt{
					{name: "l", typ: tar.TypeSymlink, link: link(outside)},
					{name: "l/file", mode: 0o644, body: "x"},
				}
			})
		})
	}
	// Safe in-dst symlink, then a write through it. DOCUMENTED DIVERGENCE: the
	// interim WASI build cannot create symlinks (exit 3 at `l`) until stuffr's
	// item 4, while the native oracle may support them, so exit-code parity is
	// not required here; only that nothing lands outside dst.
	t.Run("SafeSymlinkThenWriteThrough", func(t *testing.T) {
		safetyCase(t, false, func(dst, outside string) []tarEnt {
			return []tarEnt{
				{name: "sub/", typ: tar.TypeDir, mode: 0o755},
				{name: "l", typ: tar.TypeSymlink, link: "sub"},
				{name: "l/file", mode: 0o644, body: "x"},
			}
		})
	})
}

func TestStuffrParity_ContentRoundTrip(t *testing.T) {
	oracleBin(t)
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "d", "e"), 0o755)
	files := map[string]string{
		"a.txt":     "hello world\n",
		"d/b.bin":   string(randBytes(5000, 1)),
		"d/e/c.txt": strings.Repeat("compressible ", 500),
	}
	for n, c := range files {
		os.WriteFile(filepath.Join(src, n), []byte(c), 0o644)
	}
	check := func(t *testing.T, dir string) {
		t.Helper()
		for n, c := range files {
			got, err := os.ReadFile(filepath.Join(dir, "in", n))
			if err != nil || string(got) != c {
				t.Errorf("content mismatch for %s (err=%v)", n, err)
			}
		}
	}
	for _, ext := range []string{"tar.gz", "tar.zst", "tar.xz", "zip"} {
		for _, packer := range []string{"wasi", "oracle"} {
			t.Run(ext+"/"+packer+"-pack", func(t *testing.T) {
				work := t.TempDir()
				// pack from a dir named "in" so extraction yields in/<files>
				in := filepath.Join(work, "in")
				if err := os.Symlink(src, in); err != nil {
					t.Fatal(err)
				}
				arc := filepath.Join(work, "x."+ext)
				argv := []string{"pack", "-o", arc, "in"}
				if ext == "tar.zst" { // the pure zstd encoder is a weak fallback
					argv = []string{"pack", "--allow-weak-encoder", "-o", arc, "in"}
				}
				var pk, un func(*testing.T, string, ...string) (int, string)
				if packer == "wasi" {
					pk, un = wasiRun, oracleRun
				} else {
					pk, un = oracleRun, wasiRun
				}
				if c, se := pk(t, work, argv...); c != 0 {
					t.Fatalf("pack code=%d stderr=%s", c, se)
				}
				os.Remove(in)
				out := filepath.Join(work, "out")
				os.MkdirAll(out, 0o755)
				if c, se := un(t, work, "unpack", "--directory", out, "--", arc); c != 0 {
					t.Fatalf("unpack code=%d stderr=%s", c, se)
				}
				check(t, out)
			})
		}
	}
}

// TestStuffrInterim_MetadataReported pins what the interim embedded build does
// with file modes and symlinks (no host-fs metadata layer yet) and requires it
// to be honest: applied, or reported by the relevant warning / refusal.
func TestStuffrInterim_MetadataReported(t *testing.T) {
	unpack := func(t *testing.T, ents []tarEnt) (code int, stderr, dst string) {
		base := t.TempDir()
		dst = filepath.Join(base, "dst")
		os.MkdirAll(dst, 0o755)
		arc := filepath.Join(base, "m.tar")
		makeTar(t, arc, ents)
		code, stderr = wasiRun(t, base, "unpack", "--directory", dst, "--", arc)
		return
	}
	t.Run("Mode", func(t *testing.T) {
		code, stderr, dst := unpack(t, []tarEnt{{name: "run.sh", mode: 0o755, body: "#!/bin/sh\n"}})
		st, err := os.Stat(filepath.Join(dst, "run.sh"))
		if err != nil {
			t.Fatalf("run.sh not extracted: %v (code=%d stderr=%q)", err, code, stderr)
		}
		mode := st.Mode().Perm()
		t.Logf("code=%d mode=%v stderr=%q", code, mode, stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if mode != 0o755 && (!strings.Contains(stderr, "missing metadata") || !strings.Contains(stderr, "mode")) {
			t.Fatalf("mode %v silently dropped: no fidelity warning naming mode in stderr %q", mode, stderr)
		}
	})
	t.Run("Symlink", func(t *testing.T) {
		code, stderr, dst := unpack(t, []tarEnt{
			{name: "run.sh", mode: 0o755, body: "#!/bin/sh\n"},
			{name: "link", typ: tar.TypeSymlink, link: "run.sh"},
		})
		target, lerr := os.Readlink(filepath.Join(dst, "link"))
		t.Logf("code=%d symlinkTarget=%q symlinkErr=%v stderr=%q", code, target, lerr, stderr)
		if lerr == nil && target == "run.sh" {
			return // symlink applied
		}
		if code == 0 || !strings.Contains(stderr, "symlink") {
			t.Fatalf("symlink silently dropped: code=%d stderr=%q", code, stderr)
		}
	})
}
