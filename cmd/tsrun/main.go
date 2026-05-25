package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/tsrun/pkg/scriptengine"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tsrun:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tsrun", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 10*time.Second, "Per-script timeout")
	root := fs.String("root", "", "Script root for require resolution (default: dirname of first script)")
	emitDTS := fs.String("emit-dts", "", "Write the .d.ts for the example bindings to this path and exit")
	verbose := fs.Bool("v", false, "Verbose: log timing and full transpile output on error")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tsrun [flags] <script.ts> [script.ts ...]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	scripts := fs.Args()
	if *emitDTS == "" && len(scripts) == 0 {
		fs.Usage()
		return errors.New("no scripts given")
	}

	scriptRoot := *root
	if scriptRoot == "" && len(scripts) > 0 {
		abs, err := filepath.Abs(scripts[0])
		if err != nil {
			return err
		}
		scriptRoot = filepath.Dir(abs)
	}

	eng := scriptengine.New(scriptengine.Options{
		Timeout:    *timeout,
		ScriptRoot: scriptRoot,
	})
	if err := registerExampleAPI(eng); err != nil {
		return err
	}

	if *emitDTS != "" {
		f, err := os.Create(*emitDTS)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := eng.WriteTypes(f); err != nil {
			return err
		}
		if len(scripts) == 0 {
			return nil
		}
	}

	anyFail := false
	for _, s := range scripts {
		if err := runOne(eng, s, *verbose); err != nil {
			anyFail = true
			fmt.Printf("FAIL %s: %s\n", s, err)
		}
	}
	if anyFail {
		return errors.New("one or more scripts failed")
	}
	return nil
}

func runOne(eng *scriptengine.Engine, path string, verbose bool) error {
	start := time.Now()
	_, err := eng.RunFile(context.Background(), path)
	dur := time.Since(start)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "  duration: %s\n", dur)
		}
		return err
	}
	fmt.Printf("PASS %s (%s)\n", path, dur.Round(time.Millisecond))
	return nil
}

// registerExampleAPI wires the small example binding surface advertised by
// the README: api.log, api.assert.*, api.http.*, api.time.*, api.env.get.
func registerExampleAPI(e *scriptengine.Engine) error {
	if err := e.RegisterNamespaceFactory("api", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"log": func(call goja.FunctionCall) goja.Value {
				parts := make([]string, 0, len(call.Arguments))
				for _, a := range call.Arguments {
					parts = append(parts, a.String())
				}
				fmt.Println(strings.Join(parts, " "))
				return goja.Undefined()
			},
			"assert": map[string]any{
				"equal": func(actual, expected goja.Value, args ...goja.Value) {
					if !valuesEqual(actual, expected) {
						msg := "assert.equal failed"
						if len(args) > 0 && args[0] != nil && !goja.IsUndefined(args[0]) {
							msg = args[0].String()
						}
						panic(vm.NewGoError(fmt.Errorf("%s: expected %s, got %s", msg, expected.String(), actual.String())))
					}
				},
				"ok": func(cond goja.Value, args ...goja.Value) {
					if cond == nil || !cond.ToBoolean() {
						msg := "assert.ok failed"
						if len(args) > 0 && args[0] != nil && !goja.IsUndefined(args[0]) {
							msg = args[0].String()
						}
						panic(vm.NewGoError(errors.New(msg)))
					}
				},
			},
			"http": map[string]any{
				"get": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
					url := call.Argument(0).String()
					return httpDo(ctx, http.MethodGet, url, "")
				}),
				"post": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
					url := call.Argument(0).String()
					body := ""
					if len(call.Arguments) > 1 {
						body = call.Argument(1).String()
					}
					return httpDo(ctx, http.MethodPost, url, body)
				}),
			},
			"time": map[string]any{
				"nowMs": func() int64 { return time.Now().UnixMilli() },
				"sleep": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
					ms := call.Argument(0).ToInteger()
					timer := time.NewTimer(time.Duration(ms) * time.Millisecond)
					defer timer.Stop()
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-timer.C:
						return nil, nil
					}
				}),
			},
			"env": map[string]any{
				"get": func(call goja.FunctionCall) goja.Value {
					name := call.Argument(0).String()
					if v, ok := os.LookupEnv(name); ok {
						return vm.ToValue(v)
					}
					return goja.Undefined()
				},
			},
		}
	}); err != nil {
		return err
	}
	return nil
}

func httpDo(ctx context.Context, method, url, body string) (map[string]any, error) {
	httpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(httpCtx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status": resp.StatusCode,
		"body":   string(bs),
	}, nil
}

// valuesEqual compares two goja values using strict-equality semantics
// suitable for assert.equal: primitives compare by their export value, objects
// by JSON-like equality (good enough for the example bindings).
func valuesEqual(a, b goja.Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	if goja.IsUndefined(a) || goja.IsUndefined(b) {
		return goja.IsUndefined(a) && goja.IsUndefined(b)
	}
	if goja.IsNull(a) || goja.IsNull(b) {
		return goja.IsNull(a) && goja.IsNull(b)
	}
	return a.StrictEquals(b)
}
