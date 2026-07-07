package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// clipNoArgsExtract is the extract half for the argument-less clipboard ops.
func clipNoArgsExtract(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil }

// clipReadOp runs the resolved read backend and returns the clipboard text.
func clipReadOp(ctx context.Context, _ struct{}) (any, error) {
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

// clipWriteExtract coerces the text argument on the event loop.
func clipWriteExtract(call goja.FunctionCall) (string, error) {
	return call.Argument(0).String(), nil // JS String() coercion
}

// clipWriteOp runs the resolved write backend, feeding text via stdin.
func clipWriteOp(ctx context.Context, text string) (any, error) {
	_, writeArgv, ok, reason := resolveClipboardBackend()
	if !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	runCtx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	if err := feedStdinWrite(runCtx, writeArgv, strings.NewReader(text)); err != nil {
		return nil, fmt.Errorf("runtime.clipboard.write: %w", err)
	}
	return nil, nil
}

// feedStdinWrite runs a clipboard WRITE command, feeding the payload on stdin.
// It routes the child's stdout/stderr to os.DevNull rather than capturing them
// into a pipe: xclip and wl-copy FORK a background process to own the X/Wayland
// selection, and that daemon inherits any captured pipe's write end — so a
// pipe-backed Stderr would make cmd.Wait block until the (never-exiting) daemon
// closed it. Writing to a *os.File (DevNull) means os/exec spawns no copier
// goroutine, so Wait returns when the parent process exits. (pbcopy / osascript
// don't fork, but DevNull is correct for them too; the trade-off is that a rare
// write failure surfaces only the exit error, not the tool's stderr text.)
func feedStdinWrite(ctx context.Context, argv []string, stdin io.Reader) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // fixed platform clipboard argv
	cmd.Stdin = stdin
	if devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		cmd.Stdout = devnull
		cmd.Stderr = devnull
		defer func() { _ = devnull.Close() }()
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", argv[0], err)
	}
	return nil
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

// pngSignature is the full 8-byte PNG signature (barcode_decode.go already has
// a 4-byte pngMagic for format sniffing; clipboard write validation wants the
// complete signature).
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// isPNG reports whether b begins with the PNG signature.
func isPNG(b []byte) bool { return len(b) >= 8 && bytes.Equal(b[:8], pngSignature) }

// imageStrategy describes how PNG clipboard ops run on this host. For wl/xclip
// the argv slices stream PNG via stdout (read) / stdin (write); darwin and
// windows use kind-specific code paths (temp file + osascript / PowerShell).
type imageStrategy struct {
	kind      string
	readArgv  []string
	writeArgv []string
}

// clipboardImageBackend resolves the PNG image strategy, or ok=false + reason.
// Pure: goos / wayland / look are injected for testing.
func clipboardImageBackend(goos string, wayland bool, look func(string) bool) (imageStrategy, bool, string) {
	switch goos {
	case "darwin":
		if look("pngpaste") && look("osascript") {
			return imageStrategy{kind: "darwin"}, true, ""
		}
		return imageStrategy{}, false, "runtime.clipboard: image read needs pngpaste (e.g. `brew install pngpaste`)"
	case "linux":
		if wayland && look("wl-copy") && look("wl-paste") {
			return imageStrategy{kind: "wl",
				readArgv:  []string{"wl-paste", "--type", "image/png"},
				writeArgv: []string{"wl-copy", "--type", "image/png"}}, true, ""
		}
		if look("xclip") {
			return imageStrategy{kind: "xclip",
				readArgv:  []string{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"},
				writeArgv: []string{"xclip", "-selection", "clipboard", "-t", "image/png", "-i"}}, true, ""
		}
		return imageStrategy{}, false, "runtime.clipboard: image needs wl-clipboard or xclip (xsel cannot handle images)"
	case "windows":
		if look("powershell") {
			return imageStrategy{kind: "windows"}, true, ""
		}
		return imageStrategy{}, false, "runtime.clipboard: image needs PowerShell"
	default:
		return imageStrategy{}, false, fmt.Sprintf("runtime.clipboard: image unsupported on %q", goos)
	}
}

func resolveClipboardImageBackend() (imageStrategy, bool, string) {
	return clipboardImageBackend(runtime.GOOS, os.Getenv("WAYLAND_DISPLAY") != "", lookPath)
}

// clipboardImageAvailable reports whether PNG read+write are usable on PATH.
func clipboardImageAvailable() bool {
	_, ok, _ := resolveClipboardImageBackend()
	return ok
}

// runCapturePNG runs argv capturing stdout as PNG bytes. A non-zero exit with
// empty stdout means "no image on the clipboard" → (nil, nil); a non-zero exit
// WITH stdout, or any other failure, is an error.
func runCapturePNG(ctx context.Context, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // fixed clipboard argv
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if out.Len() == 0 {
			return nil, nil // no image present
		}
		return nil, fmt.Errorf("%s: %w%s", argv[0], err, stderrSuffix(errb.String()))
	}
	return out.Bytes(), nil
}

// feedPNGStdin runs argv with png on stdin.
// feedPNGStdin writes png to the clipboard via argv on stdin. Uses
// feedStdinWrite so the forking xclip/wl-copy daemon can't hang cmd.Wait
// (see feedStdinWrite).
func feedPNGStdin(ctx context.Context, argv []string, png []byte) error {
	return feedStdinWrite(ctx, argv, bytes.NewReader(png))
}

// writeTempPNG writes png to a temp .png file and returns its path; caller removes it.
func writeTempPNG(png []byte) (string, error) {
	f, err := os.CreateTemp("", "sercon-clip-*.png")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(png); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// darwinWriteImagePNG sets the macOS clipboard image from PNG bytes via osascript.
func darwinWriteImagePNG(ctx context.Context, png []byte) error {
	path, err := writeTempPNG(png)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	script := fmt.Sprintf("set the clipboard to (read (POSIX file %q) as «class PNGf»)", path)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script) //nolint:gosec
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("osascript: %w%s", err, stderrSuffix(errb.String()))
	}
	return nil
}

// winReadImagePNG reads the Windows clipboard image and returns PNG bytes (nil
// if the clipboard holds no image).
func winReadImagePNG(ctx context.Context) ([]byte, error) {
	path, err := writeTempPNG(nil) // create an empty temp file path to receive the PNG
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(path) }()
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms,System.Drawing; `+
		`$img=[System.Windows.Forms.Clipboard]::GetImage(); `+
		`if ($img -ne $null) { $img.Save(%q,[System.Drawing.Imaging.ImageFormat]::Png) }`, path)
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", ps) //nolint:gosec
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("powershell: %w%s", err, stderrSuffix(errb.String()))
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, nil // no image
	}
	return data, nil
}

// winWriteImagePNG sets the Windows clipboard image from PNG bytes.
func winWriteImagePNG(ctx context.Context, png []byte) error {
	path, err := writeTempPNG(png)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms,System.Drawing; `+
		`$img=[System.Drawing.Image]::FromFile(%q); `+
		`[System.Windows.Forms.Clipboard]::SetImage($img)`, path)
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", ps) //nolint:gosec
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("powershell: %w%s", err, stderrSuffix(errb.String()))
	}
	return nil
}

// clipImageReadOp backs runtime.clipboard.readImage(). Resolves to PNG bytes
// ([]byte → Uint8Array) or null when the clipboard holds no image.
func clipImageReadOp(ctx context.Context, _ struct{}) (any, error) {
	strat, ok, reason := resolveClipboardImageBackend()
	if !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	runCtx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	var (
		data []byte
		err  error
	)
	switch strat.kind {
	case "wl", "xclip":
		data, err = runCapturePNG(runCtx, strat.readArgv)
	case "darwin":
		data, err = runCapturePNG(runCtx, []string{"pngpaste", "-"})
	case "windows":
		data, err = winReadImagePNG(runCtx)
	}
	if err != nil {
		return nil, fmt.Errorf("runtime.clipboard.readImage: %w", err)
	}
	if len(data) == 0 {
		return nil, nil // null: no image on clipboard
	}
	return data, nil
}

// clipImageWriteExtract exports the PNG argument on the event loop. The
// copy matters: goja's Export of a Uint8Array returns the typed array's
// live backing store, and the work goroutine reads the bytes off-loop.
func clipImageWriteExtract(call goja.FunctionCall) ([]byte, error) {
	png, isBytes := call.Argument(0).Export().([]byte)
	if !isBytes {
		return nil, fmt.Errorf("runtime.clipboard.writeImage: expected a Uint8Array of PNG bytes")
	}
	return append([]byte(nil), png...), nil
}

// clipImageWriteOp backs runtime.clipboard.writeImage(png). Validates PNG magic.
func clipImageWriteOp(ctx context.Context, png []byte) (any, error) {
	strat, ok, reason := resolveClipboardImageBackend()
	if !ok {
		return nil, fmt.Errorf("%s", reason)
	}
	if !isPNG(png) {
		return nil, fmt.Errorf("runtime.clipboard.writeImage: data is not a PNG (bad signature)")
	}
	runCtx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	switch strat.kind {
	case "wl", "xclip":
		return nil, errImagePrefix(feedPNGStdin(runCtx, strat.writeArgv, png))
	case "darwin":
		return nil, errImagePrefix(darwinWriteImagePNG(runCtx, png))
	case "windows":
		return nil, errImagePrefix(winWriteImagePNG(runCtx, png))
	}
	return nil, nil
}

// errImagePrefix tags a write error with the binding name (nil passes through).
func errImagePrefix(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("runtime.clipboard.writeImage: %w", err)
}

// clipboardNamespace builds the runtime.clipboard member map.
func clipboardNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"available":      clipboardAvailable(),
		"imageAvailable": clipboardImageAvailable(),
		"read":           scriptengine.PromisifyAsync(vm, loop, clipNoArgsExtract, clipReadOp),
		"write":          scriptengine.PromisifyAsync(vm, loop, clipWriteExtract, clipWriteOp),
		"readImage":      scriptengine.PromisifyAsync(vm, loop, clipNoArgsExtract, clipImageReadOp),
		"writeImage":     scriptengine.PromisifyAsync(vm, loop, clipImageWriteExtract, clipImageWriteOp),
	}
}
