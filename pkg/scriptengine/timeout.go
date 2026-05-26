package scriptengine

import "errors"

// ErrScriptTimeout is returned when a script exceeds the configured Timeout.
//
// The interrupt + cancellation watcher that actually fires this lives inline
// inside Engine.Run (see engine.go) — that goroutine has visibility into both
// the timeout and the host context, plus the per-Run atomic flags needed to
// distinguish the two error paths after the fact.
var ErrScriptTimeout = errors.New("script timeout")
