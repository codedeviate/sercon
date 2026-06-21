package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// fileNamespace returns the general file read/write members that are merged
// directly onto the `fs` global (fs.writeText, fs.readBytes, fs.mkdir, …). All
// async, mirroring fs.archive. Paths are used as given (CWD-relative or
// absolute); no sandboxing, consistent with image.save / webdriver screenshot.
// Writes do NOT create parent directories — call fs.mkdir first (Node-like).
func fileNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"writeText":  scriptengine.PromisifyAsync(vm, loop, fileWriteText),
		"writeBytes": scriptengine.PromisifyAsync(vm, loop, fileWriteBytes),
		"readText":   scriptengine.PromisifyAsync(vm, loop, fileReadText),
		"readBytes":  scriptengine.PromisifyAsync(vm, loop, fileReadBytes),
		"mkdir":      scriptengine.PromisifyAsync(vm, loop, fileMkdir),
		"exists":     scriptengine.PromisifyAsync(vm, loop, fileExists),
		"remove":     scriptengine.PromisifyAsync(vm, loop, fileRemove),
		"stat":       scriptengine.PromisifyAsync(vm, loop, fileStat),
	}
}

// filePathArg reads a required, non-empty path string at argument i.
func filePathArg(call goja.FunctionCall, i int, who string) (string, error) {
	v := call.Argument(i)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return "", fmt.Errorf("fs.%s: path is required", who)
	}
	p := v.String()
	if p == "" {
		return "", fmt.Errorf("fs.%s: path is required", who)
	}
	return p, nil
}

func fileWriteText(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	path, err := filePathArg(call, 0, "writeText")
	if err != nil {
		return nil, err
	}
	tv := call.Argument(1)
	if tv == nil || goja.IsUndefined(tv) || goja.IsNull(tv) {
		return nil, errors.New("fs.writeText: text must be a string")
	}
	data := []byte(tv.String())
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // scripts choose the path
		return nil, fmt.Errorf("fs.writeText: %w", err)
	}
	return map[string]any{"path": path, "bytes": len(data)}, nil
}

func fileWriteBytes(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	path, err := filePathArg(call, 0, "writeBytes")
	if err != nil {
		return nil, err
	}
	data, ok := call.Argument(1).Export().([]byte)
	if !ok {
		return nil, errors.New("fs.writeBytes: data must be a Uint8Array")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // scripts choose the path
		return nil, fmt.Errorf("fs.writeBytes: %w", err)
	}
	return map[string]any{"path": path, "bytes": len(data)}, nil
}

func fileReadText(_ context.Context, call goja.FunctionCall) (string, error) {
	path, err := filePathArg(call, 0, "readText")
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path) //nolint:gosec // scripts choose the path
	if err != nil {
		return "", fmt.Errorf("fs.readText: %w", err)
	}
	return string(data), nil
}

func fileReadBytes(_ context.Context, call goja.FunctionCall) ([]byte, error) {
	path, err := filePathArg(call, 0, "readBytes")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path) //nolint:gosec // scripts choose the path
	if err != nil {
		return nil, fmt.Errorf("fs.readBytes: %w", err)
	}
	return data, nil
}

func fileMkdir(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	path, err := filePathArg(call, 0, "mkdir")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(path, 0o755); err != nil { //nolint:gosec // scripts choose the path
		return nil, fmt.Errorf("fs.mkdir: %w", err)
	}
	return map[string]any{"path": path}, nil
}

func fileExists(_ context.Context, call goja.FunctionCall) (bool, error) {
	path, err := filePathArg(call, 0, "exists")
	if err != nil {
		return false, err
	}
	_, serr := os.Stat(path)
	if serr == nil {
		return true, nil
	}
	if errors.Is(serr, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("fs.exists: %w", serr)
}

func fileRemove(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	path, err := filePathArg(call, 0, "remove")
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("fs.remove: %w", err)
	}
	return map[string]any{"path": path}, nil
}

func fileStat(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	path, err := filePathArg(call, 0, "stat")
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("fs.stat: %w", err)
	}
	return map[string]any{
		"size":       info.Size(),
		"isDir":      info.IsDir(),
		"modifiedMs": info.ModTime().UnixMilli(),
	}, nil
}
