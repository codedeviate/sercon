package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// diffSnapshotArgs builds `diff snapshot [-b file] [-s sel] [-c] [-d n]`.
func diffSnapshotArgs(opts map[string]any) []string {
	args := []string{"diff", "snapshot"}
	if s, _ := opts["baseline"].(string); s != "" {
		args = append(args, "-b", s)
	}
	if s, _ := opts["selector"].(string); s != "" {
		args = append(args, "-s", s)
	}
	if b, _ := opts["compact"].(bool); b {
		args = append(args, "-c")
	}
	if d, ok := opts["depth"]; ok {
		args = append(args, "-d", fmt.Sprintf("%v", numToInt(d)))
	}
	return args
}

// diffScreenshotArgs builds `diff screenshot --baseline <file> [-o out] [-t thr]`.
func diffScreenshotArgs(opts map[string]any) []string {
	args := []string{"diff", "screenshot", "--baseline", fmt.Sprintf("%v", opts["baseline"])}
	if s, _ := opts["output"].(string); s != "" {
		args = append(args, "-o", s)
	}
	if t, ok := opts["threshold"]; ok {
		args = append(args, "-t", fmt.Sprintf("%v", t))
	}
	return args
}

func (h *abHandle) diffSnapshot(ctx context.Context, opts map[string]any) (any, error) {
	return h.runJSON(ctx, diffSnapshotArgs(opts)...)
}

func (h *abHandle) diffScreenshot(ctx context.Context, opts map[string]any) (any, error) {
	if s, _ := opts["baseline"].(string); s == "" {
		return nil, errors.New("agentBrowser.diff.screenshot: opts.baseline (a baseline image path) is required")
	}
	return h.runJSON(ctx, diffScreenshotArgs(opts)...)
}

// diffURLArgs carries the two URLs to compare.
type diffURLArgs struct {
	u1, u2 string
}

func diffURLExtract(call goja.FunctionCall) (diffURLArgs, error) {
	return diffURLArgs{u1: strArg(call, 0), u2: strArg(call, 1)}, nil
}

func (h *abHandle) diffURL(ctx context.Context, a diffURLArgs) (any, error) {
	if a.u1 == "" || a.u2 == "" {
		return nil, errors.New("agentBrowser.diff.url: two URLs are required")
	}
	return h.runJSON(ctx, "diff", "url", a.u1, a.u2)
}

// addDiff wires page diffing into the handle object.
func (h *abHandle) addDiff(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["diff"] = map[string]any{
		"snapshot":   abAsync(vm, loop, abOptsArg0, h.diffSnapshot),
		"screenshot": abAsync(vm, loop, abOptsArg0, h.diffScreenshot),
		"url":        abAsync(vm, loop, diffURLExtract, h.diffURL),
	}
}
