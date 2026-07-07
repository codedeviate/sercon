package tui_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// lockedWriter serialises writes to the underlying sink so the race
// detector can only flag races inside the pane write path, not in the
// test's own output buffer.
type lockedWriter struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

// Bug 1 (concurrency): two writers targeting the same pane concurrently —
// e.g. Promise.all([exec.shell({pane:"log"}), exec.shell({pane:"log"})]),
// which produces two paneIOWriters with separate mutexes, plus a JS-loop
// pane.write — all mutate the same paneState's FallbackPane/ANSITranslator.
// Run under `go test -race`: unsynchronised access to the shared pane state
// must not race.
func TestPane_ConcurrentWritesNoRace(t *testing.T) {
	root, err := tui.ParseLayout(map[string]any{
		"rows": []any{map[string]any{"name": "log"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartFallback(&lockedWriter{}); err != nil {
		t.Fatal(err)
	}
	pane := c.Pane("log")

	var wg sync.WaitGroup
	// Two io.Writer adapters (distinct paneIOWriters) + direct pane writes,
	// all hammering the same paneState.
	writers := []func(){
		func() {
			w := pane.AsWriter()
			for i := 0; i < 200; i++ {
				_, _ = w.Write([]byte("stdout line\n"))
			}
		},
		func() {
			w := pane.AsWriter()
			for i := 0; i < 200; i++ {
				_, _ = w.Write([]byte("stderr line\n"))
			}
		},
		func() {
			for i := 0; i < 200; i++ {
				pane.Writeln("js write")
			}
		},
	}
	for _, fn := range writers {
		wg.Add(1)
		go func(f func()) { defer wg.Done(); f() }(fn)
	}
	wg.Wait()
	c.Stop()
}

// Bug 2 (correctness): a literal "[INFO]" in pane output must survive.
// tview's escape form is "[tag[]" (insert "[" before the closing "]"),
// not "[[..."; the old "[" -> "[[" doubling made tview parse "[INFO]" as
// a style tag and swallow the token. Real color tags emitted by the
// translator must stay unescaped.
func TestANSI_BracketTokenSurvives(t *testing.T) {
	tr := tui.NewANSITranslator()
	if got := tr.Translate("[INFO] started"); got != "[INFO[] started" {
		t.Errorf("literal bracket token: got %q, want %q", got, "[INFO[] started")
	}

	tr2 := tui.NewANSITranslator()
	// A real SGR color tag precedes a literal bracketed token: the color
	// tag stays a live tview tag, the literal token is escaped. (SGR 31 is
	// basic red, which maps to tview "darkred"; bright red 91 -> "red".)
	if got := tr2.Translate("\x1b[31m[ERROR]"); got != "[darkred:-:-][ERROR[]" {
		t.Errorf("color tag + bracket token: got %q, want %q", got, "[darkred:-:-][ERROR[]")
	}
}

// Bug 3 (correctness): CRLF-terminated output (e.g. exec.shell pty:true,
// where ONLCR turns every \n into \r\n) must not have its line content
// wiped by treating the \r as a bare line-reset.
func TestFallbackPane_CRLFPreservesContent(t *testing.T) {
	var b strings.Builder
	fp := tui.NewFallbackPane(&b, "x")
	_, _ = fp.Write([]byte("done\r\nnext line\r\n"))
	got := b.String()
	want := "[x] done\n[x] next line\n"
	if got != want {
		t.Errorf("CRLF stream: got %q, want %q", got, want)
	}
}

// Bug 3, chunk boundary: a \r ending one Write and the \n opening the
// next must still be recognised as one CRLF, not a reset then empty line.
func TestFallbackPane_CRLFAcrossChunks(t *testing.T) {
	var b strings.Builder
	fp := tui.NewFallbackPane(&b, "x")
	_, _ = fp.Write([]byte("done\r"))
	_, _ = fp.Write([]byte("\nmore\r\n"))
	got := b.String()
	want := "[x] done\n[x] more\n"
	if got != want {
		t.Errorf("CRLF across chunks: got %q, want %q", got, want)
	}
}

// Bug 3 regression guard: a *bare* \r (spinner overwrite, no following \n)
// must still reset the pending line so the latest frame wins.
func TestFallbackPane_BareCROverwrites(t *testing.T) {
	var b strings.Builder
	fp := tui.NewFallbackPane(&b, "x")
	_, _ = fp.Write([]byte("10%\r50%\r100%\n"))
	got := b.String()
	want := "[x] 100%\n"
	if got != want {
		t.Errorf("bare CR overwrite: got %q, want %q", got, want)
	}
}
