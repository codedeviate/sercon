package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// searchProduce runs a search (walk/grep) off-loop, sending each result
// (already converted to a plain-Go value: string or map[string]any) to out.
// It must close out when done and stop promptly when ctx is cancelled. The
// closure is built ON the loop by the caller, capturing already-parsed
// plain-Go args — it must never touch goja values itself.
type searchProduce func(ctx context.Context, out chan<- any) error

// fsSearchStream builds a JS async iterator over a search producer. next()
// dequeues one result off-loop and resolves it on the loop; HoldRun keeps the
// event loop alive until the producer is drained (or the iterator is
// cancelled early via return()).
//
// Loop safety: callers MUST extract any goja args (fsFindExtract /
// fsGrepExtract) on the loop *before* calling this function, and build
// `produce` as a closure over the resulting plain-Go args. produce itself
// runs on a background goroutine and must not touch goja.
func fsSearchStream(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, produce searchProduce) goja.Value {
	ctx, cancel := context.WithCancel(scriptengine.RunContextFromVM(vm))
	out := make(chan any, 256)
	release := eng.HoldRun("fs.search stream")

	go func() {
		defer close(out)
		_ = produce(ctx, out) // producer errors just end the stream (best-effort)
	}()

	obj := vm.NewObject()
	var released atomic.Bool
	releaseOnce := func() {
		if !released.Swap(true) {
			cancel()
			release()
		}
	}

	_ = obj.Set("next", func(goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			item, ok := <-out
			loop.RunOnLoop(func(vm *goja.Runtime) {
				result := vm.NewObject()
				if !ok {
					releaseOnce()
					_ = result.Set("done", true)
					_ = result.Set("value", goja.Undefined())
				} else {
					_ = result.Set("done", false)
					_ = result.Set("value", vm.ToValue(item))
				}
				_ = resolve(result)
			})
		}()
		return vm.ToValue(promise)
	})

	// return(): allows `break`/early-exit from `for await` to cancel the walk
	// and release the HoldRun sentinel instead of leaking the producer
	// goroutine parked on a full/blocked channel send.
	_ = obj.Set("return", func(goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		releaseOnce()
		result := vm.NewObject()
		_ = result.Set("done", true)
		_ = result.Set("value", goja.Undefined())
		_ = resolve(result)
		return vm.ToValue(promise)
	})

	installVal, err := vm.RunProgram(installAsyncIteratorProg)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.search: install iterator: %w", err)))
	}
	installFn, ok := goja.AssertFunction(installVal)
	if !ok {
		panic(vm.NewGoError(fmt.Errorf("fs.search: install iterator: not callable")))
	}
	if _, err := installFn(goja.Undefined(), vm.ToValue(obj)); err != nil {
		panic(vm.NewGoError(fmt.Errorf("fs.search: install iterator: %w", err)))
	}
	return vm.ToValue(obj)
}

// streamFind walks per args (already parsed on the loop) and sends each
// surviving entry — a display-path string, or a stat map when args.extra.stat
// is set — to out. Every send is select'd against ctx so an early iterator
// return() (break in `for await`) unblocks the walk instead of leaking this
// goroutine on a full channel.
func streamFind(ctx context.Context, a fsFindArgs, out chan<- any) error {
	err := fsSearchWalk(ctx, a.walk, func(e walkEntry) error {
		if a.extra.nameRE != nil {
			target := e.name
			if a.extra.fullPath {
				target = e.rel
			}
			if !a.extra.nameRE.MatchString(target) {
				return nil
			}
		}
		disp := relDisplay(e.abs, a.extra.absolute)
		var item any
		if a.extra.stat {
			info, err := os.Lstat(e.abs)
			if err != nil {
				if a.walk.strict {
					return err
				}
				return nil
			}
			item = map[string]any{
				"path": disp, "type": e.typ,
				"size": info.Size(), "mtimeMs": info.ModTime().UnixMilli(),
			}
		} else {
			item = disp
		}
		select {
		case out <- item:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	})
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		return fmt.Errorf("fs.find: %w", err)
	}
	return nil
}

// streamGrep greps per args (already parsed on the loop) and sends each match
// map to out, honoring ctx cancellation on send the same way streamFind does.
func streamGrep(ctx context.Context, a grepArgs, out chan<- any) error {
	err := grepEachFile(ctx, a, func(matches []grepMatch, _ int) error {
		for _, m := range matches {
			select {
			case out <- grepMatchToMap(m):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
	if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
		return fmt.Errorf("fs.grep: %w", err)
	}
	return nil
}
