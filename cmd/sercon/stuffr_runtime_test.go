package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/sys"
)

var (
	testStuffrOnce sync.Once
	testStuffrRT   *stuffrRuntime
	testStuffrErr  error
)

// testStuffr returns a runtime shared by the tests that need no fresh one.
func testStuffr(t *testing.T) *stuffrRuntime {
	t.Helper()
	testStuffrOnce.Do(func() {
		testStuffrRT, testStuffrErr = newStuffrRuntime(context.Background(), stuffrCacheDir())
	})
	if testStuffrErr != nil {
		t.Fatal(testStuffrErr)
	}
	return testStuffrRT
}

func randBytes(n int, seed int64) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

func TestStuffrRun_Formats(t *testing.T) {
	res, err := testStuffr(t).run(context.Background(), stuffrRunReq{Argv: []string{"formats", "--json"}, Cwd: "/"})
	if err != nil || res.Code != 0 {
		t.Fatalf("code=%d err=%v stderr=%s", res.Code, err, res.Stderr)
	}
	if !bytes.Contains(res.Stdout, []byte(`"format":"gzip"`)) {
		t.Fatalf("unexpected stdout: %.200s", res.Stdout)
	}
}

func TestStuffrRun_NonZeroExit(t *testing.T) {
	res, err := testStuffr(t).run(context.Background(), stuffrRunReq{Argv: []string{"list", "/definitely/missing"}, Cwd: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Code == 0 || len(res.Stderr) == 0 {
		t.Fatalf("code=%d stderr=%q", res.Code, res.Stderr)
	}
}

func TestStuffrRun_RelativeCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello stuffr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := testStuffr(t)
	res, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"pack", "a.txt", "-o", "a.txt.gz"}, Cwd: dir})
	if err != nil || res.Code != 0 {
		t.Fatalf("pack code=%d err=%v stderr=%s", res.Code, err, res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt.gz")); err != nil {
		t.Fatal(err)
	}
	res, err = rt.run(context.Background(), stuffrRunReq{Argv: []string{"cat", "a.txt.gz"}, Cwd: dir})
	if err != nil || res.Code != 0 || string(res.Stdout) != "hello stuffr\n" {
		t.Fatalf("cat code=%d err=%v out=%q stderr=%s", res.Code, err, res.Stdout, res.Stderr)
	}
}

func TestStuffrRun_PersonalityGzipStdio(t *testing.T) {
	rt := testStuffr(t)
	in := randBytes(32<<20, 1)
	// Make it partly compressible so both directions do real work.
	copy(in[8<<20:], bytes.Repeat([]byte("sercon stuffr "), 1<<18))
	comp, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"gzip", "-c"}, Cwd: "/", Stdin: in})
	if err != nil || comp.Code != 0 {
		t.Fatalf("gzip -c code=%d err=%v stderr=%s", comp.Code, err, comp.Stderr)
	}
	dec, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"gzip", "-dc"}, Cwd: "/", Stdin: comp.Stdout})
	if err != nil || dec.Code != 0 {
		t.Fatalf("gzip -dc code=%d err=%v stderr=%s", dec.Code, err, dec.Stderr)
	}
	if !bytes.Equal(dec.Stdout, in) {
		t.Fatalf("round trip mismatch: got %d bytes want %d", len(dec.Stdout), len(in))
	}
}

func TestStuffrRun_PersonalityRelativeCwd(t *testing.T) {
	dir := t.TempDir()
	rt := testStuffr(t)
	comp, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"gzip", "-c"}, Cwd: "/", Stdin: []byte("persona\n")})
	if err != nil || comp.Code != 0 {
		t.Fatalf("code=%d err=%v", comp.Code, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.txt.gz"), comp.Stdout, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"gzip", "-dc", "p.txt.gz"}, Cwd: dir})
	if err != nil || res.Code != 0 || string(res.Stdout) != "persona\n" {
		t.Fatalf("code=%d err=%v out=%q stderr=%s", res.Code, err, res.Stdout, res.Stderr)
	}
}

func cancelFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), randBytes(64<<20, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStuffrRun_Cancel(t *testing.T) {
	rt := testStuffr(t)
	dir := cancelFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := rt.run(ctx, stuffrRunReq{Argv: []string{"pack", "big.bin", "-o", "big.xz", "--format", "xz"}, Cwd: dir})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("cancel took %v", d)
	}
}

func TestStuffrRun_UsableAfterCancel(t *testing.T) {
	rt := testStuffr(t)
	dir := cancelFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := rt.run(ctx, stuffrRunReq{Argv: []string{"pack", "big.bin", "-o", "big.xz", "--format", "xz"}, Cwd: dir}); err == nil {
		t.Fatal("expected cancellation error")
	}
	res, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"formats", "--json"}, Cwd: "/"})
	if err != nil || res.Code != 0 {
		t.Fatalf("after cancel: code=%d err=%v", res.Code, err)
	}
}

func TestStuffrRun_Concurrent(t *testing.T) {
	rt := testStuffr(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := []byte(strings.Repeat(fmt.Sprintf("goroutine %d\n", i), 1000))
			c, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"gzip", "-c"}, Cwd: "/", Stdin: in})
			if err != nil || c.Code != 0 {
				errs <- fmt.Errorf("g%d gzip: code=%d err=%v", i, c.Code, err)
				return
			}
			d, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"gzip", "-dc"}, Cwd: "/", Stdin: c.Stdout})
			if err != nil || d.Code != 0 || !bytes.Equal(d.Stdout, in) {
				errs <- fmt.Errorf("g%d gunzip: code=%d err=%v", i, d.Code, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestNewStuffrRuntime_UnwritableCache(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permissions are not enforced")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0o755)
	rt, err := newStuffrRuntime(context.Background(), filepath.Join(parent, "cache"))
	if err != nil {
		t.Fatalf("unusable cache must not fail: %v", err)
	}
	res, err := rt.run(context.Background(), stuffrRunReq{Argv: []string{"formats", "--json"}, Cwd: "/"})
	if err != nil || res.Code != 0 {
		t.Fatalf("code=%d err=%v", res.Code, err)
	}
	// A file where the directory should be is also unusable.
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newStuffrRuntime(context.Background(), filepath.Join(f, "sub")); err != nil {
		t.Fatalf("file-as-cache-parent must not fail: %v", err)
	}
}

func TestStuffrRun_EmptyArgv(t *testing.T) {
	_, err := testStuffr(t).run(context.Background(), stuffrRunReq{Cwd: "/"})
	if err == nil || !strings.Contains(err.Error(), "argv must not be empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyStuffrErr(t *testing.T) {
	code, err := classifyStuffrErr(context.Background(), []string{"x"}, sys.NewExitError(5))
	if code != 5 || err != nil {
		t.Fatalf("exit: code=%d err=%v", code, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := classifyStuffrErr(ctx, []string{"x"}, errors.New("boom")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx: err=%v", err)
	}
	_, err = classifyStuffrErr(context.Background(), []string{"stuffr", "list"}, errors.New("wasm error: unreachable"))
	var trap *stuffrTrapError
	if !errors.As(err, &trap) {
		t.Fatalf("want *stuffrTrapError, got %T", err)
	}
	f := trap.ErrorFields()
	if f["kind"] != "trap" || f["detail"] != "wasm error: unreachable" {
		t.Fatalf("fields = %v", f)
	}
	if !strings.Contains(trap.Error(), "stuffr stuffr trapped") {
		t.Fatalf("msg = %s", trap.Error())
	}
}

// A streamed run's stderr goes through sercon's redirectable stderr, so a
// runtime.stderr.to(fn) handler receives every completed line, in write order,
// and nothing reaches the destination beneath it. The guest writes its stderr
// in several chunks per line from wazero's goroutine while the handler runs on
// the loop — this used to trip the line-callback's re-entrancy guard and leak
// fragments beneath (see TestCallback_ConcurrentWriteDuringHandlerIsNotReentrant).
// messages holds only the non-empty lines, so empty handler lines are dropped
// before comparing.
func TestStuffrRun_StreamFollowsStderrRedirect(t *testing.T) {
	_, beneath, restoreStdio := withCapturedStdio(t)
	defer restoreStdio()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := stuffrScript(t, `
		const lines: string[] = [];
		const restore = runtime.stderr.to((l: string) => lines.push(l));
		const r = await stuffr.pack(["a.txt"], { cwd: `+jsq(dir)+`, output: `+jsq(filepath.Join(dir, "a.tar"))+`, stream: true });
		await new Promise((res) => setTimeout(res, 100));
		restore();
		runtime.log("captured", r.messages.length > 0, lines.filter((l) => l !== "").join("|") === r.messages.join("|"));
	`)
	if !strings.Contains(out, "captured true true") {
		t.Fatalf("streamed stderr not captured by runtime.stderr.to: %q", out)
	}
	if beneath.Len() != 0 {
		t.Fatalf("streamed stderr leaked beneath the redirect: %q", beneath.String())
	}
}

func TestStuffrRun_WindowsFailsFast(t *testing.T) {
	old := stuffrGOOS
	stuffrGOOS = "windows"
	defer func() { stuffrGOOS = old }()
	_, err := testStuffr(t).run(context.Background(), stuffrRunReq{Argv: []string{"formats"}, Cwd: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not supported on Windows yet") {
		t.Fatalf("err = %v, want not-supported-on-Windows error", err)
	}
}
