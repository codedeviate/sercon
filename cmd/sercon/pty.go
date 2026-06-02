package main

import "errors"

// errPTYUnsupported is returned by startPTY on platforms without a PTY
// implementation (Windows). execShell treats it as a signal to fall back to
// the normal pipe path so { pty: true } degrades gracefully (no color)
// rather than erroring.
var errPTYUnsupported = errors.New("pty: not supported on this platform")
