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
func resetStdio() {
	stdioOutStream.reset()
	stdioErrStream.reset()
	stdioInSource.reset()
}
