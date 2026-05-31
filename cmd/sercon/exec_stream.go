package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// execStreamFn implements services.exec.stream(cmd, onLine, opts?). It spawns a
// subprocess and streams its stdout/stderr to onLine(line, stream) line by line
// as output arrives, returning a Promise that resolves to
// { exitCode, success, durationMs } on exit. Unlike exec.shell it does not
// buffer output.
//
// It is hand-rolled (not PromisifyAsync) because it dispatches many per-line
// callbacks on the loop AND resolves once on exit; it mirrors capture.go's
// LoopCallable + HoldRun structure.
//
// cmd: string → /bin/sh -c (cmd /C on Windows); string[] → argv. opts:
// { cwd?, env?, stdin?, timeout? } — timeout in ms, optional with NO default
// (0 / absent = run until exit). A non-zero exit resolves with success:false;
// spawn failure and timeout reject. onLine must be a function (synchronous
// TypeError otherwise).
func execStreamFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		cmdArg := call.Argument(0)
		if cmdArg == nil || goja.IsUndefined(cmdArg) || goja.IsNull(cmdArg) {
			panic(vm.NewTypeError("services.exec.stream: cmd argument required"))
		}
		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(vm.NewTypeError("services.exec.stream: second argument (onLine) must be a function"))
		}
		handler := scriptengine.NewLoopCallable(loop, fn)

		// Parse opts on the loop (goja values aren't safe off-loop).
		opts := map[string]any{}
		if o := call.Argument(2); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
			if m, ok := o.Export().(map[string]any); ok {
				opts = m
			}
		}
		cwd := optString(opts, "cwd", "")
		stdin := optString(opts, "stdin", "")
		timeout := optMillis(opts, "timeout", 0)
		env := buildEnv(opts)

		promise, resolve, reject := vm.NewPromise()

		argv, err := buildArgv(cmdArg)
		if err != nil {
			// Reject asynchronously so the return value is always a Promise.
			_ = reject(vm.NewGoError(fmt.Errorf("services.exec.stream: %w", err)))
			return vm.ToValue(promise)
		}

		// Keep loop.Run alive across the off-loop subprocess: a bare promise +
		// goroutine does not bump the loop's jobCount. Released once, on the
		// loop, in the waiter below.
		runHold := eng.HoldRun("exec.stream " + argv[0])

		go func() {
			start := time.Now()

			var (
				ctx    context.Context
				cancel context.CancelFunc
			)
			if timeout > 0 {
				ctx, cancel = context.WithTimeout(context.Background(), timeout)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()

			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // user-supplied argv is intentional
			if cwd != "" {
				cmd.Dir = cwd
			}
			if env != nil {
				cmd.Env = env
			}
			if stdin != "" {
				cmd.Stdin = strings.NewReader(stdin)
			}
			configureProcessTermination(cmd)

			runErr := runStream(cmd, handler)
			durationMs := time.Since(start).Milliseconds()

			exitCode := 0
			success := true
			var rejectErr error
			if runErr != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					rejectErr = ctxErr
				} else {
					var exitErr *exec.ExitError
					if errors.As(runErr, &exitErr) {
						exitCode = exitErr.ExitCode()
						success = false
					} else {
						rejectErr = runErr
					}
				}
			}

			loop.RunOnLoop(func(vm *goja.Runtime) {
				runHold()
				if rejectErr != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("services.exec.stream: %w", rejectErr)))
					return
				}
				res := scriptengine.NewOrdered().
					Set("exitCode", exitCode).
					Set("success", success).
					Set("durationMs", durationMs)
				_ = resolve(scriptengine.OrderedToValue(vm, res))
			})
		}()

		return vm.ToValue(promise)
	}
}

// runStream wires the command's stdout/stderr pipes, starts it, scans both
// streams line by line into handler, waits for the streams to drain, and waits
// for the process to exit. It returns the first setup error, or the result of
// cmd.Wait() (nil, *exec.ExitError for a non-zero exit, or another error).
func runStream(cmd *exec.Cmd, handler *scriptengine.LoopCallable) error {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go streamPipe(&wg, handler, stdoutPipe, "stdout")
	go streamPipe(&wg, handler, stderrPipe, "stderr")
	// os/exec requires draining the pipes before Wait. Both streamPipe
	// goroutines run to EOF in the normal case; they only bail early on loop
	// teardown, after which the process is killed via the context and the
	// WaitDelay set by configureProcessTermination bounds how long Wait blocks.
	wg.Wait()

	return cmd.Wait()
}

// streamPipe scans r line by line (trailing newline stripped; a final
// unterminated line is still delivered) and calls handler(line, stream) for
// each. The scan buffer starts at 64 KiB and is allowed to grow to 1 MiB so
// long lines don't error. If handler.Call returns an error (the event loop was
// terminated), scanning stops.
func streamPipe(wg *sync.WaitGroup, handler *scriptengine.LoopCallable, r io.Reader, stream string) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if _, err := handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
			return []goja.Value{vm.ToValue(line), vm.ToValue(stream)}, nil
		}); err != nil {
			return
		}
	}
}
