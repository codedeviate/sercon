package scriptengine

import "errors"

// ErrScriptTimeout is returned when a script exceeds the configured Timeout.
//
// The interrupt + cancellation watcher that actually fires this lives inline
// inside Engine.Run (see engine.go) — that goroutine has visibility into both
// the timeout and the host context, plus the per-Run atomic flags needed to
// distinguish the two error paths after the fact.
var ErrScriptTimeout = errors.New("script timeout")

// ErrTranspile is returned (wrapped) when esbuild rejects the entry script
// or a required module. Hosts can use `errors.Is(err, ErrTranspile)` to
// distinguish "the script never ran" from "the script ran and threw" — the
// sercon CLI maps this to a distinct exit code.
var ErrTranspile = errors.New("transpile error")
