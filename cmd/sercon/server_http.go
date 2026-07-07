package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// Serve-mode hooks: populated by runServe before the script runs;
// vanilla `sercon` leaves them nil. The HTTP listener consults them
// for port override (replaces the script's port), the access logger
// (called per request), and the readiness writer (one line per listener).
var (
	servePortOverride int
	serveAccessLogger func(remote, method, path string, status int, dur time.Duration)
	serveReadyWriter  io.Writer
)

// bridgeProg is the compiled JS bridge used by bridgeHandlerResult to
// chain .then() onto a handler-returned Promise. *goja.Program is safe
// to share across runtimes (per goja docs), so we compile once at
// package init and reuse for every request. Avoids per-request
// goja.Compile cost on the dispatch hot-path.
var bridgeProg = goja.MustCompile("internal:bridgeHandlerResult",
	`(p, onSettle, onReject) => p.then(onSettle, onReject)`, false)

// httpServerMembers builds the {listen} map exposed as server.http or
// server.https. isTLS picks the listener path; otherwise the two are
// identical (same handler dispatch, same route compilation).
func httpServerMembers(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, isTLS bool) map[string]any {
	return map[string]any{
		"listen": func(call goja.FunctionCall) goja.Value {
			return httpListen(vm, loop, eng, isTLS, call)
		},
		"static": httpStaticBinding(vm, loop),
	}
}

// httpListen is the entry called as `server.http.listen({...})` from JS.
// Synchronously starts the listener (so bind errors throw immediately),
// then returns a server handle object with close() and a stopped Promise.
func httpListen(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, isTLS bool, call goja.FunctionCall) goja.Value {
	opts := call.Argument(0)
	if opts == nil || goja.IsUndefined(opts) {
		panic(vm.NewTypeError("server.http.listen: options object required"))
	}
	optsObj := opts.ToObject(vm)

	port := int(optsObj.Get("port").ToInteger())
	if port == 0 {
		panic(vm.NewTypeError("server.http.listen: `port` is required"))
	}
	if servePortOverride != 0 {
		port = servePortOverride
	}
	host := "0.0.0.0"
	if v := optsObj.Get("host"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		host = v.String()
	}

	// Routes
	routesVal := optsObj.Get("routes")
	if routesVal == nil || goja.IsUndefined(routesVal) {
		panic(vm.NewTypeError("server.http.listen: `routes` is required"))
	}
	routesObj := routesVal.ToObject(vm)

	// Global middleware
	var globalMW []*scriptengine.LoopCallable
	if v := optsObj.Get("use"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		arr := v.ToObject(vm)
		n := int(arr.Get("length").ToInteger())
		for i := 0; i < n; i++ {
			fn, ok := goja.AssertFunction(arr.Get(fmt.Sprintf("%d", i)))
			if !ok {
				panic(vm.NewTypeError(fmt.Sprintf("server.http.listen: use[%d] is not a function", i)))
			}
			globalMW = append(globalMW, scriptengine.NewLoopCallable(loop, fn))
		}
	}

	// Optional per-listener request-body cap. Absent or <=0 falls back to
	// DefaultMaxServerBodyBytes. Enforced in dispatchHandler via
	// http.MaxBytesReader before any JS handler/middleware runs.
	maxBodyBytes := DefaultMaxServerBodyBytes
	if v := optsObj.Get("maxBodyBytes"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		if n := int(v.ToInteger()); n > 0 {
			maxBodyBytes = n
		}
	}

	// Optional onError handler: (err, req, res) => … invoked when a handler
	// or middleware throws/rejects, in place of the stock 500.
	var onError *scriptengine.LoopCallable
	if v := optsObj.Get("onError"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		fn, ok := goja.AssertFunction(v)
		if !ok {
			panic(vm.NewTypeError("server.http.listen: `onError` must be a function"))
		}
		onError = scriptengine.NewLoopCallable(loop, fn)
	}

	// Compile routes into a ServeMux
	mux := http.NewServeMux()
	// Tracks live SSE streams so shutdown/close can tear them down (they
	// never go idle, so http.Server.Shutdown would otherwise block on them).
	sseReg := newSSERegistry()
	for _, key := range routesObj.Keys() {
		pattern := key
		routeVal := routesObj.Get(key)
		handler, perRouteMW, err := compileRoute(vm, loop, routeVal)
		if err != nil {
			if sre, ok := err.(*staticRouteError); ok {
				// Register the static handler directly on the mux.
				registerMux(vm, pattern, func() { mux.Handle(pattern, sre.handler) })
				continue
			}
			panic(vm.NewTypeError(fmt.Sprintf("server.http.listen: route %q: %v", pattern, err)))
		}
		chain := append([]*scriptengine.LoopCallable{}, globalMW...)
		chain = append(chain, perRouteMW...)
		registerMux(vm, pattern, func() {
			mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
				dispatchHandler(loop, eng, sseReg, chain, handler, onError, w, r, maxBodyBytes)
			})
		})
	}

	// TLS bits
	var tlsConfig *tls.Config
	if isTLS {
		certVal := optsObj.Get("cert")
		if certVal == nil || goja.IsUndefined(certVal) || goja.IsNull(certVal) {
			panic(vm.NewTypeError(`server.https.listen: ` + "`cert`" + ` is required (a file path, inline PEM, or "self-signed")`))
		}
		var cert tls.Certificate
		var err error
		if strings.EqualFold(strings.TrimSpace(certVal.String()), "self-signed") {
			// Ephemeral dev cert: generated in-process, never written to disk.
			cert, err = generateSelfSignedCert(selfSignedHosts(host))
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("server.https.listen: self-signed cert: %w", err)))
			}
		} else {
			keyVal := optsObj.Get("key")
			if keyVal == nil || goja.IsUndefined(keyVal) || goja.IsNull(keyVal) {
				panic(vm.NewTypeError(`server.https.listen: ` + "`key`" + ` is required unless cert is "self-signed"`))
			}
			cert, err = loadCert(certVal.String(), keyVal.String())
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("server.https.listen: %w", err)))
			}
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	srv := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// Bind synchronously so the script learns about port-in-use errors immediately.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.listen %s: %w", addr, err)))
	}
	if isTLS {
		ln = tls.NewListener(ln, tlsConfig)
	}
	if serveReadyWriter != nil {
		fmt.Fprintf(serveReadyWriter, "READY listening on tcp/%s\n", ln.Addr().String())
	}

	// Hold the loop alive until close.
	release := eng.HoldRun(fmt.Sprintf("server.http listen %s", addr))

	stoppedPromise, stoppedResolve, _ := vm.NewPromise()
	var serveErr atomic.Value // error
	closed := atomic.Bool{}

	// Register a graceful-shutdown hook so `sercon serve` can close this
	// listener on SIGTERM/SIGINT without the script's cooperation. The hook
	// runs from a non-loop goroutine (the serve signal handler): both
	// http.Server.Shutdown and release() (ClearTimeout, enqueued as an
	// aux-job on the loop) are safe to call off-loop. An explicit
	// srv.close() removes this hook first so the listener isn't torn down
	// twice.
	removeHook := eng.AddShutdownHook(func(ctx context.Context) error {
		sseReg.teardownAll() // unblock never-idle SSE streams first
		err := srv.Shutdown(ctx)
		release()
		return err
	})

	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr.Store(err)
		}
		// Schedule the Promise resolution on the loop.
		loop.RunOnLoop(func(vm *goja.Runtime) {
			if v := serveErr.Load(); v != nil {
				e, _ := v.(error)
				_ = stoppedResolve(vm.NewGoError(e))
			} else {
				_ = stoppedResolve(goja.Undefined())
			}
			release()
		})
	}()

	// Server handle object.
	handle := vm.NewObject()
	_ = handle.Set("address", vm.ToValue(fmt.Sprintf("tcp/%s", ln.Addr().String())))
	_ = handle.Set("stopped", stoppedPromise)
	_ = handle.Set("close", func(call goja.FunctionCall) goja.Value {
		if closed.Swap(true) {
			return vm.ToValue(stoppedPromise) // already closing
		}
		removeHook()         // don't let GracefulShutdown close it a second time
		sseReg.teardownAll() // unblock never-idle SSE streams first
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
			// stoppedResolve fires from the Serve goroutine above.
		}()
		return vm.ToValue(stoppedPromise)
	})

	return handle
}

// errValue recovers the JS value an onError handler should receive for a Go
// error returned by a handler/middleware call. When the error is a goja
// exception (the usual case — a JS `throw`), it returns the *original* thrown
// value, so `err.message` matches what the script threw (rather than goja's
// "Error: msg\n\tat …" stringification). Otherwise it builds a fresh JS Error.
func errValue(vm *goja.Runtime, err error) goja.Value {
	if ex, ok := err.(*goja.Exception); ok {
		return ex.Value()
	}
	return newJSError(vm, err.Error())
}

// newJSError builds a JS Error object carrying msg, so an onError handler
// receives `err.message` like a normal thrown Error. Falls back to the bare
// string if the Error constructor is somehow unavailable.
func newJSError(vm *goja.Runtime, msg string) goja.Value {
	if ctor, ok := goja.AssertFunction(vm.Get("Error")); ok {
		if v, err := ctor(goja.Undefined(), vm.ToValue(msg)); err == nil {
			return v
		}
	}
	return vm.ToValue(msg)
}

// loadCert reads cert/key from disk OR from inline PEM strings (detected
// by the presence of "-----BEGIN").
func loadCert(certSrc, keySrc string) (tls.Certificate, error) {
	if strings.HasPrefix(certSrc, "-----BEGIN") {
		return tls.X509KeyPair([]byte(certSrc), []byte(keySrc))
	}
	return tls.LoadX509KeyPair(certSrc, keySrc)
}

// staticRouteError is a sentinel returned from compileRoute when a route
// value is a *staticMarker. The httpListen route loop catches it and
// registers the raw stdlib handler on the mux instead of going through
// the LoopCallable dispatch. Not a real error — just a convenient way
// to thread the carried handler back to the listener.
type staticRouteError struct{ handler http.Handler }

func (e *staticRouteError) Error() string { return "static-route marker" }

// registerMux runs a single ServeMux registration, converting the panic
// net/http raises for an invalid or conflicting route pattern (a bare Go
// error, e.g. `"health"` with no leading slash or two overlapping
// `{wildcard}` patterns) into a catchable goja TypeError. Route keys come
// straight from the script, so without this a bad pattern re-panics out of
// the event loop and crashes the whole process instead of throwing like
// every other bad-option path in httpListen.
func registerMux(vm *goja.Runtime, pattern string, register func()) {
	defer func() {
		if r := recover(); r != nil {
			panic(vm.NewTypeError(fmt.Sprintf("server.http.listen: route %q: %v", pattern, r)))
		}
	}()
	register()
}

// compileRoute turns a route value (bare function OR {use, handler} object)
// into a LoopCallable + per-route middleware slice.
func compileRoute(vm *goja.Runtime, loop *eventloop.EventLoop, val goja.Value) (*scriptengine.LoopCallable, []*scriptengine.LoopCallable, error) {
	// Static-handler marker: bypass LoopCallable; register raw stdlib handler.
	if marker, ok := val.Export().(*staticMarker); ok {
		return nil, nil, &staticRouteError{handler: marker.handler}
	}
	// Bare function form: (req, res) => …
	if fn, ok := goja.AssertFunction(val); ok {
		return scriptengine.NewLoopCallable(loop, fn), nil, nil
	}
	// Object form: { use: [...], handler: fn }
	obj := val.ToObject(vm)
	handlerVal := obj.Get("handler")
	handlerFn, ok := goja.AssertFunction(handlerVal)
	if !ok {
		return nil, nil, errors.New("route value must be a function or {use, handler} object")
	}
	var mw []*scriptengine.LoopCallable
	if useVal := obj.Get("use"); useVal != nil && !goja.IsUndefined(useVal) && !goja.IsNull(useVal) {
		arr := useVal.ToObject(vm)
		n := int(arr.Get("length").ToInteger())
		for i := 0; i < n; i++ {
			fn, ok := goja.AssertFunction(arr.Get(fmt.Sprintf("%d", i)))
			if !ok {
				return nil, nil, fmt.Errorf("route use[%d] is not a function", i)
			}
			mw = append(mw, scriptengine.NewLoopCallable(loop, fn))
		}
	}
	return scriptengine.NewLoopCallable(loop, handlerFn), mw, nil
}

// responseState holds the in-flight HTTP response. Mutated by JS-side
// res.* terminal methods; read by the goroutine after notify closes.
type responseState struct {
	mu        sync.Mutex
	status    int
	headers   http.Header
	cookies   []*http.Cookie
	body      []byte
	contentTy string // content type to set if not already in headers
	finalized bool
	// terminal is set only when a response body/redirect was explicitly
	// produced by a terminal (json/text/html/bytes/empty/redirect). It
	// distinguishes that from the dispatch path finalizing a handler that
	// returned WITHOUT responding — the latter must emit 204, not 200.
	terminal bool
	errored  bool
	jsError  string // captured handler-thrown error
	notify   chan struct{}

	// upgrade is set by upgradeWebSocket; signals the writer goroutine
	// to NOT write a regular response (the websocket library owns the
	// connection after Hijack). Wired by Task 4.
	upgrade bool

	// stream is set by res.sse(): the response is a long-lived
	// text/event-stream written incrementally by a pump goroutine. Unlike
	// upgrade (WebSocket hijack), the connection is NOT hijacked, so
	// dispatchHandler must stay parked on streamDone until the stream
	// closes, or net/http closes the connection. writeResponse is skipped
	// (upgrade is also set so its short-circuit fires).
	stream     bool
	streamDone chan struct{}

	// failWith routes an unhandled handler/middleware error. When an
	// onError handler is configured for the listener it is invoked as
	// (err, req, res) so the script can render a custom response;
	// otherwise — or if onError itself throws or settles without
	// finalizing — the stock 500 is emitted via markError. Set
	// per-dispatch in dispatchHandler after req/res are built (so it can
	// close over them); nil on the websocket/internal paths. Invoked only
	// from loop callbacks.
	failWith func(errVal goja.Value, fallbackMsg string)
}

func newResponseState() *responseState {
	return &responseState{
		status:  200,
		headers: http.Header{},
		notify:  make(chan struct{}),
	}
}

// markFinal flips finalized and closes notify. Idempotent — second call
// is a no-op so JS terminal-twice produces a clean TypeError elsewhere.
func (rs *responseState) markFinal() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.finalized {
		return
	}
	rs.finalized = true
	close(rs.notify)
}

// markTerminal records that a terminal explicitly produced the response
// (so writeResponse must NOT downgrade a 200 to 204) and finalizes it.
func (rs *responseState) markTerminal() {
	rs.mu.Lock()
	rs.terminal = true
	rs.mu.Unlock()
	rs.markFinal()
}

// isFinalized returns whether the response has been finalized. Takes the
// lock so callers don't race against markFinal/markError. Cheaper than
// holding the lock for a whole compound check.
func (rs *responseState) isFinalized() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.finalized
}

// markError records a handler-thrown error and finalizes.
func (rs *responseState) markError(msg string) {
	rs.mu.Lock()
	if !rs.finalized {
		rs.errored = true
		rs.jsError = msg
		rs.finalized = true
		close(rs.notify)
	}
	rs.mu.Unlock()
}

// dispatchHandler is invoked from net/http's per-request goroutine. It
// schedules a loop callback that builds req/res, invokes the middleware
// chain + handler, and waits on res.notify for finalization (either via
// a terminal call or via the handler-Promise's settlement).
func dispatchHandler(loop *eventloop.EventLoop, eng *scriptengine.Engine, sseReg *sseRegistry, chain []*scriptengine.LoopCallable, handler *scriptengine.LoopCallable, onError *scriptengine.LoopCallable, w http.ResponseWriter, r *http.Request, maxBodyBytes int) {
	startTime := time.Now()
	// Read body up front; small price for the simpler script API. Cap it
	// with MaxBytesReader so a large POST can't OOM the process before any
	// JS handler/middleware runs — an over-limit body gets a 413 here,
	// before the loop is ever scheduled.
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBodyBytes))
	bodyBytes, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = fmt.Fprintln(w, "Request Entity Too Large")
			if serveAccessLogger != nil {
				serveAccessLogger(r.RemoteAddr, r.Method, r.URL.Path, http.StatusRequestEntityTooLarge, time.Since(startTime))
			}
			return
		}
		// Non-size read errors (e.g. client hung up mid-body): fall through
		// with whatever partial bytes were read, matching the previous
		// best-effort behaviour.
	}

	state := newResponseState()

	// Compose middleware chain into a single LoopCallable-equivalent.
	// At dispatch time we invoke from a single Schedule call: the loop
	// callback builds req/res, then walks the chain by recursively
	// invoking each middleware with a `next` function.
	_, err = loopSchedule(loop, func(vm *goja.Runtime) (goja.Value, error) {
		req := buildRequestObject(vm, r, bodyBytes)
		res := buildResponseObject(vm, loop, eng, sseReg, state, w, r)

		// Wire the error router now that req/res exist. With no onError
		// configured it is exactly markError (stock 500); otherwise it
		// invokes the JS handler and falls back to markError if that
		// handler throws or settles without finalizing the response.
		state.failWith = func(errVal goja.Value, fallbackMsg string) {
			if onError == nil {
				state.markError(fallbackMsg)
				return
			}
			result, oerr := onError.CallOnLoop(vm, errVal, req, res)
			if oerr != nil {
				state.markError(fallbackMsg)
				return
			}
			// Async onError: bridge its Promise; emit the stock 500 if it
			// settles (or rejects) without producing a response.
			if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
				if _, ok := result.Export().(*goja.Promise); ok {
					thenVal, terr := vm.RunProgram(bridgeProg)
					if terr != nil {
						state.markError(fallbackMsg)
						return
					}
					thenFn, ok := goja.AssertFunction(thenVal)
					if !ok {
						state.markError(fallbackMsg)
						return
					}
					onSettle := func(call goja.FunctionCall) goja.Value {
						if !state.isFinalized() {
							state.markError(fallbackMsg)
						}
						return goja.Undefined()
					}
					onReject := func(call goja.FunctionCall) goja.Value {
						state.markError(fallbackMsg)
						return goja.Undefined()
					}
					_, _ = thenFn(goja.Undefined(), result, vm.ToValue(onSettle), vm.ToValue(onReject))
					return
				}
			}
			if !state.isFinalized() {
				state.markError(fallbackMsg)
			}
		}

		// Middleware runner. Each level returns a Promise that settles when
		// that level's chain (and everything beneath it) has finished its
		// work. `next` returns this Promise so `await next()` in JS waits
		// for the inner layers to complete — necessary for the documented
		// "post-process" middleware semantic.
		//
		// State finalisation still flows through bridgeHandlerResult →
		// markFinal/markError, which closes state.notify for the dispatcher
		// goroutine. The level Promises are independent: they let JS see
		// "the inner work is done"; the state machinery lets the Go side
		// see "the response is ready to write".
		//
		// We are already inside a loop callback (loopSchedule), so use
		// CallOnLoop on the LoopCallables — calling Call() here would
		// re-enqueue onto the (single-threaded) loop and deadlock.
		var runner func(idx int) goja.Value
		runner = func(idx int) goja.Value {
			if idx >= len(chain) {
				// Final layer: invoke the handler.
				result, err := handler.CallOnLoop(vm, req, res)
				if err != nil {
					state.failWith(errValue(vm, err), err.Error())
					return rejectedPromise(vm, err)
				}
				return propagateResult(vm, result, state)
			}
			next := func(call goja.FunctionCall) goja.Value {
				return runner(idx + 1)
			}
			result, err := chain[idx].CallOnLoop(vm, req, res, vm.ToValue(next))
			if err != nil {
				state.failWith(errValue(vm, err), err.Error())
				return rejectedPromise(vm, err)
			}
			return propagateResult(vm, result, state)
		}
		runner(0)
		return goja.Undefined(), nil
	})
	if err != nil {
		state.markError(err.Error())
	}

	<-state.notify
	state.mu.Lock()
	stream := state.stream
	streamDone := state.streamDone
	state.mu.Unlock()
	if stream {
		// SSE: keep this http handler goroutine alive so net/http does not
		// finish the response. The pump goroutine in server_sse.go owns w
		// and closes streamDone on stream close / client disconnect.
		<-streamDone
	} else {
		writeResponse(w, state)
	}
	if serveAccessLogger != nil {
		serveAccessLogger(r.RemoteAddr, r.Method, r.URL.Path, state.status, time.Since(startTime))
	}
}

// bridgeHandlerResult chains a JS .then(onSettle, onReject) onto a
// handler/middleware-returned value if it's a Promise. onSettle calls
// state.markFinal (no-op if a terminal already fired); onReject calls
// state.markError. If the value is not a Promise (sync return), and the
// state is not yet finalized, mark final (→ 204).
func bridgeHandlerResult(vm *goja.Runtime, result goja.Value, state *responseState) {
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		if !state.isFinalized() {
			state.markFinal()
		}
		return
	}
	exported := result.Export()
	if promise, ok := exported.(*goja.Promise); ok {
		// Goja's Promise carries Result() + State() but we cannot
		// synchronously await it inside this loop callback. Attach a
		// JS-side .then via the package-level bridgeProg to bridge.
		thenVal, err := vm.RunProgram(bridgeProg)
		if err != nil {
			state.markError("internal: bridge run: " + err.Error())
			return
		}
		thenFn, ok := goja.AssertFunction(thenVal)
		if !ok {
			state.markError("internal: bridge not callable")
			return
		}
		onSettle := func(call goja.FunctionCall) goja.Value {
			if !state.isFinalized() {
				state.markFinal()
			}
			return goja.Undefined()
		}
		onReject := func(call goja.FunctionCall) goja.Value {
			errVal := call.Argument(0)
			if state.failWith != nil {
				state.failWith(errVal, errVal.String())
			} else {
				state.markError(errVal.String())
			}
			return goja.Undefined()
		}
		_, _ = thenFn(goja.Undefined(), vm.ToValue(promise), vm.ToValue(onSettle), vm.ToValue(onReject))
		_ = exported
		return
	}
	// Sync return; if not yet finalized, mark final (→ 204).
	if !state.isFinalized() {
		state.markFinal()
	}
}

// propagateResult is the middleware-aware version of bridgeHandlerResult.
// In addition to signalling state.notify (so the dispatcher writes the
// response), it returns a Promise that JS callers can `await` to know
// when this layer's work has completed. For sync returns it wraps in a
// resolved Promise; for Promise returns it returns the original Promise
// (which bridgeHandlerResult has already chained .then onto for state
// finalisation).
func propagateResult(vm *goja.Runtime, result goja.Value, state *responseState) goja.Value {
	if result != nil && !goja.IsUndefined(result) && !goja.IsNull(result) {
		if _, ok := result.Export().(*goja.Promise); ok {
			bridgeHandlerResult(vm, result, state)
			return result
		}
	}
	if !state.isFinalized() {
		state.markFinal()
	}
	promise, resolve, _ := vm.NewPromise()
	_ = resolve(goja.Undefined())
	return vm.ToValue(promise)
}

// rejectedPromise returns a JS Promise that is immediately rejected with
// the given Go error. Used by the middleware runner when a level's
// CallOnLoop returned a synchronous throw.
func rejectedPromise(vm *goja.Runtime, err error) goja.Value {
	promise, _, reject := vm.NewPromise()
	_ = reject(vm.NewGoError(err))
	return vm.ToValue(promise)
}

// loopSchedule runs fn on the loop and waits for it to complete. Mirrors
// the LoopCallable.Call pattern but without a captured callable — caller
// supplies the work directly.
func loopSchedule(loop *eventloop.EventLoop, fn func(vm *goja.Runtime) (goja.Value, error)) (goja.Value, error) {
	type result struct {
		val goja.Value
		err error
	}
	done := make(chan result, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				done <- result{nil, fmt.Errorf("loopSchedule panic: %v", r)}
			}
		}()
		v, err := fn(vm)
		done <- result{v, err}
	})
	r := <-done
	return r.val, r.err
}

// writeResponse copies state into the http.ResponseWriter and writes the body.
func writeResponse(w http.ResponseWriter, state *responseState) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.upgrade {
		// WebSocket already took over the connection; nothing to write here.
		return
	}
	if state.errored {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintln(w, "Internal Server Error")
		return
	}
	// Apply queued cookies as Set-Cookie headers.
	for _, c := range state.cookies {
		http.SetCookie(w, c)
	}
	for k, vs := range state.headers {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if state.contentTy != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", state.contentTy)
	}
	status := state.status
	// A handler that returned without invoking a terminal produces the
	// documented 204 No Content, not the default 200. An explicit terminal
	// (even res.empty) sets `terminal`, so its status stands.
	if !state.terminal && status == 200 {
		status = http.StatusNoContent
	}
	w.WriteHeader(status)
	if len(state.body) > 0 {
		_, _ = w.Write(state.body)
	}
}

// buildRequestObject constructs the JS `req` from an http.Request. Field
// names match the spec's Request type verbatim.
func buildRequestObject(vm *goja.Runtime, r *http.Request, bodyBytes []byte) goja.Value {
	req := vm.NewObject()
	_ = req.Set("method", r.Method)
	_ = req.Set("url", r.URL.String())
	_ = req.Set("path", r.URL.Path)
	_ = req.Set("remote", r.RemoteAddr)
	// Query: Record<string, string[]>
	query := vm.NewObject()
	for k, vs := range r.URL.Query() {
		_ = query.Set(k, vs)
	}
	_ = req.Set("query", query)
	// Headers: lowercase keys, Record<string, string[]>
	headers := vm.NewObject()
	for k, vs := range r.Header {
		_ = headers.Set(strings.ToLower(k), vs)
	}
	_ = req.Set("headers", headers)
	// Params: stdlib http.ServeMux exposes path values via r.PathValue("name")
	// but doesn't enumerate the pattern's named segments. For v0.10.0,
	// populate params as an empty object; future enhancement may capture
	// pattern names at compile time.
	params := vm.NewObject()
	_ = req.Set("params", params)
	// Body
	_ = req.Set("body", string(bodyBytes))
	_ = req.Set("bodyBytes", vm.ToValue(bodyBytes))
	// Cookies
	cookies := vm.NewObject()
	for _, c := range r.Cookies() {
		_ = cookies.Set(c.Name, c.Value)
	}
	_ = req.Set("cookies", cookies)
	return req
}

// buildResponseObject constructs the JS `res` builder. Methods mutate
// the responseState; terminals call markFinal. The loop/eng/w/r tuple
// is threaded purely to feed res.upgradeWebSocket (Task 4); the rest
// of the builder doesn't need them.
func buildResponseObject(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, sseReg *sseRegistry, state *responseState, w http.ResponseWriter, r *http.Request) *goja.Object {
	res := vm.NewObject()
	// Re-add `res` to its own methods so chaining returns the same object.
	self := vm.ToValue(res)

	// Each res.* method acquires state.mu exactly once: check finalized
	// and mutate inside the same locked region (no TOCTOU window between
	// the check and the write). Terminals unlock BEFORE calling
	// state.markFinal — markFinal re-acquires the mutex itself, so calling
	// it while still locked would deadlock.

	_ = res.Set("status", func(call goja.FunctionCall) goja.Value {
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.status: response already finalized"))
		}
		state.status = int(call.Argument(0).ToInteger())
		state.mu.Unlock()
		return self
	})
	_ = res.Set("header", func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0).String()
		v := call.Argument(1).String()
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.header: response already finalized"))
		}
		state.headers.Add(k, v)
		state.mu.Unlock()
		return self
	})
	_ = res.Set("cookie", func(call goja.FunctionCall) goja.Value {
		c := buildCookie(vm, call)
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.cookie: response already finalized"))
		}
		state.cookies = append(state.cookies, c)
		state.mu.Unlock()
		return self
	})

	// Terminals.
	_ = res.Set("json", func(call goja.FunctionCall) goja.Value {
		raw, err := json.Marshal(call.Argument(0).Export())
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("res.json: %w", err)))
		}
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.json: response already finalized"))
		}
		state.body = raw
		state.contentTy = "application/json"
		state.mu.Unlock()
		state.markTerminal()
		return self
	})
	_ = res.Set("text", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.text: response already finalized"))
		}
		state.body = []byte(s)
		state.contentTy = "text/plain; charset=utf-8"
		state.mu.Unlock()
		state.markTerminal()
		return self
	})
	_ = res.Set("html", func(call goja.FunctionCall) goja.Value {
		s := call.Argument(0).String()
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.html: response already finalized"))
		}
		state.body = []byte(s)
		state.contentTy = "text/html; charset=utf-8"
		state.mu.Unlock()
		state.markTerminal()
		return self
	})
	_ = res.Set("bytes", func(call goja.FunctionCall) goja.Value {
		exported := call.Argument(0).Export()
		bs, ok := exported.([]byte)
		if !ok {
			panic(vm.NewTypeError("res.bytes: expected Uint8Array"))
		}
		ct := "application/octet-stream"
		if len(call.Arguments) > 1 {
			ct = call.Argument(1).String()
		}
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.bytes: response already finalized"))
		}
		// Copy: goja's Uint8Array export aliases the live ArrayBuffer, and
		// writeResponse reads state.body on the request goroutine, so a later
		// script mutation would otherwise race the write / corrupt the body.
		state.body = append([]byte(nil), bs...)
		state.contentTy = ct
		state.mu.Unlock()
		state.markTerminal()
		return self
	})
	_ = res.Set("empty", func(call goja.FunctionCall) goja.Value {
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.empty: response already finalized"))
		}
		state.body = nil
		state.mu.Unlock()
		state.markTerminal()
		return self
	})
	// upgradeWebSocket — hijack the connection and return a JS object
	// that's both an AsyncIterable<WSMessage> and has .send / .close.
	// See server_ws.go for the implementation.
	_ = res.Set("upgradeWebSocket", func(call goja.FunctionCall) goja.Value {
		var opts goja.Value
		if len(call.Arguments) > 0 {
			opts = call.Argument(0)
		}
		return upgradeWebSocketImpl(vm, loop, eng, state, w, r, opts)
	})

	// sse — start a Server-Sent Events stream. Writes text/event-stream
	// headers, parks the dispatcher on streamDone, and returns a handle
	// with send / close / closed. See server_sse.go.
	_ = res.Set("sse", func(call goja.FunctionCall) goja.Value {
		var opts goja.Value
		if len(call.Arguments) > 0 {
			opts = call.Argument(0)
		}
		return sseImpl(vm, loop, eng, sseReg, state, w, r, opts)
	})

	_ = res.Set("redirect", func(call goja.FunctionCall) goja.Value {
		loc := call.Argument(0).String()
		code := http.StatusFound
		if len(call.Arguments) > 1 {
			code = int(call.Argument(1).ToInteger())
		}
		state.mu.Lock()
		if state.finalized {
			state.mu.Unlock()
			panic(vm.NewTypeError("res.redirect: response already finalized"))
		}
		state.status = code
		state.headers.Set("Location", loc)
		state.mu.Unlock()
		state.markTerminal()
		return self
	})
	return res
}

// buildCookie turns a JS call(name, value, opts?) into an http.Cookie.
func buildCookie(vm *goja.Runtime, call goja.FunctionCall) *http.Cookie {
	c := &http.Cookie{
		Name:  call.Argument(0).String(),
		Value: call.Argument(1).String(),
	}
	if len(call.Arguments) >= 3 {
		opts := call.Argument(2).ToObject(vm)
		if v := opts.Get("domain"); v != nil && !goja.IsUndefined(v) {
			c.Domain = v.String()
		}
		if v := opts.Get("path"); v != nil && !goja.IsUndefined(v) {
			c.Path = v.String()
		}
		if v := opts.Get("maxAge"); v != nil && !goja.IsUndefined(v) {
			c.MaxAge = int(v.ToInteger())
		}
		if v := opts.Get("expires"); v != nil && !goja.IsUndefined(v) {
			c.Expires = time.UnixMilli(v.ToInteger())
		}
		if v := opts.Get("secure"); v != nil && !goja.IsUndefined(v) {
			c.Secure = v.ToBoolean()
		}
		if v := opts.Get("httpOnly"); v != nil && !goja.IsUndefined(v) {
			c.HttpOnly = v.ToBoolean()
		}
		if v := opts.Get("sameSite"); v != nil && !goja.IsUndefined(v) {
			switch strings.ToLower(v.String()) {
			case "strict":
				c.SameSite = http.SameSiteStrictMode
			case "lax":
				c.SameSite = http.SameSiteLaxMode
			case "none":
				c.SameSite = http.SameSiteNoneMode
			}
		}
	}
	return c
}
