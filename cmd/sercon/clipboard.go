package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// clipboard.go backs runtime.clipboard: read/write the host OS system clipboard
// text via the platform clipboard CLI. No pure-Go, no-cgo library covers every
// platform, so this is an external-CLI fallback (authorized): feature-detected
// on PATH, with a clean thrown error when no backend exists. The static binary
// stays fully functional without any of these tools.

const clipboardTimeout = 5 * time.Second

// clipboardBackend resolves the read and write argv for the host, or returns
// ok=false with a tailored reason when no backend is on PATH. Pure: callers
// pass GOOS, whether a Wayland session is present, and a PATH-lookup predicate,
// so it is testable without touching the real environment. readArgv/writeArgv
// are full argv slices (index 0 is the binary).
func clipboardBackend(goos string, wayland bool, look func(string) bool) (readArgv, writeArgv []string, ok bool, reason string) {
	switch goos {
	case "darwin":
		if look("pbpaste") && look("pbcopy") {
			return []string{"pbpaste"}, []string{"pbcopy"}, true, ""
		}
		return nil, nil, false, "runtime.clipboard: pbpaste/pbcopy not found on PATH"
	case "linux":
		if wayland && look("wl-paste") && look("wl-copy") {
			return []string{"wl-paste", "--no-newline"}, []string{"wl-copy"}, true, ""
		}
		if look("xclip") {
			return []string{"xclip", "-selection", "clipboard", "-o"},
				[]string{"xclip", "-selection", "clipboard", "-i"}, true, ""
		}
		if look("xsel") {
			return []string{"xsel", "--clipboard", "--output"},
				[]string{"xsel", "--clipboard", "--input"}, true, ""
		}
		return nil, nil, false, "runtime.clipboard: no clipboard backend found (install xclip, xsel, or wl-clipboard)"
	case "windows":
		if look("powershell") && look("clip") {
			return []string{"powershell", "-NoProfile", "-Command", "Get-Clipboard"},
				[]string{"clip"}, true, ""
		}
		return nil, nil, false, "runtime.clipboard: powershell/clip not found on PATH"
	default:
		return nil, nil, false, fmt.Sprintf("runtime.clipboard: unsupported platform %q", goos)
	}
}

// lookPath adapts exec.LookPath to the bool predicate clipboardBackend wants.
func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// resolveClipboardBackend is the production wiring of clipboardBackend.
func resolveClipboardBackend() (readArgv, writeArgv []string, ok bool, reason string) {
	return clipboardBackend(runtime.GOOS, os.Getenv("WAYLAND_DISPLAY") != "", lookPath)
}

// clipboardAvailable is a cheap, side-effect-free advisory: is a clipboard
// backend resolvable on PATH? Never touches the clipboard.
func clipboardAvailable() bool {
	_, _, ok, _ := resolveClipboardBackend()
	return ok
}

// trimWindowsClipboardNewline strips a single trailing CRLF/LF that PowerShell
// Get-Clipboard appends. Applied only on the Windows read path.
func trimWindowsClipboardNewline(s string) string {
	if strings.HasSuffix(s, "\r\n") {
		return strings.TrimSuffix(s, "\r\n")
	}
	return strings.TrimSuffix(s, "\n")
}

// clipReadOp runs the resolved read backend and returns the clipboard text.
func clipReadOp(ctx context.Context, _ goja.FunctionCall) (any, error) {
	readArgv, _, ok, reason := resolveClipboardBackend()
	if !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	runCtx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, readArgv[0], readArgv[1:]...) //nolint:gosec // fixed platform clipboard argv
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("runtime.clipboard.read: %s: %w%s", readArgv[0], err, stderrSuffix(errb.String()))
	}
	text := out.String()
	if runtime.GOOS == "windows" {
		text = trimWindowsClipboardNewline(text)
	}
	return text, nil
}

// clipWriteOp runs the resolved write backend, feeding text via stdin.
func clipWriteOp(ctx context.Context, call goja.FunctionCall) (any, error) {
	_, writeArgv, ok, reason := resolveClipboardBackend()
	if !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	text := call.Argument(0).String() // JS String() coercion
	runCtx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, writeArgv[0], writeArgv[1:]...) //nolint:gosec // fixed platform clipboard argv
	cmd.Stdin = strings.NewReader(text)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("runtime.clipboard.write: %s: %w%s", writeArgv[0], err, stderrSuffix(errb.String()))
	}
	return nil, nil
}

// stderrSuffix renders a truncated ": <stderr>" suffix for error messages, or
// "" when stderr was empty.
func stderrSuffix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return ": " + s
}

// clipboardNamespace builds the runtime.clipboard member map.
func clipboardNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"available": clipboardAvailable(),
		"read":      scriptengine.PromisifyAsync(vm, loop, clipReadOp),
		"write":     scriptengine.PromisifyAsync(vm, loop, clipWriteOp),
	}
}
