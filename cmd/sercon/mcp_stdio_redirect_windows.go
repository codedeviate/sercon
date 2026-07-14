//go:build windows

package main

import (
	"errors"
	"os"
)

// installStdoutRedirect is unsupported on windows.
//
// The correctness guarantee the stdio transport relies on — moving ALL of
// sercon's own output off the stdout stream so it carries only JSON-RPC — can't
// be met on windows with a self-contained trick. goja_nodejs's console module
// captures the process's stdout `*os.File` at package-init time and writes to
// its handle directly; on unix we remap the underlying fd with dup2, but windows
// has no way to point an already-captured handle at a different device (there is
// no dup2 equivalent, and SetStdHandle only affects future GetStdHandle
// lookups, not the handle the logger already holds). Rather than ship a variant
// that could leak `console.log` output into the JSON-RPC stream and corrupt it
// — which the task forbids — stdio is refused cleanly on windows.
//
// A cross-platform fix would route the engine's console through a redirectable
// printer (console.RequireWithPrinter) instead of eventloop's default; that is a
// pkg/scriptengine change tracked as a follow-up.
func installStdoutRedirect() (saved *os.File, restore func() error, err error) {
	return nil, nil, errors.New("mcp: stdio() is not supported on windows (console output cannot be separated from the JSON-RPC stream); use listen() instead")
}
