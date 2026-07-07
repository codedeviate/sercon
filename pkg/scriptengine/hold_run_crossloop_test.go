package scriptengine

import (
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// A HoldRun release that fires after the engine has moved on to a later Run
// must clear its sentinel against the loop that OWNS it (captured at HoldRun
// time), never the engine's current holdRunLoop. Clearing the wrong loop
// decrements that loop's jobCount and calls removeJob with a foreign index
// — corrupting the new Run's loop (premature exit or an out-of-range panic).
//
// This drives the loop lifecycle directly to place a release in exactly that
// cross-loop window: park a sentinel on loopA, repoint holdRunLoop at loopB
// (as a new Run would), park a known job on loopB, then fire the release.
// loopB must remain intact and keep processing jobs.
func TestHoldRun_LateReleaseDoesNotCorruptNextLoop(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	loopA := eventloop.NewEventLoop()
	loopB := eventloop.NewEventLoop()
	loopA.Start()
	defer loopA.Stop()
	loopB.Start()
	defer loopB.Stop()

	// Run A: register loopA and park a sentinel on loopA's own goroutine.
	eng.holdRunBegin(loopA)
	var release func()
	held := make(chan struct{})
	loopA.RunOnLoop(func(*goja.Runtime) {
		release = eng.HoldRun("A")
		close(held)
	})
	<-held

	// The engine moves on to Run B (loopB) while A's release is still
	// outstanding — the exact window the fix addresses. loopB has NO timer
	// jobs of its own, so a wrong-loop ClearTimeout (buggy path) calls
	// removeJob with loopA's index against loopB's empty jobs slice and
	// panics on loopB's goroutine; the fix targets loopA and leaves loopB
	// untouched.
	eng.holdRunMu.Lock()
	eng.holdRunLoop = loopB
	eng.holdRunMu.Unlock()

	// Fire the late release. Must target loopA, not loopB.
	release()
	time.Sleep(50 * time.Millisecond)

	// loopB must still be alive and processing jobs.
	pinged := make(chan struct{})
	if !loopB.RunOnLoop(func(*goja.Runtime) { close(pinged) }) {
		t.Fatal("loopB rejected a job after the cross-loop release (loop corrupted/terminated)")
	}
	select {
	case <-pinged:
	case <-time.After(2 * time.Second):
		t.Fatal("loopB stopped processing jobs after the cross-loop release")
	}
}
