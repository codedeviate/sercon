package scriptengine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// TestHoldRun_KeepsLoopAliveUntilRelease asserts that an engine binding
// holding a HoldRun sentinel keeps loop.Run from exiting, and releasing
// the hold lets the loop drain. Without HoldRun, loop.Run would return
// immediately after the script's top-level expression resolved.
func TestHoldRun_KeepsLoopAliveUntilRelease(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var release func()
	releaseAfter := 200 * time.Millisecond

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"hold": func() goja.Value {
				release = eng.HoldRun("test-hold")
				go func() {
					time.Sleep(releaseAfter)
					release()
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	_, err := eng.Run(context.Background(), "hold.ts", `test.hold();`)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if dur < releaseAfter {
		t.Fatalf("Run returned too early (%s); HoldRun should have kept loop alive >= %s", dur, releaseAfter)
	}
}

// TestHoldRun_MultipleHolders asserts that multiple concurrent HoldRun
// calls each need to release before the loop drains.
func TestHoldRun_MultipleHolders(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var releases []func()
	var mu sync.Mutex

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"hold": func() goja.Value {
				r := eng.HoldRun("multi")
				mu.Lock()
				releases = append(releases, r)
				mu.Unlock()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		rs := append([]func(){}, releases...)
		mu.Unlock()
		for _, r := range rs {
			r()
			time.Sleep(50 * time.Millisecond)
		}
	}()

	start := time.Now()
	_, err := eng.Run(context.Background(), "multi.ts", `test.hold(); test.hold(); test.hold();`)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if dur < 200*time.Millisecond {
		t.Fatalf("Run returned too early (%s); expected >= 200ms", dur)
	}
}

// TestHoldRun_ReleaseIsIdempotent asserts that calling release twice does
// not double-decrement.
func TestHoldRun_ReleaseIsIdempotent(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"holdAndRelease": func() goja.Value {
				r := eng.HoldRun("idempotent")
				go func() {
					time.Sleep(50 * time.Millisecond)
					r()
					r() // second call is no-op; must not panic, must not break refcount
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "idempotent.ts", `test.holdAndRelease();`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestHoldRun_DrainsOnRunEnd asserts that holds left at Run end are
// released by the engine's cleanup drain, so the next Run starts clean.
func TestHoldRun_DrainsOnRunEnd(t *testing.T) {
	eng := New(Options{DisableConsole: true, Timeout: 300 * time.Millisecond})
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"holdForever": func() goja.Value {
				_ = eng.HoldRun("forever") // intentionally do not release
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	// First Run will hit the engine timeout (300ms) because of the held sentinel.
	_, err := eng.Run(context.Background(), "first.ts", `test.holdForever();`)
	if !errors.Is(err, ErrScriptTimeout) {
		t.Fatalf("expected ErrScriptTimeout, got %v", err)
	}
	// Second Run starts with no inherited holds; the trivial script returns immediately.
	start := time.Now()
	_, err = eng.Run(context.Background(), "second.ts", `1+1;`)
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if dur > 100*time.Millisecond {
		t.Fatalf("second Run took %s; cleanup drain did not release held sentinel", dur)
	}
}

// TestHoldRunBegin_ResetsShutdownHooks asserts that holdRunBegin clears any
// leftover shutdownHooks at the start of a Run, mirroring the holdRunSentinels
// reset above it. This is belt-and-suspenders — drainRunCleanups already
// clears shutdownHooks at the end of every Run, so there's no known leak —
// but it keeps the two per-Run slices reset symmetrically at Run start.
func TestHoldRunBegin_ResetsShutdownHooks(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	// Register a hook the way a prior Run's long-lived binding would,
	// without going through a real Run (simulates a leftover that survived
	// past Run end despite drainRunCleanups).
	_ = eng.AddShutdownHook(func(context.Context) error { return nil })
	eng.runCleanupMu.Lock()
	before := len(eng.shutdownHooks)
	eng.runCleanupMu.Unlock()
	if before == 0 {
		t.Fatalf("setup: expected a leftover shutdown hook registered before Run")
	}

	var sawDuringRun int
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"check": func() goja.Value {
				eng.runCleanupMu.Lock()
				sawDuringRun = len(eng.shutdownHooks)
				eng.runCleanupMu.Unlock()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := eng.Run(context.Background(), "reset.ts", `test.check();`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if sawDuringRun != 0 {
		t.Fatalf("shutdownHooks not reset at Run start: len=%d (want 0)", sawDuringRun)
	}
}
