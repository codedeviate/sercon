package main

import (
	"io"
	"os"
)

// The stdio registry. These are package vars for the same reason
// consoleOut/consoleErr were: cmd/sercon is the process, and there is exactly
// one set of standard streams. (CLAUDE.md's no-package-state rule scopes to
// pkg/scriptengine, which this plan does not touch.)
var (
	stdioOutStream = newStream("stdout", os.Stdout)
	stdioErrStream = newStream("stderr", os.Stderr)
)

// stdioOut / stdioErr are what every script-facing writer writes through.
// They return the stable stream object, so a caller may hold the writer
// indefinitely and still follow later redirects.
func stdioOut() io.Writer { return stdioOutStream }
func stdioErr() io.Writer { return stdioErrStream }

// resetStdio drops every redirect on both streams, closing any files they
// opened. Called at the START of each Run (not the end) so that:
//   - `sercon a.ts b.ts` gives b.ts clean streams,
//   - each --watch re-run starts clean,
//   - the CLI's post-run FAIL / PASS / --verbose reporting still lands wherever
//     the script left the stream pointed.
//
// It is ALSO called once on the way out of each entry point — run() in main.go,
// runRun() in run_cmd.go, runServe() in serve_cmd.go — deferred so it lands
// after those final reporting writes. Without that exit drain the last (or
// only) run's stack is never popped, and a line-callback destination the script
// left in place swallows the exit-time output for good: the bytes are queued
// for a handler that can no longer run (the event loop is gone, so
// loop.RunOnLoop refuses the drain) and the process exits with them still in
// the queue. Popping the entry flushes whatever it holds to the destination
// beneath instead — see stream.closeDest.
//
// The --watch loop resets a third time, just before printing its re-run banner:
// the banner is written before runOnceForWatch reaches runOne's own reset, so
// it would otherwise land in the previous run's redirect (see watch.go).
func resetStdio() {
	stdioOutStream.reset()
	stdioErrStream.reset()
	stdioInSource.reset()
}
