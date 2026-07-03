package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
	"github.com/tebeka/selenium/firefox"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// wdDrivers maps a browser to the driver binary we start on the local path.
var wdDrivers = map[string]string{"chrome": "chromedriver", "firefox": "geckodriver"}

// byStrategy maps a JS locator strategy to a selenium By* constant.
func byStrategy(by string) (string, error) {
	switch by {
	case "css":
		return selenium.ByCSSSelector, nil
	case "xpath":
		return selenium.ByXPATH, nil
	case "id":
		return selenium.ByID, nil
	case "name":
		return selenium.ByName, nil
	case "tag":
		return selenium.ByTagName, nil
	case "className":
		return selenium.ByClassName, nil
	case "linkText":
		return selenium.ByLinkText, nil
	case "partialLinkText":
		return selenium.ByPartialLinkText, nil
	default:
		return "", fmt.Errorf("webdriver: unknown locator strategy %q (use css/xpath/id/name/tag/className/linkText/partialLinkText)", by)
	}
}

// buildCaps assembles selenium.Capabilities from connect opts.
func buildCaps(opts map[string]any) selenium.Capabilities {
	browser, _ := opts["browser"].(string)
	if browser == "" {
		browser = "chrome"
	}
	caps := selenium.Capabilities{"browserName": browser}

	headless := true
	if h, ok := opts["headless"].(bool); ok {
		headless = h
	}
	var extra []string
	if arr, ok := opts["args"].([]any); ok {
		for _, a := range arr {
			extra = append(extra, fmt.Sprintf("%v", a))
		}
	}

	switch browser {
	case "firefox":
		var args []string
		if headless {
			args = append(args, "-headless")
		}
		args = append(args, extra...)
		caps.AddFirefox(firefox.Capabilities{Args: args})
	default: // chrome
		var args []string
		if headless {
			args = append(args, "--headless=new")
		}
		args = append(args, extra...)
		caps.AddChrome(chrome.Capabilities{Args: args})
	}

	// Raw capability overrides (escape hatch), merged last.
	if raw, ok := opts["capabilities"].(map[string]any); ok {
		for k, v := range raw {
			caps[k] = v
		}
	}
	return caps
}

// wdAvailable reports whether a known driver binary is on PATH.
func wdAvailable() bool {
	for _, bin := range wdDrivers {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// wdFreePort asks the OS for a free TCP port.
func wdFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("webdriver: unexpected addr type %T", l.Addr())
	}
	return addr.Port, nil
}

// wdRegistry tracks sessions opened this Run for best-effort quit on Run end.
type wdRegistry struct {
	mu       sync.Mutex
	sessions map[*wdSession]struct{}
}

// wdSession is one WebDriver session. mu serialises commands (one WebDriver
// client is not safe for concurrent HTTP commands). svc is non-nil when this
// session started a local driver process that must be stopped on quit.
type wdSession struct {
	wd      selenium.WebDriver
	svc     *selenium.Service
	reg     *wdRegistry
	baseURL string  // <scheme>://host:port[/wd/hub] — for raw s.command requests
	browser string  // "chrome" | "firefox" — resolved in connect; gates CDP methods

	cdpMu   sync.Mutex // guards cdpConn lazy init
	cdpConn *cdpConn   // browser-level CDP connection (lazy; closed on shutdown)
	mu      sync.Mutex
	closed  atomic.Bool

	// ctx is canceled by shutdown(), aborting any in-flight raw s.command
	// HTTP request (e.g. one blocked behind an open alert or an unreachable
	// driver). cmdTimeout bounds each raw command. Both are set in connect;
	// command() guards against the zero values for directly-constructed
	// sessions (tests).
	ctx        context.Context
	cancel     context.CancelFunc
	cmdTimeout time.Duration
}

// do runs fn under the per-session mutex, rejecting a closed session. All
// WebDriver/WebElement calls for this session funnel through here so commands
// never interleave on one client.
func (s *wdSession) do(fn func() (any, error)) (any, error) {
	if s.closed.Load() {
		return nil, errors.New("webdriver: session already closed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return nil, errors.New("webdriver: session already closed")
	}
	return fn()
}

// wdAsync wraps an async session/element method as a bare goja callback. The
// methods live on a runtime-built handle object so they need .Func, like
// agentBrowser.
func wdAsync(vm *goja.Runtime, loop *eventloop.EventLoop, work func(context.Context, goja.FunctionCall) (any, error)) func(goja.FunctionCall) goja.Value {
	return scriptengine.PromisifyAsync(vm, loop, work).Func
}

func (r *wdRegistry) track(s *wdSession) {
	r.mu.Lock()
	r.sessions[s] = struct{}{}
	r.mu.Unlock()
}

func (r *wdRegistry) forget(s *wdSession) {
	r.mu.Lock()
	delete(r.sessions, s)
	r.mu.Unlock()
}

// closeAll quits every still-open session (best-effort) on Run end.
func (r *wdRegistry) closeAll() {
	r.mu.Lock()
	all := make([]*wdSession, 0, len(r.sessions))
	for s := range r.sessions {
		all = append(all, s)
	}
	r.sessions = map[*wdSession]struct{}{}
	r.mu.Unlock()
	for _, s := range all {
		s.shutdown()
	}
}

// wdCloseTimeout bounds session teardown (wd.Quit / svc.Stop) so a wedged
// driver process can't hang the Run — mirrors agentBrowser's abCloseTimeout.
// Neither selenium.WebDriver.Quit nor selenium.Service.Stop accepts a
// context, so runBounded races each call against a timer instead of
// threading a deadline through. Var (not const) so tests can shrink it
// without waiting out the real 10s.
var wdCloseTimeout = 10 * time.Second

// wdRunBounded runs fn on its own goroutine and returns once fn completes or
// timeout elapses, whichever comes first. On timeout the goroutine is left
// to finish on its own (best effort, matching the fire-and-forget teardown
// semantics elsewhere in this file) — the point is only that the caller
// (shutdown/closeAll, ultimately the Run) is never blocked past timeout.
// Distinct from secrets.go's runBounded[T]: that one returns (T, error) with
// a fixed timeout; this one is void-returning with a caller-supplied
// duration (wdCloseTimeout, shrinkable in tests).
func wdRunBounded(timeout time.Duration, fn func()) {
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// shutdown quits the WebDriver and stops a started Service. Idempotent.
// Both calls are bounded by wdCloseTimeout via runBounded so a wedged
// driver process can't hang the Run (or an explicit quit() call) forever.
func (s *wdSession) shutdown() {
	if s.closed.Swap(true) {
		return
	}
	// Abort any in-flight raw command before tearing down the driver, so a
	// command blocked on an open alert / dead endpoint unblocks promptly.
	if s.cancel != nil {
		s.cancel()
	}
	s.closeCDP()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wd != nil {
		wdRunBounded(wdCloseTimeout, func() { _ = s.wd.Quit() })
	}
	if s.svc != nil {
		wdRunBounded(wdCloseTimeout, func() { _ = s.svc.Stop() })
	}
}

// connect returns the connect work func: builds caps, dials a remote url or
// starts an installed local driver, registers the session, returns its handle.
func (r *wdRegistry) connect(vm *goja.Runtime, loop *eventloop.EventLoop) func(context.Context, goja.FunctionCall) (any, error) {
	return func(_ context.Context, call goja.FunctionCall) (any, error) {
		opts := optsArgMap(call, 0)
		caps := buildCaps(opts)
		browser, _ := opts["browser"].(string)
		if browser == "" {
			browser = "chrome"
		}

		var svc *selenium.Service
		url, _ := opts["url"].(string)
		if url == "" {
			driver := wdDrivers[browser]
			path, err := exec.LookPath(driver)
			if err != nil {
				return nil, fmt.Errorf("webdriver.connect: no url given and %s not on PATH (install it or pass opts.url)", driver)
			}
			port, err := wdFreePort()
			if err != nil {
				return nil, fmt.Errorf("webdriver.connect: %w", err)
			}
			// The dial URL must match the url-base each tebeka service
			// configures: NewChromeDriverService starts chromedriver with
			// `--url-base=wd/hub`, so it only answers under /wd/hub; geckodriver
			// is started with no url-base and serves at root. Dialing the wrong
			// prefix makes chromedriver 404 the POST /session as text/plain,
			// surfacing as `got content type "text/plain", expected "application/json"`.
			if browser == "firefox" {
				svc, err = selenium.NewGeckoDriverService(path, port)
				url = fmt.Sprintf("http://127.0.0.1:%d", port)
			} else {
				svc, err = selenium.NewChromeDriverService(path, port)
				url = fmt.Sprintf("http://127.0.0.1:%d/wd/hub", port)
			}
			if err != nil {
				return nil, fmt.Errorf("webdriver.connect: starting %s: %w", driver, err)
			}
		}

		wd, err := selenium.NewRemote(caps, url)
		if err != nil {
			if svc != nil {
				_ = svc.Stop()
			}
			return nil, fmt.Errorf("webdriver.connect: %w", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		s := &wdSession{
			wd: wd, svc: svc, reg: r,
			baseURL:    strings.TrimRight(url, "/"),
			browser:    browser,
			ctx:        ctx,
			cancel:     cancel,
			cmdTimeout: wdCommandTimeout(opts),
		}
		r.track(s)
		return s.jsObject(vm, loop), nil
	}
}

// wdDefaultCommandTimeout bounds each raw s.command HTTP request unless the
// caller overrides it via connect's opts.commandTimeout (milliseconds).
const wdDefaultCommandTimeout = 30 * time.Second

// wdCommandTimeout reads opts.commandTimeout (ms) for the per-command HTTP
// timeout, falling back to wdDefaultCommandTimeout. A non-positive value
// disables the timeout (relies on ctx cancellation / driver alone).
func wdCommandTimeout(opts map[string]any) time.Duration {
	v, ok := opts["commandTimeout"]
	if !ok {
		return wdDefaultCommandTimeout
	}
	var ms float64
	switch t := v.(type) {
	case float64:
		ms = t
	case int64:
		ms = float64(t)
	case int:
		ms = float64(t)
	default:
		return wdDefaultCommandTimeout
	}
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// quit closes the session explicitly (de-registers + shutdown).
func (s *wdSession) quit(_ context.Context, _ goja.FunctionCall) (any, error) {
	s.reg.forget(s)
	s.shutdown()
	o := scriptengine.NewOrdered()
	o.Set("closed", true)
	return o, nil
}

// jsObject builds the goja-facing session handle. Method groups are added
// across tasks. Safe to call with vm=nil/loop=nil (only the quit method is
// wired; stub groups are no-ops that don't deref vm/loop at construction).
func (s *wdSession) jsObject(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	obj := map[string]any{
		"quit": wdAsync(vm, loop, s.quit),
	}
	s.addNav(obj, vm, loop)     // Task 3
	s.addFind(obj, vm, loop)    // Task 4
	s.addPage(obj, vm, loop)    // Task 5
	s.addScript(obj, vm, loop)  // Task 5
	s.addCookies(obj, vm, loop) // Task 5
	s.addWaits(obj, vm, loop)   // Task 5
	s.addWindows(obj, vm, loop) // Phase 2
	s.addFrames(obj, vm, loop)  // Phase 2
	s.addAlerts(obj, vm, loop)      // Phase 2
	s.addWindowRect(obj, vm, loop) // Phase 2
	s.addActions(obj, vm, loop)    // Phase 2
	s.addCDP(obj, vm, loop)        // 0004: Chrome-only CDP trusted click
	return obj
}

// webdriverNamespace builds services.webdriver.*. Registers a Run-end cleanup
// that quits any session left open.
func webdriverNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine) map[string]any {
	reg := &wdRegistry{sessions: map[*wdSession]struct{}{}}
	if vm != nil {
		e.AddRunCleanup(reg.closeAll)
	}
	return map[string]any{
		"available": wdAvailable(),
		"probe": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (*scriptengine.Ordered, error) {
			u, _ := optsArgMap(call, 0)["url"].(string)
			if u == "" {
				return nil, errors.New("webdriver.probe: opts.url is required")
			}
			resp, err := http.Get(strings.TrimRight(u, "/") + "/status")
			o := scriptengine.NewOrdered()
			if err != nil {
				o.Set("ready", false)
				o.Set("error", err.Error())
				return o, nil
			}
			defer resp.Body.Close()
			_, _ = io.Copy(io.Discard, resp.Body)
			o.Set("ready", resp.StatusCode == http.StatusOK)
			o.Set("status", resp.StatusCode)
			return o, nil
		}),
		"connect": scriptengine.PromisifyAsync(vm, loop, reg.connect(vm, loop)),
	}
}

