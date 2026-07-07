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
		"writeText":  scriptengine.PromisifyAsync(vm, loop, fileWriteTextExtract, fileWriteText),
		"writeBytes": scriptengine.PromisifyAsync(vm, loop, fileWriteBytesExtract, fileWriteBytes),
		"readText":   scriptengine.PromisifyAsync(vm, loop, filePathExtract("readText"), fileReadText),
		"readBytes":  scriptengine.PromisifyAsync(vm, loop, filePathExtract("readBytes"), fileReadBytes),
		"mkdir":      scriptengine.PromisifyAsync(vm, loop, filePathExtract("mkdir"), fileMkdir),
		"exists":     scriptengine.PromisifyAsync(vm, loop, filePathExtract("exists"), fileExists),
		"remove":     scriptengine.PromisifyAsync(vm, loop, filePathExtract("remove"), fileRemove),
		"stat":       scriptengine.PromisifyAsync(vm, loop, filePathExtract("stat"), fileStat),
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

// filePathExtract returns an extract func for the path-only bindings
// (readText, readBytes, mkdir, exists, remove, stat): argument 0 is a
// required, non-empty path string. who names the binding for error messages.
func filePathExtract(who string) func(goja.FunctionCall) (string, error) {
	return func(call goja.FunctionCall) (string, error) {
		return filePathArg(call, 0, who)
	}
}

// fileWriteTextArgs carries the on-loop-extracted arguments of fs.writeText.
type fileWriteTextArgs struct {
	path string
	data []byte
}

func fileWriteTextExtract(call goja.FunctionCall) (fileWriteTextArgs, error) {
	path, err := filePathArg(call, 0, "writeText")
	if err != nil {
		return fileWriteTextArgs{}, err
	}
	tv := call.Argument(1)
	if tv == nil || goja.IsUndefined(tv) || goja.IsNull(tv) {
		return fileWriteTextArgs{}, errors.New("fs.writeText: text must be a string")
	}
	return fileWriteTextArgs{path: path, data: []byte(tv.String())}, nil
}

func fileWriteText(_ context.Context, args fileWriteTextArgs) (map[string]any, error) {
	if err := os.WriteFile(args.path, args.data, 0o644); err != nil { //nolint:gosec // scripts choose the path
		return nil, fmt.Errorf("fs.writeText: %w", err)
	}
	return map[string]any{"path": args.path, "bytes": len(args.data)}, nil
}

// fileWriteBytesArgs carries the on-loop-extracted arguments of fs.writeBytes.
type fileWriteBytesArgs struct {
	path string
	data []byte
}

func fileWriteBytesExtract(call goja.FunctionCall) (fileWriteBytesArgs, error) {
	path, err := filePathArg(call, 0, "writeBytes")
	if err != nil {
		return fileWriteBytesArgs{}, err
	}
	data, ok := call.Argument(1).Export().([]byte)
	if !ok {
		return fileWriteBytesArgs{}, errors.New("fs.writeBytes: data must be a Uint8Array")
	}
	return fileWriteBytesArgs{path: path, data: data}, nil
}

func fileWriteBytes(_ context.Context, args fileWriteBytesArgs) (map[string]any, error) {
	if err := os.WriteFile(args.path, args.data, 0o644); err != nil { //nolint:gosec // scripts choose the path
		return nil, fmt.Errorf("fs.writeBytes: %w", err)
	}
	return map[string]any{"path": args.path, "bytes": len(args.data)}, nil
}

func fileReadText(_ context.Context, path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // scripts choose the path
	if err != nil {
		return "", fmt.Errorf("fs.readText: %w", err)
	}
	return string(data), nil
}

func fileReadBytes(_ context.Context, path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec // scripts choose the path
	if err != nil {
		return nil, fmt.Errorf("fs.readBytes: %w", err)
	}
	return data, nil
}

func fileMkdir(_ context.Context, path string) (map[string]any, error) {
	if err := os.MkdirAll(path, 0o755); err != nil { //nolint:gosec // scripts choose the path
		return nil, fmt.Errorf("fs.mkdir: %w", err)
	}
	return map[string]any{"path": path}, nil
}

func fileExists(_ context.Context, path string) (bool, error) {
	_, serr := os.Stat(path)
	if serr == nil {
		return true, nil
	}
	if errors.Is(serr, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("fs.exists: %w", serr)
}

func fileRemove(_ context.Context, path string) (map[string]any, error) {
	if err := os.RemoveAll(path); err != nil {
		return nil, fmt.Errorf("fs.remove: %w", err)
	}
	return map[string]any{"path": path}, nil
}

func fileStat(_ context.Context, path string) (map[string]any, error) {
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
