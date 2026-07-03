package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// serveSMTPLogger is the serve-mode hook for per-stage SMTP access logging.
// Populated by runServe (cmd/sercon/serve_cmd.go) before the script runs;
// vanilla `sercon` leaves it nil so no logging fires. Mirrors the
// serveAccessLogger pattern from cmd/sercon/server_http.go.
var serveSMTPLogger func(remote, stage, detail string, accepted bool, dur time.Duration)

// smtpAwaitBridge attaches .then() to a handler-returned Promise so the
// off-loop SMTP backend goroutine can await it. *goja.Program is safe to
// share across runtimes; compiled once at package init.
var smtpAwaitBridge = goja.MustCompile("internal:smtpAwait",
	`(p, onSettle, onReject) => p.then(onSettle, onReject)`, false)

// smtpServerMembers builds the {listen} map exposed as server.smtp.
func smtpServerMembers(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"listen": func(call goja.FunctionCall) goja.Value {
			return smtpListen(vm, loop, eng, call)
		},
	}
}

// smtpListen is the entry called as `server.smtp.listen({...})` from JS.
// Synchronously binds (so port-in-use throws immediately), then returns
// the server handle. Per-stage handler validation happens up front.
func smtpListen(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, call goja.FunctionCall) goja.Value {
	opts := call.Argument(0)
	if opts == nil || goja.IsUndefined(opts) {
		panic(vm.NewTypeError("server.smtp.listen: options object required"))
	}
	optsObj := opts.ToObject(vm)

	port := int(optsObj.Get("port").ToInteger())
	if port == 0 {
		panic(vm.NewTypeError("server.smtp.listen: `port` is required"))
	}
	if servePortOverride != 0 {
		port = servePortOverride
	}
	host := "0.0.0.0"
	if v := optsObj.Get("host"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		host = v.String()
	}
	hostname := ""
	if v := optsObj.Get("hostname"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		hostname = v.String()
	}
	if hostname == "" {
		if h, err := os.Hostname(); err == nil {
			hostname = h
		} else {
			hostname = "localhost"
		}
	}

	handlersVal := optsObj.Get("handlers")
	if handlersVal == nil || goja.IsUndefined(handlersVal) {
		panic(vm.NewTypeError("server.smtp.listen: `handlers` is required"))
	}
	handlersObj := handlersVal.ToObject(vm)
	mkFn := func(name string, required bool) *scriptengine.LoopCallable {
		v := handlersObj.Get(name)
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			if required {
				panic(vm.NewTypeError(fmt.Sprintf("server.smtp.listen: handlers.%s is required", name)))
			}
			return nil
		}
		fn, ok := goja.AssertFunction(v)
		if !ok {
			panic(vm.NewTypeError(fmt.Sprintf("server.smtp.listen: handlers.%s must be a function", name)))
		}
		return scriptengine.NewLoopCallable(loop, fn)
	}
	onMail := mkFn("onMail", true)
	onRcpt := mkFn("onRcpt", true)
	onData := mkFn("onData", true)

	var authFn *scriptengine.LoopCallable
	if v := optsObj.Get("auth"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		fn, ok := goja.AssertFunction(v)
		if !ok {
			panic(vm.NewTypeError("server.smtp.listen: `auth` must be a function"))
		}
		authFn = scriptengine.NewLoopCallable(loop, fn)
	}

	var tlsConfig *tls.Config
	if v := optsObj.Get("starttls"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		tlsObj := v.ToObject(vm)
		cert, err := loadCert(tlsObj.Get("cert").String(), tlsObj.Get("key").String())
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("server.smtp.listen: %w", err)))
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}

	allowInsecureAuth := false
	if v := optsObj.Get("allowInsecureAuth"); v != nil && !goja.IsUndefined(v) {
		allowInsecureAuth = v.ToBoolean()
	}

	maxBytes := int64(10 * 1024 * 1024)
	if v := optsObj.Get("maxMessageBytes"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		// Only honour a positive override. A zero/negative value would make
		// io.LimitReader(r, maxBytes+1) truncate every body to 1 byte and
		// reject it as "too large", so we keep the 10 MB default instead.
		if mb := v.ToInteger(); mb > 0 {
			maxBytes = mb
		}
	}
	maxRcpt := 100
	if v := optsObj.Get("maxRecipients"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		maxRcpt = int(v.ToInteger())
	}
	sessionTimeout := 30 * time.Second
	if v := optsObj.Get("sessionTimeout"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		sessionTimeout = time.Duration(v.ToInteger()) * time.Millisecond
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.smtp.listen %s: %w", addr, err)))
	}

	be := &smtpBackend{
		vm:       vm,
		loop:     loop,
		onMail:   onMail,
		onRcpt:   onRcpt,
		onData:   onData,
		authFn:   authFn,
		maxBytes: maxBytes,
	}

	server := gosmtp.NewServer(be)
	server.Addr = addr
	server.Domain = hostname
	server.MaxMessageBytes = maxBytes
	server.MaxRecipients = maxRcpt
	server.ReadTimeout = sessionTimeout
	server.WriteTimeout = sessionTimeout
	server.AllowInsecureAuth = allowInsecureAuth
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig
	}

	release := eng.HoldRun(fmt.Sprintf("server.smtp listen %s", addr))

	if serveReadyWriter != nil {
		fmt.Fprintf(serveReadyWriter, "READY listening on tcp/%s (smtp)\n", ln.Addr().String())
	}

	stoppedPromise, stoppedResolve, _ := vm.NewPromise()
	closed := atomic.Bool{}
	closeOnce := atomic.Bool{}

	// gracefulClose performs the same teardown the JS close() does; shared
	// so both the explicit close() and the shutdown hook drive it. Guarded
	// by closeOnce (mirrors doClose in server_tcp.go) so the body runs at
	// most once: GracefulShutdown snapshots the hook slice under a mutex and
	// releases it before running hooks, so an explicit close()'s removeHook()
	// can lose the race against a concurrent GracefulShutdown — both the JS
	// close and the shutdown hook then reach here at the same time. Without
	// the guard that's a real crash, not just wasted work: go-smtp's
	// Server.Close() does a non-atomic check-then-close(s.done), so two
	// concurrent calls can both pass the "not yet closed" check and both
	// call close(s.done), panicking with "close of closed channel" and
	// taking down the whole `sercon serve` process. ln.Close() and release()
	// are individually safe to call more than once, but are gated by the
	// same guard for symmetry and to keep this a true run-once teardown.
	gracefulClose := func() {
		if closeOnce.Swap(true) {
			return
		}
		// server.Close() closes s.done so Serve returns cleanly, but it
		// only closes listeners already registered in s.listeners. If it
		// races ahead of the `go server.Serve(ln)` registration step, Serve
		// would otherwise block in Accept forever. Closing ln directly
		// guarantees Accept unblocks regardless of ordering.
		_ = server.Close()
		_ = ln.Close()
		release()
	}

	// Register a graceful-shutdown hook so `sercon serve` can stop this
	// listener on SIGTERM/SIGINT. An explicit close() removes it first.
	removeHook := eng.AddShutdownHook(func(context.Context) error {
		gracefulClose()
		return nil
	})

	go func() {
		serveErr := server.Serve(ln)
		loop.RunOnLoop(func(vm *goja.Runtime) {
			if serveErr != nil && !errors.Is(serveErr, gosmtp.ErrServerClosed) {
				_ = stoppedResolve(vm.NewGoError(serveErr))
			} else {
				_ = stoppedResolve(goja.Undefined())
			}
			release()
		})
	}()

	handle := vm.NewObject()
	_ = handle.Set("address", vm.ToValue(fmt.Sprintf("tcp/%s", ln.Addr().String())))
	_ = handle.Set("stopped", stoppedPromise)
	_ = handle.Set("close", func(call goja.FunctionCall) goja.Value {
		if closed.Swap(true) {
			return vm.ToValue(stoppedPromise)
		}
		removeHook() // don't let GracefulShutdown close it a second time
		go gracefulClose()
		return vm.ToValue(stoppedPromise)
	})
	return handle
}

// smtpBackend implements gosmtp.Backend. One backend per listen() call;
// it spawns one smtpSession per accepted connection.
type smtpBackend struct {
	vm       *goja.Runtime
	loop     *eventloop.EventLoop
	onMail   *scriptengine.LoopCallable
	onRcpt   *scriptengine.LoopCallable
	onData   *scriptengine.LoopCallable
	authFn   *scriptengine.LoopCallable
	maxBytes int64
}

func (b *smtpBackend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	remote := ""
	if c.Conn() != nil {
		remote = c.Conn().RemoteAddr().String()
	}
	return &smtpSession{
		backend:  b,
		conn:     c,
		remote:   remote,
		envelope: smtpEnvelope{remote: remote, helo: c.Hostname()},
	}, nil
}

// smtpEnvelope is the Go-side mirror of the JS Envelope. Updated as the
// SMTP transaction proceeds; rebuilt as a fresh JS object each time a
// callback fires.
type smtpEnvelope struct {
	from              string
	recipients        []string
	remote            string
	helo              string
	authenticatedUser string
	tlsVersion        string
	tlsCipher         string
}

// smtpSession implements gosmtp.Session (and gosmtp.AuthSession when an
// auth callback is configured). Each method translates a protocol stage
// into a JS callback invocation via LoopCallable.
type smtpSession struct {
	backend  *smtpBackend
	conn     *gosmtp.Conn
	remote   string
	envelope smtpEnvelope
}

// loginServer implements sasl.Server for the LOGIN mechanism, which
// go-sasl doesn't ship a server for. LOGIN is a 2-step exchange: the
// server prompts "Username:", then "Password:", then runs the check.
// Non-standard but widely used by mail clients, so we support it
// alongside PLAIN.
type loginServer struct {
	check    func(username, password string) error
	username string
	state    int
}

func (l *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch l.state {
	case 0:
		l.state = 1
		return []byte("Username:"), false, nil
	case 1:
		l.username = string(response)
		l.state = 2
		return []byte("Password:"), false, nil
	case 2:
		if err := l.check(l.username, string(response)); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	default:
		return nil, false, errors.New("login: unexpected state")
	}
}

// AuthMechanisms reports the SASL mechanisms this session supports. We
// advertise PLAIN and LOGIN, and only when an auth callback was configured
// — otherwise the server omits AUTH from its EHLO capabilities entirely.
func (s *smtpSession) AuthMechanisms() []string {
	if s.backend.authFn == nil {
		return nil
	}
	return []string{sasl.Plain, sasl.Login}
}

// Auth returns a SASL server for the requested mechanism. Both PLAIN and
// LOGIN route credentials through a shared check that invokes the JS auth
// callback and awaits a possibly-async result.
func (s *smtpSession) Auth(mech string) (sasl.Server, error) {
	if s.backend.authFn == nil {
		return nil, gosmtp.ErrAuthUnsupported
	}
	s.captureTLS()
	// Shared credential check: invoke the JS auth callback, await a
	// possibly-async result, succeed only on a truthy resolved value.
	check := func(username, password string) error {
		start := time.Now()
		val, err := s.backend.authFn.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
			return []goja.Value{
				vm.ToValue(username),
				vm.ToValue(password),
				buildEnvelopeJS(vm, s.envelope),
			}, nil
		})
		val, err = awaitOnLoop(s.backend.loop, val, err)
		if err != nil {
			s.logStage("AUTH", fmt.Sprintf("user=%s", username), false, start)
			return errors.New("authentication failed")
		}
		if val != nil && val.ToBoolean() {
			s.envelope.authenticatedUser = username
			s.logStage("AUTH", fmt.Sprintf("user=%s", username), true, start)
			return nil
		}
		s.logStage("AUTH", fmt.Sprintf("user=%s", username), false, start)
		return errors.New("authentication failed")
	}
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return check(username, password)
		}), nil
	case sasl.Login:
		return &loginServer{check: check}, nil
	default:
		return nil, gosmtp.ErrAuthUnsupported
	}
}

func (s *smtpSession) Mail(from string, opts *gosmtp.MailOptions) error {
	start := time.Now()
	s.captureTLS()
	s.envelope.from = from
	val, err := s.backend.onMail.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		return []goja.Value{buildEnvelopeJS(vm, s.envelope)}, nil
	})
	val, err = awaitOnLoop(s.backend.loop, val, err)
	v := verdictFromValue(val, err)
	s.logStage("MAIL", fmt.Sprintf("from=<%s>", from), v.accept, start)
	if !v.accept {
		s.envelope.from = ""
		return v.err()
	}
	return nil
}

func (s *smtpSession) Rcpt(to string, opts *gosmtp.RcptOptions) error {
	start := time.Now()
	s.envelope.recipients = append(s.envelope.recipients, to)
	val, err := s.backend.onRcpt.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		return []goja.Value{
			buildEnvelopeJS(vm, s.envelope),
			vm.ToValue(to),
		}, nil
	})
	val, err = awaitOnLoop(s.backend.loop, val, err)
	v := verdictFromValue(val, err)
	s.logStage("RCPT", fmt.Sprintf("to=<%s>", to), v.accept, start)
	if !v.accept {
		s.envelope.recipients = s.envelope.recipients[:len(s.envelope.recipients)-1]
		return v.err()
	}
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	start := time.Now()
	limited := io.LimitReader(r, s.backend.maxBytes+1)
	var buf bytes.Buffer
	n, err := io.Copy(&buf, limited)
	if err != nil {
		s.logStage("DATA", fmt.Sprintf("error=%v", err), false, start)
		return err
	}
	if n > s.backend.maxBytes {
		s.logStage("DATA", fmt.Sprintf("bytes=%d too-big", n), false, start)
		return errors.New("message too large")
	}
	raw := buf.Bytes()
	parsed, perr := enmime.ReadEnvelope(bytes.NewReader(raw))
	if perr != nil {
		s.logStage("DATA", fmt.Sprintf("parse-error=%v", perr), false, start)
		return fmt.Errorf("parse message: %w", perr)
	}
	val, err := s.backend.onData.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		return []goja.Value{
			buildEnvelopeJS(vm, s.envelope),
			buildMessageJS(vm, parsed, raw),
		}, nil
	})
	val, err = awaitOnLoop(s.backend.loop, val, err)
	v := verdictFromValue(val, err)
	s.logStage("DATA", fmt.Sprintf("bytes=%d recipients=%d", n, len(s.envelope.recipients)), v.accept, start)
	if !v.accept {
		return v.err()
	}
	return nil
}

func (s *smtpSession) Reset() {
	s.envelope.from = ""
	s.envelope.recipients = nil
}

func (s *smtpSession) Logout() error {
	s.logStage("QUIT", "", true, time.Now())
	return nil
}

// captureTLS records the negotiated TLS version/cipher on the envelope if
// the connection has completed a (STARTTLS) handshake. Safe to call
// repeatedly; later stages observe the same state.
func (s *smtpSession) captureTLS() {
	if s.conn == nil || s.envelope.tlsVersion != "" {
		return
	}
	if state, ok := s.conn.TLSConnectionState(); ok {
		s.envelope.tlsVersion = tlsVersionName(state.Version)
		s.envelope.tlsCipher = tls.CipherSuiteName(state.CipherSuite)
	}
}

// tlsVersionName maps a tls.Version constant to a human-readable string.
func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}

func (s *smtpSession) logStage(stage, detail string, accepted bool, start time.Time) {
	if serveSMTPLogger == nil {
		return
	}
	serveSMTPLogger(s.remote, stage, detail, accepted, time.Since(start))
}

// smtpVerdict is the result of running a JS handler.
type smtpVerdict struct {
	accept bool
	reason string
	temp   bool
}

func (v smtpVerdict) err() error {
	code := 550
	if v.temp {
		code = 451
	}
	reason := v.reason
	if reason == "" {
		if v.temp {
			reason = "Temporary failure"
		} else {
			reason = "Command refused"
		}
	}
	return &gosmtp.SMTPError{Code: code, EnhancedCode: gosmtp.NoEnhancedCode, Message: reason}
}

// awaitOnLoop resolves a possibly-Promise handler return value. If val is
// not a Promise it passes through unchanged. If it IS a Promise, this
// schedules a .then on the loop and blocks the calling (off-loop) goroutine
// until the Promise settles, returning the resolved value or the rejection
// as an error.
//
// Safe to call from a go-smtp backend method (which runs on the per-
// connection goroutine, off the event loop). The loop must be alive
// (HoldRun keeps it so) for the microtask to run. Do NOT call from inside
// a loop callback — that would re-enter RunOnLoop and deadlock.
//
// callErr is the error from the preceding LoopCallable.Call; if non-nil it
// short-circuits (the handler threw synchronously).
func awaitOnLoop(loop *eventloop.EventLoop, val goja.Value, callErr error) (goja.Value, error) {
	if callErr != nil {
		return nil, callErr
	}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return val, nil
	}
	if _, ok := val.Export().(*goja.Promise); !ok {
		return val, nil // not a Promise — pass through
	}
	type settled struct {
		v   goja.Value
		err error
	}
	done := make(chan settled, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		bridgeVal, err := vm.RunProgram(smtpAwaitBridge)
		if err != nil {
			done <- settled{nil, fmt.Errorf("await bridge: %w", err)}
			return
		}
		bridge, ok := goja.AssertFunction(bridgeVal)
		if !ok {
			done <- settled{nil, errors.New("await bridge not callable")}
			return
		}
		onSettle := func(call goja.FunctionCall) goja.Value {
			done <- settled{call.Argument(0), nil}
			return goja.Undefined()
		}
		onReject := func(call goja.FunctionCall) goja.Value {
			done <- settled{nil, fmt.Errorf("%s", call.Argument(0).String())}
			return goja.Undefined()
		}
		if _, err := bridge(goja.Undefined(), val, vm.ToValue(onSettle), vm.ToValue(onReject)); err != nil {
			done <- settled{nil, fmt.Errorf("await .then: %w", err)}
		}
	})
	r := <-done
	return r.v, r.err
}

// verdictFromValue maps a JS handler's return value (or thrown error) into
// an SMTP-level accept/reject verdict.
//
//	true / undefined  → accept
//	false             → 550 Command refused
//	string            → 550 <string>
//	thrown error      → 451 Temporary failure (with err message as reason)
func verdictFromValue(val goja.Value, callErr error) smtpVerdict {
	if callErr != nil {
		return smtpVerdict{accept: false, reason: callErr.Error(), temp: true}
	}
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return smtpVerdict{accept: true}
	}
	exported := val.Export()
	switch x := exported.(type) {
	case bool:
		return smtpVerdict{accept: x}
	case string:
		return smtpVerdict{accept: false, reason: x}
	}
	return smtpVerdict{accept: true}
}

// buildEnvelopeJS constructs the JS Envelope object on the loop. Must be
// called from within a LoopCallable buildArgs closure (where vm is valid).
//
// Key order is stable (from, recipients, remote, helo) with the optional
// authenticatedUser / tls keys appended only when present, so JSON.stringify
// is deterministic for canonical-hash callers.
func buildEnvelopeJS(vm *goja.Runtime, e smtpEnvelope) goja.Value {
	o := scriptengine.NewOrdered().
		Set("from", e.from).
		Set("recipients", e.recipients).
		Set("remote", e.remote).
		Set("helo", e.helo)
	if e.authenticatedUser != "" {
		o.Set("authenticatedUser", e.authenticatedUser)
	}
	if e.tlsVersion != "" {
		o.Set("tls", scriptengine.NewOrdered().
			Set("version", e.tlsVersion).
			Set("cipher", e.tlsCipher))
	}
	return scriptengine.OrderedToValue(vm, o)
}

// buildMessageJS constructs the JS Message object on the loop, populated
// from the enmime-parsed envelope plus the raw DATA bytes.
//
// Key order is stable: from, to, cc, subject, headers, body, attachments,
// raw. Header names (dynamic) are emitted in sorted order so the nested
// `headers` object is deterministic — enmime exposes them via a map, which
// has no preserved wire order, so a sort is the only stable choice.
func buildMessageJS(vm *goja.Runtime, parsed *enmime.Envelope, raw []byte) goja.Value {
	// enmime exposes headers via parsed.Root.Header (a textproto.MIMEHeader,
	// i.e. a map). Lowercase the keys and emit in sorted order for a stable
	// Record<string, string[]>.
	headers := scriptengine.NewOrdered()
	if parsed.Root != nil {
		keys := make([]string, 0, len(parsed.Root.Header))
		for k := range parsed.Root.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			headers.Set(strings.ToLower(k), parsed.Root.Header[k])
		}
	}

	body := scriptengine.NewOrdered().
		Set("text", parsed.Text).
		Set("html", parsed.HTML)

	atts := make([]*scriptengine.Ordered, 0, len(parsed.Attachments))
	for _, a := range parsed.Attachments {
		atts = append(atts, scriptengine.NewOrdered().
			Set("filename", a.FileName).
			Set("contentType", a.ContentType).
			Set("bytes", a.Content))
	}

	o := scriptengine.NewOrdered().
		Set("from", parsed.GetHeader("From")).
		Set("to", splitAddresses(parsed.GetHeader("To"))).
		Set("cc", splitAddresses(parsed.GetHeader("Cc"))).
		Set("subject", parsed.GetHeader("Subject")).
		Set("headers", headers).
		Set("body", body).
		Set("attachments", atts).
		Set("raw", raw)
	return scriptengine.OrderedToValue(vm, o)
}

// splitAddresses splits an address-list header value on commas, trimming
// whitespace. RFC 5322 quoted-comma handling is best-effort; scripts that
// need strict parsing can re-parse via headers + their own mail package.
func splitAddresses(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
