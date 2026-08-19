package main

import (
	"os"
	"path/filepath"
	"strconv"
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

// readBytes returns raw bytes as a Uint8Array.
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
