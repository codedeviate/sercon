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
	"reflect"
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
	exitOK        = 0 // every script passed
	exitUsage     = 1 // CLI argument / setup error (flag parsing, missing scripts, …)
	exitTranspile = 2 // at least one script failed to transpile (never ran)
	exitTimeout   = 3 // at least one script timed out or was context-cancelled
	exitThrow     = 4 // at least one script ran and threw an exception
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "serve":
			os.Exit(runServe(os.Args[2:]))
		case "run":
			os.Exit(runRun(os.Args[2:]))
		case "init":
			os.Exit(runInit(os.Args[2:]))
		}
	}
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// Split the raw args at the first standalone "--": everything after it
	// is the user argument vector handed to scripts as runtime.argv[2:]. We
	// do this before flag parsing because Go's flag package stops at the
	// first positional token and would surface "--" as a bogus script path.
	var userArgs []string
	for i, a := range args {
		if a == "--" {
			userArgs = args[i+1:]
			args = args[:i]
			break
		}
	}

	fs := flag.NewFlagSet("sercon", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 10*time.Second, "Per-script timeout")
	root := fs.String("root", "", "Script root for require resolution (default: dirname of first script)")
	emitDTS := fs.String("emit-dts", "", "Write the .d.ts for the example bindings to this path and exit")
	emitReference := fs.String("emit-reference", "", "Write the markdown binding reference to this path and exit")
	verbose := fs.Bool("v", false, "Verbose: trace transpile output and module resolutions to stderr; also print duration on script failure")
	helpShort := fs.Bool("h", false, "Show in-depth, colorized help and exit")
	helpLong := fs.Bool("help", false, "Show in-depth, colorized help and exit")
	examples := fs.Bool("examples", false, "Show in-depth, colorized script examples of all features and exit")
	version := fs.Bool("version", false, "Print the engine version and exit")
	watch := fs.Bool("watch", false, "Re-run on every .ts / .tsx / .js / .jsx / .json / .d.ts change under the script root. Ctrl-C exits.")
	noPager := fs.Bool("no-pager", false, "Don't page --help / --examples through $PAGER even on a terminal.")
	fs.Usage = func() { showHelp(os.Stderr) }
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	switch {
	case *helpShort || *helpLong:
		pageOutput(*noPager, showHelp)
		return exitOK
	case *version:
		showVersion(os.Stdout)
		return exitOK
	case *examples:
		pageOutput(*noPager, showExamples)
		return exitOK
	}

	scripts := fs.Args()
	if *emitDTS == "" && *emitReference == "" && len(scripts) == 0 {
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
		Timeout:        *timeout,
		ScriptRoot:     scriptRoot,
		ProgramName:    "sercon",
		WatchMode:      *watch,
		DisableConsole: true, // CLI provides its own clean `console` (console.go)
	}
	if *verbose {
		engOpts.Verbose = os.Stderr
	}
	eng := scriptengine.New(engOpts)
	if err := registerSurface(eng); err != nil {
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
		if len(scripts) == 0 && *emitReference == "" {
			return exitOK
		}
	}

	if *emitReference != "" {
		f, err := os.Create(*emitReference)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sercon:", err)
			return exitUsage
		}
		defer f.Close()
		if err := eng.WriteReference(f); err != nil {
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
		return runWatchLoop(eng, scripts, scriptRoot, *verbose, os.Stdout, userArgs)
	}

	worst := exitOK
	for _, s := range scripts {
		err := runOne(eng, s, *verbose, userArgs)
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
func runOne(eng *scriptengine.Engine, path string, verbose bool, userArgs []string) error {
	start := time.Now()
	var err error
	label := path
	if path == "-" {
		label = "<stdin>"
		var data []byte
		data, err = io.ReadAll(os.Stdin)
		if err == nil {
			_, err = eng.Run(context.Background(), "<stdin>", string(data), scriptengine.WithArgs(userArgs))
		}
	} else {
		_, err = eng.RunFile(context.Background(), path, scriptengine.WithArgs(userArgs))
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

// registerSurface wires sercon's ten top-level script-facing globals:
// runtime, crypto, text, codec, fs, net, db, server, services, tui.
// Each is registered via RegisterNamespaceFactory so per-Run
// constructions that need the loop (Promise-returning bindings, TUI
// controller, server listeners) get fresh state every run. JSDoc lives
// in docs.go; the engine patches runtime.argv onto the runtime
// namespace at Run time.
func registerSurface(e *scriptengine.Engine) error {
	// `console` is a browser/Node-compat shim (see console.go). Registered
	// alongside the ten reserved globals; the CLI disables the engine's
	// built-in console (Options.DisableConsole) so this one is authoritative.
	if err := e.RegisterNamespaceFactory("console", func(vm *goja.Runtime, _ *eventloop.EventLoop) map[string]any {
		return consoleNamespace(vm)
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("runtime", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"log": func(call goja.FunctionCall) goja.Value {
				parts := make([]string, 0, len(call.Arguments))
				for _, a := range call.Arguments {
					parts = append(parts, formatValue(vm, a))
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
			// argv is a placeholder: the engine patches the real per-Run argv
			// onto this object after registrations are applied. Registering
			// here ensures the d.ts emitter surfaces `argv: string[]` with
			// JSDoc — runtime behaviour is identical to v0.8.x.
			"argv": []string{},
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("crypto", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"hash":    hashNamespace(vm),
			"jwt":     jwtNamespace(vm),
			"encrypt": encryptNamespace(vm),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("text", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"str":     strNamespace(vm),
			"preg":    pregNamespace(vm),
			"preg2":   preg2Namespace(vm),
			"charset": charsetNamespace(vm, loop),
			"jq":      jqNamespace(vm, loop),
			"diff":    diffNamespace(vm, loop),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("codec", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"compression": compressionNamespace(vm, loop),
			"barcode":     barcodeNamespace(vm, loop),
			"checkdigit":  checkdigitNamespace(vm),
			"php":         phpNamespace(vm),
			"perl":        perlNamespace(vm),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("fs", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"path":    pathNamespace(vm),
			"archive": archiveNamespace(vm, loop),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
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
			"probe":     probeNamespace(vm, loop),
			"netstatus": netstatusNamespace(vm, loop),
			"email":     emailNamespace(vm, loop),
			"browser":   browserNamespace(vm, loop),
			"tcp":       tcpNamespace(vm, loop, e),
			"udp":       udpNamespace(vm, loop, e),
			"icmp":      icmpNamespace(vm, loop, e),
			"capture":   captureNamespace(vm, loop, e),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("db", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"sqlite":     sqliteNamespace(vm, loop),
			"postgres":   postgresNamespace(vm, loop),
			"mysql":      mysqlNamespace(vm, loop),
			"mssql":      mssqlNamespace(vm, loop),
			"clickhouse": clickhouseNamespace(vm, loop),
			"oracle":     oracleNamespace(vm, loop),
			"redis":      redisNamespace(vm, loop),
			"memcached":  memcachedNamespace(vm, loop),
			"ldap":       ldapNamespace(vm, loop),
			"dict":       dictNamespace(vm, loop),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("services", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"exec": map[string]any{
				"shell": scriptengine.PromisifyAsync(vm, loop, execShell),
				"http":  scriptengine.PromisifyAsync(vm, loop, execHTTP),
			},
			"git": gitNamespace(vm, loop),
			"gh":  ghNamespace(vm, loop),
			"ai":  aiNamespace(vm, loop),
		}
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("tui", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return tuiNamespace(vm, loop, e)
	}); err != nil {
		return err
	}
	if err := e.RegisterNamespaceFactory("server", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return serverNamespace(vm, loop, e)
	}); err != nil {
		return err
	}
	// Decorate the registered surface with JSDoc strings so the emitted
	// .d.ts grows useful editor hover. Docs are gathered in docs.go
	// (centralised so lockstep updates touch one file).
	e.SetDocs("runtime", "Script-host scaffolding: logging, assertions, time, environment, runtime.argv.")
	e.SetMemberDocsStructured("runtime", runtimeDocs())
	e.SetDocs("crypto", "Hashing, JWT, age encryption — anything that produces a digest, signature, or ciphertext.")
	e.SetMemberDocsStructured("crypto", cryptoDocs())
	e.SetDocs("text", "String / regex / charset / data manipulation — all text-shaped transforms.")
	e.SetMemberDocsStructured("text", textDocs())
	e.SetDocs("codec", "Binary-format codecs: compression, barcodes, check digits.")
	e.SetMemberDocsStructured("codec", codecDocs())
	e.SetDocs("fs", "Filesystem operations: path manipulation and archive create/extract.")
	e.SetMemberDocsStructured("fs", fsDocs())
	e.SetDocs("net", "Network clients and probes: HTTP, TCP/DNS/TLS/NTP/WHOIS probes, netstatus, email auth, browser-style sessions.")
	e.SetMemberDocsStructured("net", netDocs())
	e.SetDocs("db", "Database / KV / directory clients: SQLite, PostgreSQL, MySQL/MariaDB, SQL Server, Redis, memcached, LDAP, dict.")
	e.SetMemberDocsStructured("db", dbDocs())
	e.SetDocs("services", "Subprocess and external-CLI / service wrappers: shell, git, gh, AI providers.")
	e.SetMemberDocsStructured("services", servicesDocs())
	e.SetDocs("tui", "Multi-pane terminal UI: layout, pane, write, focus.")
	e.SetMemberDocsStructured("tui", tuiDocs())
	e.SetDocs("server", "Network servers: HTTP/HTTPS listeners with routing, middleware, static files, WebSocket upgrade.")
	e.SetMemberDocsStructured("server", serverDocs())
	e.SetDocs("console", "Browser/Node-style console shim: log/info/debug to stdout, warn/error to stderr. For porting scripts; runtime.log is the native equivalent.")
	e.SetMemberDocsStructured("console", consoleDocs())
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
	if a.StrictEquals(b) {
		return true
	}
	// Deep structural equality for objects/arrays — StrictEquals is reference
	// equality, so two distinct objects with identical contents need this.
	// Export() turns goja objects into Go map[string]any / []any, which
	// reflect.DeepEqual compares recursively (map key order is irrelevant).
	return reflect.DeepEqual(a.Export(), b.Export())
}
