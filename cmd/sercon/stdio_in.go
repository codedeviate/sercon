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
// A single mutex serialises reads as well as source swaps, so two concurrent
// readLine() calls cannot interleave halves of a line.
type inSource struct {
	mu     sync.Mutex
	base   inEntry
	stack  []inEntry
	nextID uint64
}

var stdioInSource = &inSource{
	base: inEntry{kind: "stdin", r: bufio.NewReader(os.Stdin)},
}

// active returns the effective entry. Called with mu held.
func (s *inSource) active() *inEntry {
	if n := len(s.stack); n > 0 {
		return &s.stack[n-1]
	}
	return &s.base
}

// read drains the active source.
//
// Known limitation: a blocking read against the real process stdin cannot be
// interrupted by the run's deadline — the run is still killed, but this
// goroutine parks until the pipe closes. File and string sources have no such
// issue, since they never block indefinitely.
func (s *inSource) read() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return io.ReadAll(s.active().r)
}

// readLine returns the next line without its newline. ok is false at EOF. A
// final line with no trailing newline is still returned.
func (s *inSource) readLine() (line string, ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, err := s.active().r.ReadString('\n')
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
	s.mu.Lock()
	s.nextID++
	e.id = s.nextID
	id := e.id
	s.stack = append(s.stack, e)
	s.mu.Unlock()

	var once sync.Once
	return func() { once.Do(func() { s.pop(id) }) }
}

func (s *inSource) pop(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.stack {
		if s.stack[i].file != nil {
			_ = s.stack[i].file.Close()
		}
	}
	s.stack = nil
}

// sourceInfo describes the active source for runtime.stdin.source().
func (s *inSource) sourceInfo() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// installLinesIterProg makes the object returned by lines() an AsyncIterable.
// Same pattern as server_ws.go: *goja.Program is safe to share across
// runtimes, so compile once and run per-VM.
var installLinesIterProg = goja.MustCompile("internal:install-stdin-lines-iter",
	`(obj) => { obj[Symbol.asyncIterator] = function() { return this; }; }`, false)

// wrapStdinScopedFnProg wraps stdin.scoped's callback in a compiled async
// function that hands its resolved value to a Go-side capture callback right
// before the wrapper's own promise settles.
//
// This exists because stdin.scoped must resolve to the CALLBACK's own value
// (unlike the output-side scoped, which always resolves to undefined —
// see stdio_bindings.go's callScopedFn/settleAfter). Those two helpers only
// support a fixed `value func() goja.Value`, evaluated after cleanup; they
// have no hook to observe what the callback's promise actually resolved
// with. Rather than re-deriving that here in Go — which would mean
// re-reading `result.then` ourselves and re-implementing the exact
// panic-on-throwing-getter guard `callScopedFn` already carries (a Critical
// finding from Task 5's review) — the capture happens in JS, where member
// access and `await` are native operations with no equivalent Go-level
// Object.Get panic to guard against. The wrapper is still driven through
// callScopedFn/settleAfter unchanged for the actual chaining, cleanup
// ordering, and error fidelity; only the resolved-value capture is new.
//
// `await fn()` resolves `captured` (via the closure below) strictly before
// the wrapper's own `return v` settles its promise, so by the time
// settleAfter's onOK fires on that same promise, `captured` already holds
// the real value.
var wrapStdinScopedFnProg = goja.MustCompile("internal:wrap-stdin-scoped-fn",
	`(fn, capture) => { return async function() { const v = await fn(); capture(v); return v; }; }`, false)

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
		func(_ context.Context, _ struct{}) (any, error) {
			b, err := s.read()
			if err != nil {
				return nil, err
			}
			return string(b), nil
		})

	readBytes := scriptengine.PromisifyAsync(vm, loop,
		func(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil },
		func(_ context.Context, _ struct{}) (any, error) { return s.read() })

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
			installVal, err := vm.RunProgram(installLinesIterProg)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("lines: install async iterator: %w", err)))
			}
			installFn, ok := goja.AssertFunction(installVal)
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("lines: install async iterator: not callable")))
			}
			if _, err := installFn(goja.Undefined(), vm.ToValue(obj)); err != nil {
				panic(vm.NewGoError(fmt.Errorf("lines: install async iterator: %w", err)))
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
			fnArg := call.Argument(1)
			if _, ok := goja.AssertFunction(fnArg); !ok {
				panic(vm.NewGoError(fmt.Errorf("scoped: second argument must be a function")))
			}
			entry, err := parseInSource(vm, call.Argument(0))
			if err != nil {
				panic(vm.NewGoError(err))
			}

			// See wrapStdinScopedFnProg's doc: the wrapper captures fn's
			// resolved value into `captured` before its own promise settles,
			// so callScopedFn/settleAfter can be reused unchanged while still
			// resolving stdin.scoped to the callback's own value.
			wrapVal, err := vm.RunProgram(wrapStdinScopedFnProg)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("scoped: %w", err)))
			}
			wrapFn, ok := goja.AssertFunction(wrapVal)
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("scoped: value wrapper is not callable")))
			}
			captured := goja.Undefined()
			captureFn := vm.ToValue(func(c goja.FunctionCall) goja.Value {
				captured = c.Argument(0)
				return goja.Undefined()
			})
			wrappedVal, err := wrapFn(goja.Undefined(), fnArg, captureFn)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("scoped: %w", err)))
			}
			wrapped, ok := goja.AssertFunction(wrappedVal)
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("scoped: wrapped callback is not callable")))
			}

			restore := s.push(entry)
			return callScopedFn(vm, wrapped, restore, func() goja.Value { return captured })
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
		return inEntry{kind: "stdin", r: bufio.NewReader(os.Stdin)}, nil
	}
	return inEntry{}, fmt.Errorf("from: unknown source %q (want { file }, { text }, or \"stdin\")", v.String())
}
