package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/dop251/goja"
)

// openerArgv returns the fixed platform-opener argv PREFIX (the caller appends
// the single target as one more argv element — never through a shell). ok is
// false when no opener is on PATH. look is exec.LookPath as a bool predicate,
// injected so the platform table is unit-testable without touching PATH.
func openerArgv(goos string, look func(string) bool) (argv []string, ok bool) {
	switch goos {
	case "darwin":
		if look("open") {
			return []string{"open"}, true
		}
	case "windows":
		// rundll32 ... FileProtocolHandler <target> opens URLs and files via the
		// registered handler; `cmd /c start "" <target>` is the fallback (start's
		// first quoted argument is the window title, so it must stay empty).
		if look("rundll32") {
			return []string{"rundll32", "url.dll,FileProtocolHandler"}, true
		}
		if look("cmd") {
			return []string{"cmd", "/c", "start", ""}, true
		}
	default: // linux, *bsd, …
		if look("xdg-open") {
			return []string{"xdg-open"}, true
		}
		if look("gnome-open") {
			return []string{"gnome-open"}, true
		}
	}
	return nil, false
}

// lookPathBool adapts exec.LookPath to the bool predicate openerArgv expects.
func lookPathBool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// openAvailable is a cheap, side-effect-free advisory: is an OS opener on PATH?
// Exposed as runtime.openAvailable so scripts can branch without a try/catch.
func openAvailable() bool {
	_, ok := openerArgv(runtime.GOOS, lookPathBool)
	return ok
}

// openExtract validates the single target argument on the event loop.
func openExtract(call goja.FunctionCall) (string, error) {
	t := call.Argument(0)
	if t == nil || goja.IsUndefined(t) || goja.IsNull(t) {
		return "", errors.New("runtime.open: target is required")
	}
	s := t.String()
	if s == "" {
		return "", errors.New("runtime.open: target is empty")
	}
	return s, nil
}

// openOp launches the platform opener with target as ONE argv element (no
// shell, so a URL with special characters can't inject), returning as soon as
// the child is spawned — fire-and-forget, the browser's lifetime is not awaited.
// The child is reaped in the background so it doesn't linger as a zombie.
func openOp(_ context.Context, target string) (any, error) {
	prefix, ok := openerArgv(runtime.GOOS, lookPathBool)
	if !ok {
		return nil, errors.New("runtime.open: no OS opener found on PATH (e.g. install xdg-open on Linux)")
	}
	args := append(append([]string{}, prefix[1:]...), target)
	cmd := exec.Command(prefix[0], args...) //nolint:gosec // fixed platform opener argv + single non-shell target
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("runtime.open: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil, nil
}
