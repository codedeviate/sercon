package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// stuffrExitError is thrown by the typed helpers when stuffr exits non-zero.
type stuffrExitError struct {
	Argv   []string
	Code   int
	Stderr string
}

func stuffrExitKind(code int) string {
	switch code {
	case 1:
		return "failed"
	case 2:
		return "usage"
	case 3:
		return "unsupported"
	case 4:
		return "degraded"
	case 5:
		return "corrupt"
	case 6:
		return "limit"
	case 7:
		return "unsafe"
	}
	return "unknown"
}

func (e *stuffrExitError) Error() string {
	verb := ""
	if len(e.Argv) > 0 {
		verb = e.Argv[0]
	}
	first, _, _ := strings.Cut(strings.TrimSpace(e.Stderr), "\n")
	msg := fmt.Sprintf("stuffr %s exited %d (%s): %s", verb, e.Code, stuffrExitKind(e.Code), strings.TrimSpace(first))
	if e.Code == 4 {
		// Exit 4 means the work was done but something was lost or unverified.
		msg += " (output was written)"
	}
	return msg
}

// ErrorFields exposes structured fields on the thrown JS error.
func (e *stuffrExitError) ErrorFields() map[string]any {
	return map[string]any{"code": e.Code, "kind": stuffrExitKind(e.Code), "stderr": e.Stderr, "argv": e.Argv}
}

// stuffrOpts is the union of helper options and RunOptions.
type stuffrOpts struct {
	Output           string
	Format           string
	Level            *int
	Force            bool
	AllowWeakEncoder bool
	MemoryLimit      string
	Directory        string
	SamePermissions  bool

	Cwd    string
	Stdin  []byte
	Env    map[string]string
	Stream bool
}

// stuffrPath rewrites a lone "-" (which stuffr treats as stdin even after
// "--") to "./-" so it names the file.
func stuffrPath(p string) string {
	if p == "-" {
		return "./-"
	}
	return p
}

// buildStuffrArgv builds the argv for a typed helper: verb, flags, then
// positionals after "--".
func buildStuffrArgv(verb string, positional []string, o stuffrOpts) []string {
	argv := []string{verb}
	flag := func(name, val string) {
		if val != "" {
			argv = append(argv, name, val)
		}
	}
	sw := func(name string, on bool) {
		if on {
			argv = append(argv, name)
		}
	}
	level := func() {
		if o.Level != nil {
			argv = append(argv, "--level", strconv.Itoa(*o.Level))
		}
	}
	switch verb {
	case "pack":
		flag("--output", o.Output)
		flag("--format", o.Format)
		level()
		sw("--force", o.Force)
		sw("--allow-weak-encoder", o.AllowWeakEncoder)
		flag("--memory-limit", o.MemoryLimit)
	case "unpack":
		flag("--directory", o.Directory)
		sw("--same-permissions", o.SamePermissions)
		sw("--force", o.Force)
		flag("--memory-limit", o.MemoryLimit)
	case "convert":
		flag("--format", o.Format)
		level()
		sw("--force", o.Force)
		sw("--allow-weak-encoder", o.AllowWeakEncoder)
		flag("--memory-limit", o.MemoryLimit)
	case "list", "info":
		argv = append(argv, "--json")
		flag("--memory-limit", o.MemoryLimit)
	case "formats":
		argv = append(argv, "--json")
	default: // test, cat
		flag("--memory-limit", o.MemoryLimit)
	}
	if len(positional) > 0 {
		argv = append(argv, "--")
		for _, p := range positional {
			argv = append(argv, stuffrPath(p))
		}
	}
	return argv
}

// parseStuffrOpts reads an options object on the loop.
func parseStuffrOpts(v goja.Value) (stuffrOpts, error) {
	var o stuffrOpts
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return o, nil
	}
	m, ok := v.Export().(map[string]any)
	if !ok {
		return o, errors.New("options must be an object")
	}
	str := func(key string) (string, error) {
		x, ok := m[key]
		if !ok || x == nil {
			return "", nil
		}
		s, ok := x.(string)
		if !ok {
			return "", fmt.Errorf("option %q must be a string", key)
		}
		return s, nil
	}
	boolean := func(key string) bool { b, _ := m[key].(bool); return b }
	var err error
	if o.Output, err = str("output"); err != nil {
		return o, err
	}
	if o.Format, err = str("format"); err != nil {
		return o, err
	}
	if o.Directory, err = str("directory"); err != nil {
		return o, err
	}
	if o.Cwd, err = str("cwd"); err != nil {
		return o, err
	}
	o.Force = boolean("force")
	o.AllowWeakEncoder = boolean("allowWeakEncoder")
	o.SamePermissions = boolean("samePermissions")
	o.Stream = boolean("stream")
	switch x := m["level"].(type) {
	case nil:
	case int64:
		l := int(x)
		o.Level = &l
	case float64:
		l := int(x)
		o.Level = &l
	default:
		return o, errors.New(`option "level" must be a number`)
	}
	switch x := m["memoryLimit"].(type) {
	case nil:
	case string:
		o.MemoryLimit = x
	case int64:
		o.MemoryLimit = strconv.FormatInt(x, 10)
	case float64:
		o.MemoryLimit = strconv.FormatInt(int64(x), 10)
	default:
		return o, errors.New(`option "memoryLimit" must be a string or number`)
	}
	switch x := m["stdin"].(type) {
	case nil:
	case string:
		o.Stdin = []byte(x)
	case []byte:
		o.Stdin = append([]byte(nil), x...)
	case goja.ArrayBuffer:
		o.Stdin = append([]byte(nil), x.Bytes()...)
	default:
		return o, fmt.Errorf("option \"stdin\" must be a string, Uint8Array or ArrayBuffer (got %T)", x)
	}
	if x, ok := m["env"]; ok && x != nil {
		em, ok := x.(map[string]any)
		if !ok {
			return o, errors.New(`option "env" must be an object`)
		}
		o.Env = make(map[string]string, len(em))
		for k, val := range em {
			o.Env[k] = fmt.Sprint(val)
		}
	}
	return o, nil
}

// runReq turns options plus argv into a run request, resolving cwd to an
// absolute path (relative values resolve against the process cwd).
func (o stuffrOpts) runReq(argv []string) (stuffrRunReq, error) {
	cwd := o.Cwd
	if cwd == "" || !filepath.IsAbs(cwd) {
		wd, err := os.Getwd()
		if err != nil {
			return stuffrRunReq{}, fmt.Errorf("stuffr: resolve cwd: %w", err)
		}
		cwd = filepath.Join(wd, cwd)
	}
	return stuffrRunReq{Argv: argv, Cwd: cwd, Stdin: o.Stdin, Env: o.Env, Stream: o.Stream}, nil
}

// stuffrStringList reads an array of strings.
func stuffrStringList(v goja.Value, what string) ([]string, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, fmt.Errorf("%s must be an array of strings", what)
	}
	switch x := v.Export().(type) {
	case []string:
		return append([]string(nil), x...), nil
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string", what, i)
			}
			out[i] = s
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s must be an array of strings", what)
}

func stuffrString(v goja.Value, what string) (string, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "", fmt.Errorf("%s is required", what)
	}
	s, ok := v.Export().(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", what)
	}
	return s, nil
}

// stuffrCall is the on-loop-extracted input of a stuffr binding.
type stuffrCall struct {
	req    stuffrRunReq
	helper bool // throw stuffrExitError on non-zero exit
	post   func(stdout []byte) (any, error)
}

func stuffrWork(ctx context.Context, c stuffrCall) (any, error) {
	rt, err := sharedStuffrRuntime()
	if err != nil {
		return nil, err
	}
	res, err := rt.run(ctx, c.req)
	if err != nil {
		return nil, err
	}
	if c.helper {
		if res.Code != 0 {
			return nil, &stuffrExitError{Argv: c.req.Argv, Code: res.Code, Stderr: string(res.Stderr)}
		}
		return c.post(res.Stdout)
	}
	return &stuffrRunOut{Code: res.Code, Stdout: res.Stdout, Stderr: string(res.Stderr)}, nil
}

// stuffrRunOut and stuffrBytesOut carry raw results from the work goroutine
// to stuffrOnLoop, which builds the JS values (Uint8Array, text()) on the loop.
type stuffrRunOut struct {
	Code   int
	Stdout []byte
	Stderr string
}

type stuffrBytesOut struct{ B []byte }

func newUint8Array(vm *goja.Runtime, b []byte) goja.Value {
	if b == nil {
		b = []byte{}
	}
	u8, err := vm.New(vm.Get("Uint8Array"), vm.ToValue(vm.NewArrayBuffer(b)))
	if err != nil {
		panic(vm.NewGoError(err))
	}
	return u8
}

// stuffrOnLoop wraps an async binding so raw results are converted on the loop
// (goja's ToValue maps a Go []byte to a plain Array, not a Uint8Array).
func stuffrOnLoop(vm *goja.Runtime, ab scriptengine.AsyncBinding, ts string) scriptengine.AsyncBinding {
	conv := func(call goja.FunctionCall) goja.Value {
		switch x := call.Argument(0).Export().(type) {
		case *stuffrRunOut:
			stdout := x.Stdout
			obj := vm.NewObject()
			_ = obj.Set("code", x.Code)
			_ = obj.Set("stdout", newUint8Array(vm, stdout))
			_ = obj.Set("stderr", x.Stderr)
			_ = obj.Set("text", func() string { return string(stdout) })
			return obj
		case *stuffrBytesOut:
			return newUint8Array(vm, x.B)
		}
		return call.Argument(0)
	}
	fn := func(call goja.FunctionCall) goja.Value {
		p := ab.Func(call)
		then, ok := goja.AssertFunction(p.ToObject(vm).Get("then"))
		if !ok {
			return p
		}
		out, err := then(p, vm.ToValue(conv))
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return out
	}
	return scriptengine.AsyncBinding{Func: fn, TSReturnType: ts}
}

func stuffrNone([]byte) (any, error)    { return nil, nil }
func stuffrBytes(b []byte) (any, error) { return &stuffrBytesOut{B: b}, nil }
func stuffrJSON(b []byte) (any, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("stuffr: parse JSON output: %w", err)
	}
	return v, nil
}

// stuffrNamespace builds the `stuffr` global. It never dereferences vm or
// loop outside the per-call closures, so d.ts introspection (nil, nil) is safe.
func stuffrNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	// helper wraps a typed verb: argsFn extracts positionals + the options
	// argument index from the call.
	helper := func(verb string, nPos int, optIdx int, variadic bool, post func([]byte) (any, error), ts string) scriptengine.AsyncBinding {
		return stuffrOnLoop(vm, scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (stuffrCall, error) {
				var pos []string
				if variadic {
					list, err := stuffrStringList(call.Argument(0), "inputs")
					if err != nil {
						return stuffrCall{}, fmt.Errorf("stuffr.%s: %w", verb, err)
					}
					if len(list) == 0 {
						return stuffrCall{}, fmt.Errorf("stuffr.%s: inputs must not be empty", verb)
					}
					pos = list
				}
				for i := 0; i < nPos; i++ {
					s, err := stuffrString(call.Argument(i), fmt.Sprintf("argument %d", i+1))
					if err != nil {
						return stuffrCall{}, fmt.Errorf("stuffr.%s: %w", verb, err)
					}
					pos = append(pos, s)
				}
				o, err := parseStuffrOpts(call.Argument(optIdx))
				if err != nil {
					return stuffrCall{}, fmt.Errorf("stuffr.%s: %w", verb, err)
				}
				req, err := o.runReq(buildStuffrArgv(verb, pos, o))
				if err != nil {
					return stuffrCall{}, err
				}
				return stuffrCall{req: req, helper: true, post: post}, nil
			}, stuffrWork), ts)
	}
	run := stuffrOnLoop(vm, scriptengine.PromisifyAsync(vm, loop,
		func(call goja.FunctionCall) (stuffrCall, error) {
			argv, err := stuffrStringList(call.Argument(0), "argv")
			if err != nil {
				return stuffrCall{}, fmt.Errorf("stuffr.run: %w", err)
			}
			if len(argv) == 0 {
				return stuffrCall{}, errors.New("stuffr.run: argv must not be empty")
			}
			o, err := parseStuffrOpts(call.Argument(1))
			if err != nil {
				return stuffrCall{}, fmt.Errorf("stuffr.run: %w", err)
			}
			req, err := o.runReq(argv)
			if err != nil {
				return stuffrCall{}, err
			}
			return stuffrCall{req: req}, nil
		}, stuffrWork), "{ code: number; stdout: Uint8Array; stderr: string; text(): string }")
	return map[string]any{
		"run":     run,
		"pack":    helper("pack", 0, 1, true, stuffrNone, "void"),
		"unpack":  helper("unpack", 1, 1, false, stuffrNone, "void"),
		"convert": helper("convert", 2, 2, false, stuffrNone, "void"),
		"test":    helper("test", 1, 1, false, stuffrNone, "void"),
		"cat":     helper("cat", 1, 1, false, stuffrBytes, "Uint8Array"),
		"list":    helper("list", 1, 1, false, stuffrJSON, "unknown"),
		"info":    helper("info", 1, 1, false, stuffrJSON, "unknown"),
		"formats": helper("formats", 0, 0, false, stuffrJSON, "unknown"),
		"version": stuffrVersion(),
	}
}
