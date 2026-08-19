package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fromString swaps the source; read() drains it.
func TestStdin_FromStringRead(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("hello\nworld\n");
		export default await runtime.stdin.read();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "hello\nworld\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// from("stdin") pushes the REAL process stdin as a new entry — documented as
// exactly that — so it must share the base entry's reader rather than wrapping
// fd 0 a second time. A second bufio.Reader cannot see what the base reader has
// already buffered (so a script that swaps in a fixture and then pushes "stdin"
// back to read real input saw a false EOF), and anything it buffers itself is
// thrown away when it is popped.
func TestStdin_FromStdinSharesTheBaseReader(t *testing.T) {
	// Swap the whole source so "the real stdin" is a deterministic fixture
	// rather than whatever fd 0 happens to be under `go test`.
	old := stdioInSource
	stdioInSource = &inSource{base: inEntry{kind: "stdin", r: bufio.NewReader(strings.NewReader("a\nb\nc\n"))}}
	t.Cleanup(func() {
		stdioInSource.reset()
		stdioInSource = old
	})

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		const first = await runtime.stdin.readLine();
		const restore = runtime.stdin.from("stdin");
		const second = await runtime.stdin.readLine();
		restore();
		const third = await runtime.stdin.readLine();
		const show = (v) => v === null ? "EOF" : v;
		export default [show(first), show(second), show(third)].join("|");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// b, not EOF: the pushed entry continues the same reader. And c afterwards:
	// popping it must not have discarded buffered bytes (nor closed os.Stdin —
	// the entry owns no file).
	if got, want := val.Export(), "a|b|c"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// readLine returns one line at a time and null at EOF.
func TestStdin_ReadLineAndEOF(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("a\nb\n");
		const first = await runtime.stdin.readLine();
		const second = await runtime.stdin.readLine();
		const third = await runtime.stdin.readLine();
		export default first + "|" + second + "|" + (third === null ? "EOF" : third);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "a|b|EOF"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A final line without a trailing newline is still returned.
func TestStdin_ReadLineNoTrailingNewline(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("only");
		const a = await runtime.stdin.readLine();
		const b = await runtime.stdin.readLine();
		export default a + "|" + (b === null ? "EOF" : b);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "only|EOF"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// lines() is an async iterable.
func TestStdin_LinesAsyncIterator(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("x\ny\nz\n");
		const got = [];
		for await (const line of runtime.stdin.lines()) { got.push(line); }
		export default got.join(",");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "x,y,z"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// readBytes returns raw bytes as a Go-slice-backed array-like object (goja
// wraps []byte this way, not as an actual Uint8Array — instanceof Uint8Array
// is false; length and indexing are what matter here).
func TestStdin_ReadBytes(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("AB");
		const b = await runtime.stdin.readBytes();
		export default b.length + ":" + b[0] + "," + b[1];
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "2:65,66"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// fromFile reads from disk; the restore fn puts the previous source back.
func TestStdin_FromFileAndRestore(t *testing.T) {
	defer stdioInSource.reset()

	path := filepath.Join(t.TempDir(), "in.txt")
	if err := os.WriteFile(path, []byte("from-disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("outer\n");
		const r = runtime.stdin.fromFile(`+strconv.Quote(path)+`);
		const inner = await runtime.stdin.readLine();
		r();
		const outer = await runtime.stdin.readLine();
		export default inner + "|" + outer;
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "from-disk|outer"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// fromFile on a missing path throws at the call site.
func TestStdin_FromFileMissingThrows(t *testing.T) {
	defer stdioInSource.reset()

	bad := filepath.Join(t.TempDir(), "nope.txt")
	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `runtime.stdin.fromFile(`+strconv.Quote(bad)+`);`); err == nil {
		t.Fatal("expected a throw for a missing file")
	}
}

// scoped swaps the source for the callback's duration.
func TestStdin_Scoped(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("outer\n");
		const inner = await runtime.stdin.scoped({ text: "inner\n" }, async () => {
			return await runtime.stdin.readLine();
		});
		const outer = await runtime.stdin.readLine();
		export default inner + "|" + outer;
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "inner|outer"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// source() describes the active source.
func TestStdin_SourceInfo(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("x");
		export default runtime.stdin.source().kind;
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := val.Export(); got != "text" {
		t.Fatalf("got %v want \"text\"", got)
	}
}

// scoped restores the outer source even when the callback throws
// synchronously, and the throw propagates with the original Error intact.
// Mirrors TestBinding_ScopedRestoresOnThrow (stdio_test.go) for the input
// side, where settleAfter's value callback now has to be exercised through
// the callback-error path rather than the fixed-value path.
func TestStdin_ScopedRestoresOnThrow(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("outer\n");
		let threw = false;
		try {
			await runtime.stdin.scoped({ text: "inner\n" }, () => { throw new Error("boom"); });
		} catch (e) {
			threw = true;
			runtime.assert.ok(e instanceof Error, "caught value must be an Error");
			runtime.assert.ok(e.message === "boom", "message must round-trip, got " + e.message);
		}
		runtime.assert.ok(threw, "the throw must propagate");
		export default await runtime.stdin.readLine();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "outer"; got != want {
		t.Fatalf("got %q want %q — source was not restored", got, want)
	}
}

// scoped restores the outer source even when reading the callback's
// result's `then` property panics (a throwing getter, or a revoked Proxy).
// Mirrors TestBinding_ScopedRestoresOnThenGetterThrow.
func TestStdin_ScopedRestoresOnThenGetterThrow(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("outer\n");
		let threw = false;
		try {
			await runtime.stdin.scoped({ text: "inner\n" }, () => ({ get then() { throw new Error("boom"); } }));
		} catch (e) {
			threw = true;
		}
		runtime.assert.ok(threw, "the throw must propagate");
		export default await runtime.stdin.readLine();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "outer"; got != want {
		t.Fatalf("got %q want %q — source was not restored", got, want)
	}
}

// scoped restores the outer source when an async callback's returned
// promise REJECTS (as opposed to a synchronous throw). Mirrors
// TestBinding_ScopedRestoresOnAsyncReject.
func TestStdin_ScopedRestoresOnAsyncReject(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("outer\n");
		let threw = false;
		try {
			await runtime.stdin.scoped({ text: "inner\n" }, async () => {
				await runtime.time.sleep(5);
				throw new Error("boom");
			});
		} catch (e) {
			threw = true;
			runtime.assert.ok(e instanceof Error, "caught value must be an Error");
			runtime.assert.ok(e.message === "boom", "message must round-trip, got " + e.message);
		}
		runtime.assert.ok(threw, "the throw must propagate");
		export default await runtime.stdin.readLine();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "outer"; got != want {
		t.Fatalf("got %q want %q — source was not restored", got, want)
	}
}

// scoped preserves the thrown Error's identity when calling result.then
// itself throws synchronously (distinct from the async-reject case above:
// this hits settleAfter's OTHER cleanup+reject site). Mirrors
// TestBinding_ScopedRestoresOnThenCallThrow.
func TestStdin_ScopedRestoresOnThenCallThrow(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("outer\n");
		let threw = false;
		try {
			await runtime.stdin.scoped({ text: "inner\n" }, () => ({
				then() { throw new Error("boom"); },
			}));
		} catch (e) {
			threw = true;
			runtime.assert.ok(e instanceof Error, "caught value must be an Error");
			runtime.assert.ok(e.message === "boom", "message must round-trip, got " + e.message);
		}
		runtime.assert.ok(threw, "the throw must propagate");
		export default await runtime.stdin.readLine();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "outer"; got != want {
		t.Fatalf("got %q want %q — source was not restored", got, want)
	}
}

// Two readLine() calls kicked off concurrently (via Promise.all, which
// invokes PromisifyAsync's on-loop extract for both before either goroutine
// runs) must not interleave halves of a line. This exercises inSource's
// readMu directly — the thing that has to keep serialising reads once
// stateMu/readMu are split (see inSource's doc in stdio_in.go), and it fails
// under -race if the read path is ever left unsynchronised.
func TestStdin_ReadLineConcurrentNoInterleave(t *testing.T) {
	defer stdioInSource.reset()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdin.fromString("aaaa\nbbbb\n");
		const [a, b] = await Promise.all([runtime.stdin.readLine(), runtime.stdin.readLine()]);
		export default [a, b].sort().join("|");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "aaaa|bbbb"; got != want {
		t.Fatalf("got %q want %q — a line was split or interleaved", got, want)
	}
}
