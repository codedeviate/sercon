//go:build unix

package main

import (
	"archive/tar"
	"archive/zip"
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

func TestStuffrJS_UnpackSurfacesMessages(t *testing.T) {
	dir := t.TempDir()
	makeTar(t, filepath.Join(dir, "x.tar"), []tarEnt{{name: "run.sh", mode: 0o755, body: "#!/bin/sh\n"}})
	out := stuffrScript(t, `
		const r = await stuffr.unpack(`+jsq(filepath.Join(dir, "x.tar"))+`, { directory: `+jsq(filepath.Join(dir, "out"))+` });
		runtime.log("plain", Object.getPrototypeOf(r) === Object.prototype, Array.isArray(r.messages));
		runtime.log("hit", r.messages.some((l: string) => l.includes("missing metadata") && l.includes("mode")));
	`)
	if !strings.Contains(out, "plain true true") || !strings.Contains(out, "hit true") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrJS_StrictFidelityThrowsDegraded(t *testing.T) {
	dir := t.TempDir()
	makeTar(t, filepath.Join(dir, "x.tar"), []tarEnt{{name: "run.sh", mode: 0o755, body: "#!/bin/sh\n"}})
	out := stuffrScript(t, `
		try {
			await stuffr.unpack(`+jsq(filepath.Join(dir, "x.tar"))+`, { directory: `+jsq(filepath.Join(dir, "out"))+`, strictFidelity: true });
			runtime.log("no throw");
		} catch (e: any) {
			runtime.log("code", e.code, e.kind, /output was written/.test(e.message));
		}
	`)
	if !strings.Contains(out, "code 4 degraded true") {
		t.Fatalf("unexpected: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "run.sh")); err != nil {
		t.Fatalf("extracted file should still exist: %v", err)
	}
}

// interimFixture creates base/src/{secret (0600), tool (0755), d/ (0700)} and
// returns base. The stuffr cwd is base.
func interimFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	src := filepath.Join(base, "src")
	for _, d := range []string{src, filepath.Join(src, "d")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]os.FileMode{"secret": 0o600, "tool": 0o755, "d/in": 0o600}
	for n, m := range files {
		p := filepath.Join(src, n)
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, m); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(src, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	return base
}

func tarModes(t *testing.T, path string) map[string]int64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]int64{}
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		out[strings.TrimSuffix(h.Name, "/")] = h.Mode & 0o777
	}
	return out
}

func zipModes(t *testing.T, path string) map[string]os.FileMode {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string]os.FileMode{}
	for _, f := range zr.File {
		out[strings.TrimSuffix(f.Name, "/")] = f.Mode().Perm()
	}
	return out
}

// TestStuffrInterim_PackReportsMode pins what the embedded build (stuffr
// 0.31.0+, no host-fs layer) does with file modes on pack: it cannot read
// them, so every archive still RECORDS 0644 for files (a 0600 secret is
// widened, a 0755 tool loses its exec bit) — but stuffr now SAYS so: each
// entry warns "missing metadata: mode" and --strict-fidelity exits 4
// (degraded, output written). Before 0.31.0 zip and single-stream outputs
// claimed "no fidelity loss". If this test fails because modes are now
// recorded (stuffr's host-fs layer, item 4), update MANUAL §5.16,
// docs_stuffr.go and CHANGELOG, then flip the 0644 assertions.
func TestStuffrInterim_PackReportsMode(t *testing.T) {
	const modeWarn = "missing metadata: mode"
	t.Run("Tar", func(t *testing.T) {
		base := interimFixture(t)
		out := filepath.Join(base, "o.tar")
		code, stderr := wasiRun(t, base, "pack", "--output", out, "--", "src")
		m := tarModes(t, out)
		t.Logf("code=%d modes=%v stderr=%q", code, m, stderr)
		if code != 0 || m["src/secret"] != 0o644 || m["src/tool"] != 0o644 || m["src/d"] != 0o755 {
			t.Fatalf("pack behaviour changed (update docs!): code=%d modes=%v", code, m)
		}
		if !strings.Contains(stderr, modeWarn) || !strings.Contains(stderr, "uid_gid") {
			t.Fatalf("expected mode and owner (uid_gid) warnings, got %q", stderr)
		}
		out2 := filepath.Join(base, "o2.tar")
		if c, _ := wasiRun(t, base, "pack", "--strict-fidelity", "--output", out2, "--", "src"); c != 4 {
			t.Fatalf("strict tar pack exit = %d, want 4", c)
		}
	})
	t.Run("Zip", func(t *testing.T) {
		base := interimFixture(t)
		out := filepath.Join(base, "o.zip")
		code, stderr := wasiRun(t, base, "pack", "--output", out, "--", "src")
		m := zipModes(t, out)
		t.Logf("code=%d modes=%v stderr=%q", code, m, stderr)
		if code != 0 || m["src/secret"] != 0o644 || m["src/tool"] != 0o644 {
			t.Fatalf("pack behaviour changed (update docs!): code=%d modes=%v", code, m)
		}
		if !strings.Contains(stderr, "`src/secret` is missing metadata: mode") || strings.Contains(stderr, "no fidelity loss") {
			t.Fatalf("zip pack must name the lost mode per entry, got %q", stderr)
		}
		out2 := filepath.Join(base, "o2.zip")
		c, stderr2 := wasiRun(t, base, "pack", "--strict-fidelity", "--output", out2, "--", "src")
		if c != 4 || !strings.Contains(stderr2, modeWarn) {
			t.Fatalf("strict zip pack: code=%d stderr=%q, want 4 + mode warning", c, stderr2)
		}
		if _, err := os.Stat(out2); err != nil {
			t.Fatalf("exit 4 must still write the output: %v", err)
		}
	})
	t.Run("SingleStreamGz", func(t *testing.T) {
		base := interimFixture(t)
		out := filepath.Join(base, "secret.gz")
		code, stderr := wasiRun(t, base, "pack", "--strict-fidelity", "--output", out, "--", "src/secret")
		st, err := os.Stat(out)
		if err != nil {
			t.Fatalf("no output: %v (code=%d stderr=%q)", err, code, stderr)
		}
		t.Logf("code=%d mode=%v stderr=%q", code, st.Mode().Perm(), stderr)
		if code != 4 || !strings.Contains(stderr, modeWarn) || strings.Contains(stderr, "no fidelity loss") {
			t.Fatalf("strict .gz pack: code=%d stderr=%q, want 4 + mode warning", code, stderr)
		}
	})
	t.Run("OracleGap", func(t *testing.T) {
		oracleBin(t)
		base := interimFixture(t)
		out := filepath.Join(base, "o.tar")
		if code, stderr := oracleRun(t, base, "pack", "--output", out, "--", "src"); code != 0 {
			t.Fatalf("oracle pack: %d %q", code, stderr)
		}
		m := tarModes(t, out)
		t.Logf("oracle modes=%v", m)
		if m["src/secret"] != 0o600 || m["src/tool"] != 0o755 {
			t.Fatalf("oracle should preserve 0600/0755, got %v", m)
		}
		zout := filepath.Join(base, "o.zip")
		if code, stderr := oracleRun(t, base, "pack", "--output", zout, "--", "src"); code != 0 {
			t.Fatalf("oracle zip pack: %d %q", code, stderr)
		}
		zm := zipModes(t, zout)
		t.Logf("oracle zip modes=%v", zm)
		if zm["src/secret"] != 0o600 || zm["src/tool"] != 0o755 {
			t.Fatalf("oracle zip should preserve 0600/0755, got %v", zm)
		}
	})
}

// TestStuffrInterim_SalvageReportsMode pins salvage under the embedded build
// (stuffr 0.31.0+): salvage -C cannot apply modes, so restored files keep the
// mode they were created with (0600 here) at exit 0, and stuffr notes each one
// ("its mode could not be applied"). salvage --index N -o FILE works (before
// 0.31.0 it trapped on std::env::temp_dir()) and leaves no staging directory
// (stuffr-salvage-one-*) behind.
func TestStuffrInterim_SalvageReportsMode(t *testing.T) {
	base := interimFixture(t)
	arc := filepath.Join(base, "s.tar")
	if code, stderr := wasiRun(t, base, "pack", "--output", arc, "--", "src"); code != 0 {
		t.Fatalf("pack: %d %q", code, stderr)
	}
	dst := filepath.Join(base, "salv")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	code, stderr := wasiRun(t, base, "salvage", "-C", dst, "--", arc)
	t.Logf("salvage -C: code=%d stderr=%q", code, stderr)
	var found int
	filepath.WalkDir(dst, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found++
			st, _ := os.Stat(p)
			if st.Mode().Perm() != 0o600 {
				t.Errorf("%s mode = %v, want 0600 (update docs if modes are now applied)", p, st.Mode().Perm())
			}
		}
		return nil
	})
	if code != 0 || found != 3 {
		t.Fatalf("salvage -C: code=%d files=%d, want 0 and 3", code, found)
	}
	for _, f := range []string{"src/secret", "src/tool", "src/d/in"} {
		if !strings.Contains(stderr, "`"+f+"`: written, but its mode could not be applied") {
			t.Errorf("salvage -C: no mode note for %s in %q", f, stderr)
		}
	}
	one := filepath.Join(base, "one")
	res, err := testStuffr(t).run(context.Background(), stuffrRunReq{
		Argv: []string{"salvage", "--index", "2", "-o", one, "--", arc}, Cwd: base})
	if err != nil {
		t.Fatalf("salvage --index -o: %v", err)
	}
	if res.Code != 0 {
		t.Fatalf("salvage --index -o: code=%d stderr=%q", res.Code, res.Stderr)
	}
	if b, err := os.ReadFile(one); err != nil || string(b) != "x\n" {
		t.Fatalf("salvage --index -o output: %q, %v", b, err)
	}
	if left, _ := filepath.Glob(filepath.Join(base, "stuffr-salvage-one-*")); len(left) != 0 {
		t.Fatalf("staging directory left behind: %v", left)
	}
}
