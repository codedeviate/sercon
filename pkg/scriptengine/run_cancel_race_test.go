package scriptengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// Run must remain killable even when the deadline/cancel watcher wins the
// race against the loop callback storing the vm. With an already-cancelled
// context, a non-terminating script must return the cancellation error
// promptly — never hang. Before the fix, if the watcher ran Terminate
// before loop.Run's setRunning (which resets terminated=false), and loaded
// vmRef==nil (skipping vm.Interrupt), the script ran uninterruptibly.
func TestRun_PreCancelledContextNeverHangs(t *testing.T) {
	for i := 0; i < 300; i++ {
		eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // already cancelled before Run starts

		done := make(chan error, 1)
		go func() {
			_, err := eng.Run(ctx, "loop.ts", `while (true) {}`)
			done <- err
		}()

		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("iter %d: expected a cancellation error, got nil", i)
			}
		case <-time.After(4 * time.Second):
			t.Fatalf("iter %d: Run hung on a pre-cancelled context (watcher/vmRef kill-path race)", i)
		}
	}
}

// Same invariant via a tiny timeout instead of a pre-cancelled context.
func TestRun_TinyTimeoutNeverHangs(t *testing.T) {
	for i := 0; i < 300; i++ {
		eng := scriptengine.New(scriptengine.Options{
			ScriptRoot:     t.TempDir(),
			DisableConsole: true,
			Timeout:        time.Millisecond,
		})
		done := make(chan error, 1)
		go func() {
			_, err := eng.Run(context.Background(), "loop.ts", `while (true) {}`)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("iter %d: expected a timeout error, got nil", i)
			}
		case <-time.After(4 * time.Second):
			t.Fatalf("iter %d: Run hung despite a 1ms timeout", i)
		}
	}
}
