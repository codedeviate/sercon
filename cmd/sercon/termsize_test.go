package main

import "testing"

// A non-terminal fd (-1) must report tty=false and the $COLUMNS/$LINES-or-80x24
// fallback rather than erroring.
func TestTerminalSize_NonTTYFallback(t *testing.T) {
	t.Setenv("COLUMNS", "")
	t.Setenv("LINES", "")
	cols, rows, tty := terminalSize(-1)
	if tty {
		t.Fatalf("fd -1 should not be a tty")
	}
	if cols != 80 || rows != 24 {
		t.Fatalf("fallback = %dx%d, want 80x24", cols, rows)
	}
}

func TestTerminalSize_EnvOverride(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	t.Setenv("LINES", "40")
	cols, rows, tty := terminalSize(-1)
	if tty || cols != 120 || rows != 40 {
		t.Fatalf("env override = %dx%d tty=%v, want 120x40 tty=false", cols, rows, tty)
	}
}

func TestEnvDim(t *testing.T) {
	t.Setenv("X", "0") // non-positive is rejected → fallback
	if got := envDim("X", 24); got != 24 {
		t.Fatalf("non-positive should fall back: got %d", got)
	}
	t.Setenv("X", "notanumber")
	if got := envDim("X", 24); got != 24 {
		t.Fatalf("unparseable should fall back: got %d", got)
	}
	t.Setenv("X", "100")
	if got := envDim("X", 24); got != 100 {
		t.Fatalf("valid should win: got %d", got)
	}
}
