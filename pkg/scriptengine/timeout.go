package scriptengine

import (
	"context"
	"errors"
	"time"

	"github.com/dop251/goja"
)

// ErrScriptTimeout is returned when a script exceeds the configured Timeout.
var ErrScriptTimeout = errors.New("script timeout")

// withInterrupt runs fn while a watcher goroutine monitors both ctx.Done()
// and an optional timeout. If either fires, vm.Interrupt is invoked so the
// VM aborts the next bytecode step.
//
// The returned error distinguishes:
//   - ErrScriptTimeout if the timeout fired,
//   - ctx.Err() if the host context cancelled,
//   - the script error otherwise.
//
// The watcher goroutine always exits via the stop channel so timeout watchers
// do not accumulate across runs.
func withInterrupt(ctx context.Context, timeout time.Duration, vm *goja.Runtime, fn func() error) error {
	stop := make(chan struct{})
	var (
		timedOut bool
		canceled bool
	)

	go func() {
		var timeoutC <-chan time.Time
		if timeout > 0 {
			t := time.NewTimer(timeout)
			defer t.Stop()
			timeoutC = t.C
		}
		select {
		case <-stop:
			return
		case <-ctx.Done():
			canceled = true
			vm.Interrupt(ctx.Err())
		case <-timeoutC:
			timedOut = true
			vm.Interrupt(ErrScriptTimeout)
		}
	}()

	err := fn()
	close(stop)

	if err == nil {
		return nil
	}

	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		if timedOut {
			return ErrScriptTimeout
		}
		if canceled {
			return ctx.Err()
		}
	}
	return err
}
