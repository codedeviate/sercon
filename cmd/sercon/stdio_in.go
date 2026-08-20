package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/term"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// inEntry is one source on the stdin stack.
type inEntry struct {
	kind string // "stdin" | "file" | "text"
	path string
	r    *bufio.Reader
	file *os.File // owned when kind == "file"
	id   uint64
}

// inSource is the stdin side of the registry: a stack of sources with one
// bufio.Reader over the effective one.
//
// Two separate mutexes, deliberately not one:
//
//   - stateMu guards the stack/nextID structure (push, pop, reset,
//     sourceInfo). It is only ever held briefly, never across a blocking
//     read.
//   - readMu serialises the actual blocking reads (read, readLine) against
//     each other, so two concurrent readLine() calls cannot interleave
//     halves of a line.
//
// A single shared mutex across both would mean a blocking read against the
// real process stdin (a TTY, or a pipe that's never closed) parks holding
// it — and resetStdio() (called at the START of every Run, including
// --watch re-runs) would then hang forever waiting for stateMu, since it
// runs on the main goroutine. Splitting the locks means reset/pop/push/
// source() never wait on an in-flight read: a reset or pop that closes a
// file out from under a concurrent read just makes that read error out
// (or, for the real stdin, leaves it parked — see read()'s doc — but
// without blocking anything else).
//
// read()/readLine() snapshot the active entry's *bufio.Reader under stateMu
// before starting the actual (unlocked-by-stateMu) I/O, then serialise the
// I/O itself under readMu.
type inSource struct {
	readMu sync.Mutex

	stateMu sync.Mutex
	base    inEntry
	stack   []inEntry
	nextID  uint64
}

var stdioInSource = &inSource{
	base: inEntry{kind: "stdin", r: bufio.NewReader(os.Stdin)},
}

// active returns the effective entry. Called with stateMu held.
func (s *inSource) active() *inEntry {
	if n := len(s.stack); n > 0 {
		return &s.stack[n-1]
	}
	return &s.base
}

// activeReader snapshots the effective entry's reader under stateMu, for
// callers that then read from it without holding stateMu (see the type doc).
func (s *inSource) activeReader() *bufio.Reader {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.active().r
}

// read drains the active source.
//
// Known limitation: a blocking read against the real process stdin cannot be
// interrupted by the run's deadline — the run is still killed, but this
// goroutine parks until the pipe closes. File and string sources have no such
// issue, since they never block indefinitely. Because the lock held across
// that block is readMu, not stateMu, a stuck read never blocks push/pop/
// reset/source() on later Runs — see the type doc.
func (s *inSource) read() ([]byte, error) {
	r := s.activeReader()
	s.readMu.Lock()
	defer s.readMu.Unlock()
	return io.ReadAll(r)
}

// readLine returns the next line without its newline. ok is false at EOF. A
// final line with no trailing newline is still returned.
func (s *inSource) readLine() (line string, ok bool, err error) {
	r := s.activeReader()
	s.readMu.Lock()
	defer s.readMu.Unlock()
	text, err := r.ReadString('\n')
	switch {
	case err == nil:
		return strings.TrimSuffix(text, "\n"), true, nil
	case errors.Is(err, io.EOF) && text != "":
		return text, true, nil
	case errors.Is(err, io.EOF):
		return "", false, nil
	default:
		return "", false, err
	}
}

func (s *inSource) push(e inEntry) (restore func()) {
	s.stateMu.Lock()
	s.nextID++
	e.id = s.nextID
	id := e.id
	s.stack = append(s.stack, e)
	s.stateMu.Unlock()

	var once sync.Once
	return func() { once.Do(func() { s.pop(id) }) }
}

func (s *inSource) pop(id uint64) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for i := range s.stack {
		if s.stack[i].id != id {
			continue
		}
		if s.stack[i].file != nil {
			_ = s.stack[i].file.Close()
		}
		s.stack = append(s.stack[:i], s.stack[i+1:]...)
		return
	}
}

// reset drops every source swap, closing any files they opened.
func (s *inSource) reset() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	for i := range s.stack {
		if s.stack[i].file != nil {
			_ = s.stack[i].file.Close()
		}
	}
	s.stack = nil
}

// sourceInfo describes the active source for runtime.stdin.source().
func (s *inSource) sourceInfo() map[string]any {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	e := s.active()
	info := map[string]any{"kind": e.kind}
	if e.path != "" {
		info["path"] = e.path
	}
	// tty is only meaningful for the real process stdin; a file or a string is
	// never a terminal.
	info["tty"] = e.kind == "stdin" && term.IsTerminal(int(os.Stdin.Fd()))
	return info
}

// textEntry / fileEntry build the two swappable sources.
func textEntry(text string) inEntry {
	return inEntry{kind: "text", r: bufio.NewReader(strings.NewReader(text))}
}

func fileEntry(path string) (inEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return inEntry{}, err
	}
	return inEntry{kind: "file", path: path, r: bufio.NewReader(f), file: f}, nil
}

// inStreamBinding builds the runtime.stdin handle.
func inStreamBinding(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine) map[string]any {
	s := stdioInSource

	// Reads block until data or EOF. They run off-loop through
	// PromisifyAsync, so the script's await does not stall the loop.
	//
	// Known limitation: a blocking read on the real process stdin cannot be
	// interrupted by the run's deadline — the run is still killed, but the
	// read goroutine parks until the pipe closes. Reading from a file or a
	// string source has no such issue.
	//
	// Separate known limitation: when the script itself was read from stdin
	// (`sercon -`), stdin is already drained by the time the script runs, so
	// read() / readLine() / lines() simply observe EOF immediately.
	read := scriptengine.PromisifyAsync(vm, loop,
		func(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
		func(_ context.Context, _ struct{}) (string, error) {
			b, err := s.read()
			if err != nil {
				return "", err
			}
			return string(b), nil
		})

	readBytes := scriptengine.PromisifyAsync(vm, loop,
		func(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
		func(_ context.Context, _ struct{}) ([]byte, error) { return s.read() })

	// readLine resolves to a string or to null (EOF), so its resolved type
	// can't be pinned down to one Go type — it stays `any`.
	readLine := scriptengine.PromisifyAsync(vm, loop,
		func(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
		func(_ context.Context, _ struct{}) (any, error) {
			line, ok, err := s.readLine()
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, nil // resolves to null
			}
			return line, nil
		})

	return map[string]any{
		"read":      read,
		"readBytes": readBytes,
		"readLine":  readLine,
		"lines": func(goja.FunctionCall) goja.Value {
			obj := vm.NewObject()
			if err := obj.Set("next", readLinesNext(vm, loop, s)); err != nil {
				panic(vm.NewGoError(fmt.Errorf("lines: %w", err)))
			}
			if err := installAsyncIterator(vm, obj); err != nil {
				panic(vm.NewGoError(fmt.Errorf("lines: %w", err)))
			}
			return vm.ToValue(obj)
		},
		"from": func(call goja.FunctionCall) goja.Value {
			entry, err := parseInSource(vm, call.Argument(0))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return restoreFn(vm, s.push(entry))
		},
		"fromFile": func(call goja.FunctionCall) goja.Value {
			entry, err := fileEntry(call.Argument(0).String())
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("fromFile: %w", err)))
			}
			return restoreFn(vm, s.push(entry))
		},
		"fromString": func(call goja.FunctionCall) goja.Value {
			return restoreFn(vm, s.push(textEntry(call.Argument(0).String())))
		},
		"reset": func(goja.FunctionCall) goja.Value {
			s.reset()
			return goja.Undefined()
		},
		"source": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(s.sourceInfo())
		},
		"scoped": func(call goja.FunctionCall) goja.Value {
			fn, ok := goja.AssertFunction(call.Argument(1))
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("scoped: second argument must be a function")))
			}
			entry, err := parseInSource(vm, call.Argument(0))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			restore := s.push(entry)
			// Unlike the output-side scoped (always undefined) or capture
			// (always the buffer), stdin.scoped resolves to the callback's
			// own resolved value: settleAfter's value callback now receives
			// that value directly (call.Argument(0) from the thenable's
			// onOK, or the plain result in the synchronous path), so this
			// is just the identity function.
			return callScopedFn(vm, fn, restore, func(v goja.Value) goja.Value { return v })
		},
	}
}

// readLinesNext builds the async iterator's next(), resolving
// { value, done } per the iteration protocol.
//
// obj.Set below is a plain goja.Object.Set at script-run time, not a
// registration walked by the engine's unwrapAsyncBindings — so this must
// return the bare callable (AsyncBinding.Func), not the AsyncBinding carrier
// itself, or goja sees a non-callable Go struct and `for await` fails with
// "Not a function".
func readLinesNext(vm *goja.Runtime, loop *eventloop.EventLoop, s *inSource) func(goja.FunctionCall) goja.Value {
	return scriptengine.PromisifyAsync(vm, loop,
		func(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
		func(_ context.Context, _ struct{}) (any, error) {
			line, ok, err := s.readLine()
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]any{"done": true}, nil
			}
			return map[string]any{"value": line, "done": false}, nil
		}).Func
}

// parseInSource turns { file } | { text } | "stdin" into an inEntry.
func parseInSource(vm *goja.Runtime, v goja.Value) (inEntry, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return inEntry{}, fmt.Errorf("from: a source is required ({ file } | { text } | \"stdin\")")
	}
	if obj, ok := v.(*goja.Object); ok {
		if fv := obj.Get("file"); fv != nil && !goja.IsUndefined(fv) && !goja.IsNull(fv) {
			return fileEntry(fv.String())
		}
		if tv := obj.Get("text"); tv != nil && !goja.IsUndefined(tv) && !goja.IsNull(tv) {
			return textEntry(tv.String()), nil
		}
		return inEntry{}, fmt.Errorf("from: object source needs a `file` or `text` property")
	}
	if v.String() == "stdin" {
		// SHARE the base entry's reader rather than wrapping os.Stdin a second
		// time. A fresh bufio.Reader over fd 0 cannot see whatever the base
		// reader has already buffered (so a script that swaps in a fixture and
		// then pushes "stdin" back to read real input sees a false EOF), and
		// anything the second reader buffers is discarded when it is popped.
		//
		// base.r is assigned once, in stdioInSource's initialiser, and never
		// mutated — so no stateMu is needed to read it. file stays nil: this
		// entry does not own os.Stdin and must never close it on pop.
		return inEntry{kind: "stdin", r: stdioInSource.base.r}, nil
	}
	return inEntry{}, fmt.Errorf("from: unknown source %q (want { file }, { text }, or \"stdin\")", v.String())
}
