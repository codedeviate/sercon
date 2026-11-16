package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func jsq(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func stuffrScript(t *testing.T, script string) string {
	t.Helper()
	out, err := runScript(t, script)
	if err != nil {
		t.Fatalf("script failed: %v\noutput: %s", err, out)
	}
	return out
}

func TestStuffrJS_RunReturnsCode(t *testing.T) {
	out := stuffrScript(t, `
		const r = await stuffr.run(["list", "/definitely/missing"]);
		runtime.log(r.code !== 0, typeof r.stderr, r.stdout instanceof Uint8Array, typeof r.text);
		const f = await stuffr.run(["formats", "--json"]);
		runtime.log(f.code, JSON.parse(f.text()).formats.length > 0);
	`)
	if !strings.Contains(out, "true string true function") || !strings.Contains(out, "0 true") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrJS_PackListUnpack(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := stuffrScript(t, `
		const dir = `+jsq(dir)+`;
		await stuffr.pack(["a.txt"], { cwd: dir, output: "x.tar.gz" });
		const l = await stuffr.list(dir + "/x.tar.gz");
		runtime.log("list", JSON.stringify(l).includes("a.txt"));
		await stuffr.unpack(dir + "/x.tar.gz", { directory: dir + "/out" });
		await stuffr.test(dir + "/x.tar.gz");
		const i = await stuffr.info(dir + "/x.tar.gz");
		runtime.log("info", typeof i);
	`)
	if !strings.Contains(out, "list true") || !strings.Contains(out, "info object") {
		t.Fatalf("unexpected: %q", out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "out", "a.txt"))
	if err != nil || string(b) != "alpha" {
		t.Fatalf("unpacked: %q %v", b, err)
	}
}

func TestStuffrJS_HelperThrowsWithFields(t *testing.T) {
	out := stuffrScript(t, `
		try { await stuffr.unpack("/definitely/missing.tar.gz"); runtime.log("no throw"); }
		catch (e) {
			runtime.log(e.code, e.kind, typeof e.stderr, e.argv[0], String(e.message).startsWith("stuffr unpack exited"));
		}
	`)
	if strings.Contains(out, "no throw") || !strings.Contains(out, "unpack true") || !strings.Contains(out, "string unpack") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrJS_Cat(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644)
	out := stuffrScript(t, `
		const dir = `+jsq(dir)+`;
		await stuffr.pack(["a.txt"], { cwd: dir, output: "a.txt.gz", format: "gzip" });
		const b = await stuffr.cat(dir + "/a.txt.gz");
		runtime.log(b instanceof Uint8Array, String.fromCharCode(...b));
	`)
	if !strings.Contains(out, "true hello") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrJS_Formats(t *testing.T) {
	out := stuffrScript(t, `
		const f = await stuffr.formats();
		runtime.log(Array.isArray(f.formats), JSON.stringify(f.formats).includes("zstd"));
	`)
	if !strings.Contains(out, "true true") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrJS_Version(t *testing.T) {
	out := stuffrScript(t, `runtime.log(stuffr.version);`)
	if strings.TrimSpace(out) != stuffrVersion() || stuffrVersion() == "" {
		t.Fatalf("version %q vs %q", out, stuffrVersion())
	}
}

func TestStuffrJS_AwkwardPaths(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"-": "dash", "-x.txt": "lead", "sp ace.txt": "space"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := stuffrScript(t, `
		const dir = `+jsq(dir)+`;
		await stuffr.pack(["-", "-x.txt", "sp ace.txt"], { cwd: dir, output: "w.tar" });
		await stuffr.unpack(dir + "/w.tar", { directory: dir + "/out" });
		runtime.log("ok");
	`)
	if !strings.Contains(out, "ok") {
		t.Fatalf("unexpected: %q", out)
	}
	for name, body := range map[string]string{"-": "dash", "-x.txt": "lead", "sp ace.txt": "space"} {
		b, err := os.ReadFile(filepath.Join(dir, "out", name))
		if err != nil || string(b) != body {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
}

func TestStuffrJS_PackZstdNeedsWeakEncoderFlag(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("zstd body"), 0o644)
	out := stuffrScript(t, `
		const dir = `+jsq(dir)+`;
		try { await stuffr.pack(["a.txt"], { cwd: dir, output: "a.tar.zst" }); runtime.log("no throw"); }
		catch (e) { runtime.log("rejected", e.kind); }
		await stuffr.pack(["a.txt"], { cwd: dir, output: "b.tar.zst", allowWeakEncoder: true });
		await stuffr.unpack(dir + "/b.tar.zst", { directory: dir + "/out" });
		runtime.log("roundtrip");
	`)
	if strings.Contains(out, "no throw") || !strings.Contains(out, "rejected") || !strings.Contains(out, "roundtrip") {
		t.Fatalf("unexpected: %q", out)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "out", "a.txt")); err != nil || string(b) != "zstd body" {
		t.Fatalf("unpacked: %q %v", b, err)
	}
}

func TestStuffrJS_MemoryLimitPassed(t *testing.T) {
	argv := buildStuffrArgv("unpack", []string{"x"}, stuffrOpts{MemoryLimit: "1GiB"})
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "--memory-limit 1GiB") || !strings.Contains(joined, "-- x") {
		t.Fatalf("argv = %v", argv)
	}
	for _, verb := range []string{"pack", "unpack", "convert", "test"} {
		a := buildStuffrArgv(verb, []string{"x"}, stuffrOpts{StrictFidelity: true})
		if !slices.Contains(a, "--strict-fidelity") {
			t.Fatalf("%s argv = %v", verb, a)
		}
		if b := buildStuffrArgv(verb, []string{"x"}, stuffrOpts{}); slices.Contains(b, "--strict-fidelity") {
			t.Fatalf("%s argv without option = %v", verb, b)
		}
	}
	argv = buildStuffrArgv("pack", []string{"-"}, stuffrOpts{Output: "o.tar", Format: "tar", Level: intp(3), Force: true, AllowWeakEncoder: true})
	want := "pack --output o.tar --format tar --level 3 --force --allow-weak-encoder -- ./-"
	if strings.Join(argv, " ") != want {
		t.Fatalf("argv = %v, want %s", argv, want)
	}
}

func intp(i int) *int { return &i }

func TestStuffrJS_Exit4MessageSaysOutputWritten(t *testing.T) {
	e := &stuffrExitError{Argv: []string{"pack", "x"}, Code: 4, Stderr: "warning: lost\nmore"}
	msg := e.Error()
	if !strings.Contains(msg, "stuffr pack exited 4 (degraded)") || !strings.Contains(msg, "warning: lost") || !strings.Contains(msg, "(output was written)") || strings.Contains(msg, "more") {
		t.Fatalf("msg = %q", msg)
	}
	if got := (&stuffrExitError{Argv: []string{"x"}, Code: 99}).Error(); !strings.Contains(got, "(unknown)") || strings.Contains(got, "output was written") {
		t.Fatalf("msg = %q", got)
	}
	f := e.ErrorFields()
	if f["code"] != 4 || f["kind"] != "degraded" {
		t.Fatalf("fields = %v", f)
	}
}

func TestStuffrJS_RelativeCwdOption(t *testing.T) {
	wd, _ := os.Getwd()
	dir, err := os.MkdirTemp(wd, "stuffr-cwd-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("rel"), 0o644)
	out := stuffrScript(t, `
		await stuffr.pack(["a.txt"], { cwd: `+jsq(filepath.Base(dir))+`, output: "r.tar" });
		runtime.log("ok");
	`)
	if !strings.Contains(out, "ok") {
		t.Fatalf("unexpected: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "r.tar")); err != nil {
		t.Fatal(err)
	}
}

func TestStuffrJS_Stdin(t *testing.T) {
	out := stuffrScript(t, `
		const gz = await codec.compression.compress("gzip", "plain text");
		const r = await stuffr.run(["cat", "-"], { stdin: new Uint8Array(gz) });
		runtime.log(r.code, JSON.stringify(r.text()));
		const r2 = await stuffr.run(["cat", "-"], { stdin: new Uint8Array(gz).buffer });
		runtime.log(r2.code, r2.text());
	`)
	if !strings.Contains(out, `0 "plain text"`) || !strings.Contains(out, "0 plain text") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrJS_JSONIsPlainObjectWithDocumentOrder(t *testing.T) {
	res, err := testStuffr(t).run(t.Context(), stuffrRunReq{Argv: []string{"formats", "--json"}, Cwd: "/"})
	if err != nil || res.Code != 0 {
		t.Fatalf("run: %v code=%d", err, res.Code)
	}
	var doc struct {
		Formats []json.RawMessage `json:"formats"`
	}
	if err := json.Unmarshal(res.Stdout, &doc); err != nil || len(doc.Formats) == 0 {
		t.Fatalf("parse: %v", err)
	}
	dec := json.NewDecoder(strings.NewReader(string(doc.Formats[0])))
	dec.Token()
	var keys []string
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var skip json.RawMessage
		dec.Decode(&skip)
	}
	out := stuffrScript(t, `
		const f = await stuffr.formats();
		runtime.log(Object.keys(f.formats[0]).join(","));
		runtime.log(Object.getPrototypeOf(f) === Object.prototype, Object.getPrototypeOf(f.formats[0]) === Object.prototype, Array.isArray(f.formats));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != strings.Join(keys, ",") {
		t.Fatalf("key order %q, want %q", lines[0], strings.Join(keys, ","))
	}
	if lines[1] != "true true true" {
		t.Fatalf("not plain: %q", lines[1])
	}
}

func TestStuffrJS_VoidHelpersResolveMessages(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	out := stuffrScript(t, `
		const r = await stuffr.pack(["a.txt"], { cwd: `+jsq(dir)+`, output: "a.tar" });
		runtime.log(Object.getPrototypeOf(r) === Object.prototype, Array.isArray(r.messages), r.messages.length > 0 && r.messages.every((l: any) => typeof l === "string"));
	`)
	if !strings.Contains(out, "true true true") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestStuffrMessages_Lines(t *testing.T) {
	if got := stuffrMessages(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty stderr must give a non-nil empty slice, got %#v", got)
	}
	got := stuffrMessages([]byte("a\r\n\n  \n  - b\n"))
	if len(got) != 2 || got[0] != "a" || got[1] != "  - b" {
		t.Fatalf("got %q", got)
	}
}

func TestStuffrJS_OptionTypeErrorsAndEmptyStderr(t *testing.T) {
	if _, err := runScript(t, `await stuffr.pack(["a"], { force: "yes" });`); err == nil || !strings.Contains(err.Error(), "must be a boolean") {
		t.Fatalf("err = %v", err)
	}
	if got := (&stuffrExitError{Argv: []string{"test"}, Code: 1}).Error(); got != "stuffr test exited 1 (failed)" {
		t.Fatalf("msg = %q", got)
	}
}
