package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// screenshotArgs builds the screenshot command. It NEVER appends a path
// positional — the CLI writes a temp file we read afterwards — which
// sidesteps the ambiguous `screenshot [selector] [path]` grammar. An
// optional selector scopes the capture.
func screenshotArgs(opts map[string]any) []string {
	args := []string{"screenshot"}
	if sel, _ := opts["selector"].(string); sel != "" {
		args = append(args, sel)
	}
	if b, _ := opts["full"].(bool); b {
		args = append(args, "--full")
	}
	if b, _ := opts["annotate"].(bool); b {
		args = append(args, "--annotate")
	}
	if f, _ := opts["format"].(string); f != "" {
		args = append(args, "--screenshot-format", f)
	}
	if q, ok := opts["quality"]; ok {
		args = append(args, "--screenshot-quality", fmt.Sprintf("%v", numToInt(q)))
	}
	return args
}

// abCapturePath extracts data.path from a `{success,data,error}` capture
// response.
func abCapturePath(stdout string) (string, error) {
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return "", fmt.Errorf("agentBrowser: parsing capture response: %w", err)
	}
	p, _ := resp.Data["path"].(string)
	if p == "" {
		return "", errors.New("agentBrowser: capture response contained no file path")
	}
	return p, nil
}

// captureFormat returns the format label for the result object.
func captureFormat(opts map[string]any, def string) string {
	if f, _ := opts["format"].(string); f != "" {
		return f
	}
	return def
}

// deliverCapture reads tempPath, then either relocates the bytes to userPath
// (returning { path, size, format }) or returns them inline as bytes
// (returning { bytes, format }). The temp file is always removed.
func deliverCapture(tempPath, userPath, format string) (any, error) {
	defer os.Remove(tempPath) //nolint:errcheck // best-effort cleanup
	data, err := os.ReadFile(tempPath)
	if err != nil {
		return nil, fmt.Errorf("agentBrowser: reading capture output: %w", err)
	}
	o := scriptengine.NewOrdered()
	if userPath != "" {
		if err := os.WriteFile(userPath, data, 0o644); err != nil {
			return nil, fmt.Errorf("agentBrowser: writing capture to %s: %w", userPath, err)
		}
		o.Set("path", userPath)
		o.Set("size", len(data))
		o.Set("format", format)
		return o, nil
	}
	o.Set("bytes", data) // []byte -> JS binary (see d.ts note in Task 5)
	o.Set("format", format)
	return o, nil
}

// screenshotParams carries the extracted screenshot(path?, opts?) inputs.
type screenshotParams struct {
	userPath string
	opts     map[string]any
}

// screenshotExtract disambiguates arg0, which may be a path string OR the
// opts object.
func screenshotExtract(call goja.FunctionCall) (screenshotParams, error) {
	p := screenshotParams{opts: map[string]any{}}
	if m, ok := call.Argument(0).Export().(map[string]any); ok {
		p.opts = m
	} else {
		p.userPath = strArg(call, 0)
		if m, ok := call.Argument(1).Export().(map[string]any); ok {
			p.opts = m
		}
	}
	return p, nil
}

// screenshot captures the page. Signature: screenshot(path?, opts?).
func (h *abHandle) screenshot(ctx context.Context, p screenshotParams) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, screenshotArgs(p.opts)...)
	if err != nil {
		return nil, err
	}
	tempPath, err := abCapturePath(out)
	if err != nil {
		return nil, err
	}
	return deliverCapture(tempPath, p.userPath, captureFormat(p.opts, "png"))
}

// pdf saves the page as PDF. Signature: pdf(path?, opts?). The CLI requires a
// path, so when the caller wants bytes we use a temp .pdf.
func (h *abHandle) pdf(ctx context.Context, userPath string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	target := userPath
	cleanup := ""
	if target == "" {
		f, err := os.CreateTemp("", "sercon-ab-*.pdf")
		if err != nil {
			return nil, fmt.Errorf("agentBrowser.pdf: temp file: %w", err)
		}
		target = f.Name()
		_ = f.Close()
		cleanup = target
	}
	if _, err := abRunChecked(ctx, h.session, h.global, h.timeout, "pdf", target); err != nil {
		if cleanup != "" {
			_ = os.Remove(cleanup)
		}
		return nil, err
	}
	if userPath != "" {
		// CLI wrote directly to the user's path; report metadata.
		info, err := os.Stat(userPath)
		if err != nil {
			return nil, fmt.Errorf("agentBrowser.pdf: stat output: %w", err)
		}
		o := scriptengine.NewOrdered()
		o.Set("path", userPath)
		o.Set("size", int(info.Size()))
		o.Set("format", "pdf")
		return o, nil
	}
	return deliverCapture(target, "", "pdf")
}

// addCapture wires screenshot/pdf into the handle object.
func (h *abHandle) addCapture(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["screenshot"] = abAsync(vm, loop, screenshotExtract, h.screenshot)
	obj["pdf"] = abAsync(vm, loop, abStrArg0, h.pdf)
}
