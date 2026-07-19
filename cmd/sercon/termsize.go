package main

import (
	"os"
	"strconv"

	"github.com/dop251/goja"
	"golang.org/x/term"
)

// termSizeFn implements runtime.termSize(): the controlling terminal's size as
// { columns, rows, tty }. Synchronous — a single ioctl, no PromisifyAsync.
func termSizeFn(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		cols, rows, tty := terminalSize(int(os.Stdout.Fd()))
		return vm.ToValue(map[string]any{"columns": cols, "rows": rows, "tty": tty})
	}
}

// terminalSize returns (columns, rows, isTTY) for the given fd (stdout). When
// fd is not a terminal (piped/redirected) it reports tty=false and falls back to
// $COLUMNS/$LINES, then 80x24 — so callers can format output without having to
// special-case the non-TTY path, while `tty` still lets them detect it.
func terminalSize(fd int) (int, int, bool) {
	if w, h, err := term.GetSize(fd); err == nil && w > 0 && h > 0 {
		return w, h, true
	}
	return envDim("COLUMNS", 80), envDim("LINES", 24), false
}

// envDim reads a positive integer terminal dimension from an environment
// variable, falling back to the default when unset or unparseable.
func envDim(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
