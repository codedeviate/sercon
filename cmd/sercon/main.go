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

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// Exit codes mapped from script outcomes. Higher numbers win when multiple
// scripts run with different failure types — that way a single integer
// communicates the worst thing that happened.
const (
	exitOK       = 0 // every script passed
	exitUsage    = 1 // CLI argument / setup error (flag parsing, missing scripts, …)
	exitTranspile = 2 // at least one script failed to transpile (never ran)
	exitTimeout  = 3 // at least one script timed out or was context-cancelled
	exitThrow    = 4 // at least one script ran and threw an exception
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("sercon", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 10*time.Second, "Per-script timeout")
	root := fs.String("root", "", "Script root for require resolution (default: dirname of first script)")
	emitDTS := fs.String("emit-dts", "", "Write the .d.ts for the example bindings to this path and exit")
	verbose := fs.Bool("v", false, "Verbose: trace transpile output and module resolutions to stderr; also print duration on script failure")
	helpShort := fs.Bool("h", false, "Show in-depth, colorized help and exit")
	helpLong := fs.Bool("help", false, "Show in-depth, colorized help and exit")
	examples := fs.Bool("examples", false, "Show in-depth, colorized script examples of all features and exit")
	version := fs.Bool("version", false, "Print the engine version and exit")
	watch := fs.Bool("watch", false, "Re-run on every .ts / .tsx / .js / .jsx / .json / .d.ts change under the script root. Ctrl-C exits.")
	fs.Usage = func() { showHelp(os.Stderr) }
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	switch {
	case *helpShort || *helpLong:
		showHelp(os.Stdout)
		return exitOK
	case *version:
		showVersion(os.Stdout)
		return exitOK
	case *examples:
		showExamples(os.Stdout)
		return exitOK
	}

	scripts := fs.Args()
	if *emitDTS == "" && len(scripts) == 0 {
		fmt.Fprintln(os.Stderr, "sercon: no scripts given")
		fs.Usage()
		return exitUsage
	}

	scriptRoot := *root
	if scriptRoot == "" && len(scripts) > 0 {
		// Pick a script root from the first non-stdin script. Stdin scripts
		// share the cwd that sercon was launched in.
		seed := scripts[0]
		for _, s := range scripts {
			if s != "-" {
				seed = s
				break
			}
		}
		abs, err := filepath.Abs(seed)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sercon:", err)
			return exitUsage
		}
		scriptRoot = filepath.Dir(abs)
	}

	engOpts := scriptengine.Options{
		Timeout:    *timeout,
		ScriptRoot: scriptRoot,
	}
	if *verbose {
		engOpts.Verbose = os.Stderr
	}
	eng := scriptengine.New(engOpts)
	if err := registerExampleAPI(eng); err != nil {
		fmt.Fprintln(os.Stderr, "sercon:", err)
		return exitUsage
	}

	if *emitDTS != "" {
		f, err := os.Create(*emitDTS)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sercon:", err)
			return exitUsage
		}
		defer f.Close()
		if err := eng.WriteTypes(f); err != nil {
			fmt.Fprintln(os.Stderr, "sercon:", err)
			return exitUsage
		}
		if len(scripts) == 0 {
			return exitOK
		}
	}

	if *watch {
		// --watch is a long-running mode: do the initial run, then
		// block on fsnotify. It owns its own exit code semantics
		// (always 0 on clean shutdown via Ctrl-C; usage errors on
		// setup failure). Per-script throws inside a watch session
		// are logged but don't propagate as the process exit.
		return runWatchLoop(eng, scripts, scriptRoot, *verbose, os.Stdout)
	}

	worst := exitOK
	for _, s := range scripts {
		err := runOne(eng, s, *verbose)
		if err == nil {
			continue
		}
		code := classifyErr(err)
		if code > worst {
			worst = code
		}
		label := s
		if s == "-" {
			label = "<stdin>"
		}
		fmt.Printf("FAIL %s: %s\n", label, err)
	}
	return worst
}

// runOne executes a single script source, either a file path or "-" for
// stdin. On success it prints a PASS line and returns nil.
func runOne(eng *scriptengine.Engine, path string, verbose bool) error {
	start := time.Now()
	var err error
	label := path
	if path == "-" {
		label = "<stdin>"
		var data []byte
		data, err = io.ReadAll(os.Stdin)
		if err == nil {
			_, err = eng.Run(context.Background(), "<stdin>", string(data))
		}
	} else {
		_, err = eng.RunFile(context.Background(), path)
	}
	dur := time.Since(start)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "  duration: %s\n", dur)
		}
		return err
	}
	fmt.Printf("PASS %s (%s)\n", label, dur.Round(time.Millisecond))
	return nil
}

// classifyErr maps an Engine error to one of the documented exit codes.
// Transpile errors win their own bucket because they mean the script never
// even started; timeouts and cancellations share a bucket because both
// flavour "the script ran too long for us to wait"; everything else is a
// generic JS throw.
func classifyErr(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, scriptengine.ErrTranspile):
		return exitTranspile
	case errors.Is(err, scriptengine.ErrScriptTimeout),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return exitTimeout
	default:
		return exitThrow
	}
}

// registerExampleAPI wires the small example binding surface advertised by
// the README: api.log, api.assert.*, api.http.*, api.time.*, api.env.get,
// api.hash.*, api.str.*, api.path.*, api.net.*, api.email.*,
// api.compression.*, api.barcode.*, api.text.*, api.checkdigit.*,
// api.archive.*, api.diff.*, api.jq.*, api.exec.*, api.git.*, api.gh.*,
// api.preg.*, api.preg2.*, api.jwt.*, api.encrypt.*, api.sqlite.*,
// api.netstatus.*, api.browser.*, api.redis.*.
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
				"request": scriptengine.PromisifyAsync(vm, loop, httpRequestCall),
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
				"format": func(call goja.FunctionCall) goja.Value {
					ms := call.Argument(0).ToInteger()
					layout := call.Argument(1).String()
					loc := time.Local
					if v := call.Argument(2); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
						l, err := time.LoadLocation(v.String())
						if err != nil {
							panic(vm.NewGoError(fmt.Errorf("time.format: %w", err)))
						}
						loc = l
					}
					return vm.ToValue(strftime(time.UnixMilli(ms).In(loc), layout))
				},
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
			"hash":        hashNamespace(vm),
			"str":         strNamespace(vm),
			"path":        pathNamespace(vm),
			"net":         netNamespace(vm, loop),
			"email":       emailNamespace(vm, loop),
			"compression": compressionNamespace(vm, loop),
			"barcode":     barcodeNamespace(vm, loop),
			"text":        textNamespace(vm, loop),
			"checkdigit":  checkdigitNamespace(vm),
			"archive":     archiveNamespace(vm, loop),
			"diff":        diffNamespace(vm, loop),
			"jq":          jqNamespace(vm, loop),
			"exec":        execNamespace(vm, loop),
			"git":         gitNamespace(vm, loop),
			"gh":          ghNamespace(vm, loop),
			"preg":        pregNamespace(vm),
			"preg2":       preg2Namespace(vm),
			"jwt":         jwtNamespace(vm),
			"encrypt":     encryptNamespace(vm),
			"sqlite":      sqliteNamespace(vm, loop),
			"netstatus":   netstatusNamespace(vm, loop),
			"browser":     browserNamespace(vm, loop),
			"redis":       redisNamespace(vm, loop),
		}
	}); err != nil {
		return err
	}
	// Decorate the registered surface with JSDoc strings so the emitted
	// .d.ts grows useful editor hover. Docs are gathered in api_docs.go
	// (centralised so lockstep updates touch one file).
	e.SetDocs("api", "sercon's built-in script surface. The full reference lives in MANUAL.md; the JSDoc blocks here are the at-a-glance summary.")
	e.SetMemberDocs("api", apiDocs())
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

// strftime renders a Go time.Time using a tiny strftime token subset: the
// common date/time pieces (%Y/%y/%m/%d/%H/%M/%S, %F, %T, %j) plus weekday
// (%A/%a) and month (%B/%b) names in English, plus zone (%z/%Z) and the
// literal "%%". Unknown `%X` tokens are emitted verbatim so users get a
// visible signal rather than a silent rewrite.
func strftime(t time.Time, format string) string {
	var out strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			out.WriteString(t.Format("2006"))
		case 'y':
			out.WriteString(t.Format("06"))
		case 'm':
			out.WriteString(t.Format("01"))
		case 'd':
			out.WriteString(t.Format("02"))
		case 'H':
			out.WriteString(t.Format("15"))
		case 'M':
			out.WriteString(t.Format("04"))
		case 'S':
			out.WriteString(t.Format("05"))
		case 'T':
			out.WriteString(t.Format("15:04:05"))
		case 'F':
			out.WriteString(t.Format("2006-01-02"))
		case 'j':
			fmt.Fprintf(&out, "%03d", t.YearDay())
		case 'A':
			out.WriteString(t.Weekday().String())
		case 'a':
			s := t.Weekday().String()
			if len(s) > 3 {
				s = s[:3]
			}
			out.WriteString(s)
		case 'B':
			out.WriteString(t.Month().String())
		case 'b':
			s := t.Month().String()
			if len(s) > 3 {
				s = s[:3]
			}
			out.WriteString(s)
		case 'z':
			out.WriteString(t.Format("-0700"))
		case 'Z':
			out.WriteString(t.Format("MST"))
		case '%':
			out.WriteByte('%')
		default:
			out.WriteByte('%')
			out.WriteByte(format[i])
		}
	}
	return out.String()
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
