package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

const loadMaxConcurrency = 1000

// classifyTarget reports whether host (an IP literal or "localhost") is a
// PUBLIC address. Loopback / private / link-local / ULA / unspecified /
// "localhost" are non-public. A non-IP, non-localhost hostname returns true
// (fail-safe — callers resolve hostnames to IPs before classifying).
func classifyTarget(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	return true
}

// targetIsPublic resolves host (IP or hostname) and reports whether it should
// be treated as public. A hostname resolving only to private IPs is private;
// resolution failure is fail-safe public.
func targetIsPublic(ctx context.Context, host string) bool {
	if net.ParseIP(host) != nil || strings.EqualFold(host, "localhost") {
		return classifyTarget(host)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, a := range ips {
		if classifyTarget(a.IP.String()) {
			return true
		}
	}
	return false
}

// redirectGuard returns an http.Client.CheckRedirect that re-applies the
// public-target guardrail to every redirect hop, not just the initial URL.
// The HTTP client follows redirects transparently, so without this a
// data-named target (a redirect Location, which is attacker/server
// controllable) could bounce a "confirmed non-public" load test onto a
// public host, bypassing the dual-use guard entirely. The chain is also
// capped at 10 hops as a sanity backstop.
func redirectGuard(ctx context.Context, confirm bool) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("net.load.http: stopped after 10 redirects")
		}
		if targetIsPublic(ctx, req.URL.Hostname()) && !confirm {
			return fmt.Errorf("net.load.http: refusing to follow redirect to public host %q without confirm:true", req.URL.Hostname())
		}
		return nil
	}
}

// percentiles returns nearest-rank percentile values (ps in 0..100) over xs.
// xs is sorted in place. Empty xs → zeros.
func percentiles(xs []float64, ps ...float64) []float64 {
	out := make([]float64, len(ps))
	if len(xs) == 0 {
		return out
	}
	sort.Float64s(xs)
	for i, p := range ps {
		switch {
		case p <= 0:
			out[i] = xs[0]
		case p >= 100:
			out[i] = xs[len(xs)-1]
		default:
			rank := int(math.Ceil(p/100*float64(len(xs)))) - 1
			if rank < 0 {
				rank = 0
			}
			if rank >= len(xs) {
				rank = len(xs) - 1
			}
			out[i] = xs[rank]
		}
	}
	return out
}

// reqOutcome is one request's result. status 0 means a transport failure.
type reqOutcome struct {
	latencyMs float64
	status    int
	errKind   string
}

// errKindOf buckets a transport error for the report.
func errKindOf(err error) string {
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return "refused"
	case strings.Contains(msg, "no such host"):
		return "dns"
	case strings.Contains(msg, "connection reset"):
		return "reset"
	default:
		return "error"
	}
}

// loadHTTPOp backs net.load.http(opts). Runs off-loop (PromisifyAsync), so it
// reads opts from a plain map snapshot, not live goja values.
func loadHTTPOp(ctx context.Context, call goja.FunctionCall) (any, error) {
	opts, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		return nil, errors.New("net.load.http: options object required")
	}
	target := optString(opts, "url", "")
	if target == "" {
		return nil, errors.New("net.load.http: `url` is required")
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("net.load.http: invalid url %q (need http/https)", target)
	}
	method := strings.ToUpper(optString(opts, "method", "GET"))
	headers := optStringMap(opts, "headers")
	body := optString(opts, "body", "")
	concurrency := optInt(opts, "concurrency", 10)
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > loadMaxConcurrency {
		return nil, fmt.Errorf("net.load.http: concurrency %d exceeds the %d cap", concurrency, loadMaxConcurrency)
	}
	requests := optInt(opts, "requests", 0)
	duration := optMillis(opts, "duration", 0)
	if (requests <= 0) == (duration <= 0) {
		return nil, errors.New("net.load.http: provide exactly one of `requests` (>0) or `duration` (>0 ms)")
	}
	rps := optInt(opts, "rps", 0)
	timeout := optMillis(opts, "timeout", 10*time.Second)
	confirm := optBool(opts, "confirm", false)

	// Dual-use guardrail: refuse public targets without explicit confirm.
	if targetIsPublic(ctx, u.Hostname()) && !confirm {
		return nil, fmt.Errorf("net.load.http: refusing to load-test public host %q without confirm:true (authorized self-testing only)", u.Hostname())
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency * 2,
			MaxConnsPerHost:     concurrency * 2,
		},
		CheckRedirect: redirectGuard(ctx, confirm),
	}

	// Stop condition.
	runCtx := ctx
	var cancel context.CancelFunc
	if duration > 0 {
		runCtx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}
	var counter int64 // requests-mode budget
	nextJob := func() bool {
		if duration > 0 {
			select {
			case <-runCtx.Done():
				return false
			default:
				return true
			}
		}
		return atomic.AddInt64(&counter, 1) <= int64(requests)
	}

	// Optional client-side rate limit.
	var tokens <-chan time.Time
	if rps > 0 {
		t := time.NewTicker(time.Second / time.Duration(rps))
		defer t.Stop()
		tokens = t.C
	}

	perWorker := make([][]reqOutcome, concurrency)
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			local := make([]reqOutcome, 0, 64)
			for nextJob() {
				if tokens != nil {
					select {
					case <-tokens:
					case <-runCtx.Done():
						return
					}
				}
				req, rerr := http.NewRequestWithContext(runCtx, method, target, bodyReader(body))
				if rerr != nil {
					local = append(local, reqOutcome{errKind: "error"})
					continue
				}
				for k, v := range headers {
					req.Header.Set(k, v)
				}
				t0 := time.Now()
				resp, derr := client.Do(req)
				lat := float64(time.Since(t0).Microseconds()) / 1000.0
				if derr != nil {
					local = append(local, reqOutcome{latencyMs: lat, errKind: errKindOf(derr)})
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				local = append(local, reqOutcome{latencyMs: lat, status: resp.StatusCode})
			}
			perWorker[idx] = local
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	return buildLoadReport(target, method, concurrency, elapsed, perWorker), nil
}

// bodyReader returns an io.Reader for the request body, or a true nil interface
// for an empty body (returning a typed (*strings.Reader)(nil) would make a
// non-nil io.Reader and break http.NewRequestWithContext).
func bodyReader(body string) io.Reader {
	if body == "" {
		return nil
	}
	return strings.NewReader(body)
}

// buildLoadReport aggregates per-worker outcomes into the report Ordered.
func buildLoadReport(target, method string, concurrency int, elapsed time.Duration, perWorker [][]reqOutcome) *scriptengine.Ordered {
	var sent, completed, failed, fivexx int
	statusCounts := map[int]int{}
	errCounts := map[string]int{}
	lats := make([]float64, 0, 1024)
	for _, ws := range perWorker {
		for _, o := range ws {
			sent++
			if o.status > 0 {
				completed++
				statusCounts[o.status]++
				if o.status >= 500 {
					fivexx++
				}
				lats = append(lats, o.latencyMs)
			} else {
				failed++
				errCounts[o.errKind]++
			}
		}
	}
	secs := elapsed.Seconds()
	rps := 0.0
	if secs > 0 {
		rps = float64(completed) / secs
	}
	errorRate := 0.0
	if sent > 0 {
		errorRate = float64(failed+fivexx) / float64(sent)
	}
	pct := percentiles(append([]float64(nil), lats...), 0, 50, 90, 95, 99, 100)
	mean := 0.0
	if len(lats) > 0 {
		sum := 0.0
		for _, v := range lats {
			sum += v
		}
		mean = sum / float64(len(lats))
	}
	round2 := func(f float64) float64 { return math.Round(f*100) / 100 }

	latency := scriptengine.NewOrdered().
		Set("min", round2(pct[0])).Set("mean", round2(mean)).
		Set("p50", round2(pct[1])).Set("p90", round2(pct[2])).
		Set("p95", round2(pct[3])).Set("p99", round2(pct[4])).
		Set("max", round2(pct[5]))

	sc := scriptengine.NewOrdered()
	scKeys := make([]int, 0, len(statusCounts))
	for k := range statusCounts {
		scKeys = append(scKeys, k)
	}
	sort.Ints(scKeys)
	for _, k := range scKeys {
		sc.Set(fmt.Sprintf("%d", k), statusCounts[k])
	}
	ec := scriptengine.NewOrdered()
	ecKeys := make([]string, 0, len(errCounts))
	for k := range errCounts {
		ecKeys = append(ecKeys, k)
	}
	sort.Strings(ecKeys)
	for _, k := range ecKeys {
		ec.Set(k, errCounts[k])
	}

	return scriptengine.NewOrdered().
		Set("target", target).Set("method", method).
		Set("concurrency", concurrency).
		Set("durationMs", elapsed.Milliseconds()).
		Set("sent", sent).Set("completed", completed).Set("failed", failed).
		Set("rps", round2(rps)).Set("errorRate", round2(errorRate)).
		Set("latency", latency).
		Set("statusCounts", sc).
		Set("errors", ec)
}

// loadNamespace builds the net.load member map.
func loadNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"http": scriptengine.PromisifyAsync(vm, loop, loadHTTPOp),
	}
}
