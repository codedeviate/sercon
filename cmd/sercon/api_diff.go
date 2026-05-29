package main

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/pmezard/go-difflib/difflib"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// diffNamespace wires `text.diff.*`. Only one member for now (`compare`);
// the namespace exists so future additions (word-level / char-level
// diffs, patch application) land in a stable location.
func diffNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"compare": scriptengine.PromisifyAsync(vm, loop, diffCompare),
	}
}

// diffCompare produces a unified diff between two inputs. Inputs are
// either strings (treated as UTF-8) or any goja byte sequence
// (ArrayBuffer / Uint8Array). Binary inputs short-circuit with an empty
// `diff` and `binary: true` — printing a unified diff of binary content
// isn't useful.
//
// Result shape:
//
//	{
//	  identical: boolean,   // a and b are byte-equal
//	  binary:    boolean,   // either input has a NUL byte in its first 8 KB
//	  added:     number,    // '+' lines, excluding the file header
//	  removed:   number,    // '-' lines, excluding the file header
//	  diff:      string,    // unified diff text; empty when identical / binary
//	  format:    "unified",
//	}
func diffCompare(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	aBytes, err := exportBytes(call.Argument(0))
	if err != nil {
		return nil, fmt.Errorf("diff.compare: argument 0: %w", err)
	}
	bBytes, err := exportBytes(call.Argument(1))
	if err != nil {
		return nil, fmt.Errorf("diff.compare: argument 1: %w", err)
	}

	// optsAsMap is hardcoded to position 1 (the typical
	// `func(target, opts)` shape); diff.compare's opts sit at position 2
	// instead.
	var opts map[string]any
	if optsArg := call.Argument(2); optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
		if m, ok := optsArg.Export().(map[string]any); ok {
			opts = m
		}
	}
	contextLines := optInt(opts, "context", 3)
	fromFile := optString(opts, "fromFile", "a")
	toFile := optString(opts, "toFile", "b")

	identical := bytes.Equal(aBytes, bBytes)
	binary := looksBinary(aBytes) || looksBinary(bBytes)

	out := map[string]any{
		"identical": identical,
		"binary":    binary,
		"added":     0,
		"removed":   0,
		"diff":      "",
		"format":    "unified",
	}
	if identical || binary {
		return out, nil
	}

	udiff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(aBytes)),
		B:        difflib.SplitLines(string(bBytes)),
		FromFile: fromFile,
		ToFile:   toFile,
		Context:  contextLines,
	}
	diffText, err := difflib.GetUnifiedDiffString(udiff)
	if err != nil {
		return nil, fmt.Errorf("diff.compare: %w", err)
	}

	added, removed := countAddedRemoved(diffText)
	out["diff"] = diffText
	out["added"] = added
	out["removed"] = removed
	return out, nil
}

// looksBinary inspects the first 8 KB of the input for NUL bytes. NUL is
// the cheapest reliable signal: text formats reserve byte 0x00 (or
// effectively never use it), while binary formats almost always have it
// in their headers or padding. We don't try to be cleverer because the
// goal is just to skip pretty-printing in the obviously-wrong case.
func looksBinary(b []byte) bool {
	limit := len(b)
	if limit > 8192 {
		limit = 8192
	}
	for i := 0; i < limit; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}

// countAddedRemoved walks the unified-diff text counting body-only `+`/`-`
// lines. The file headers (`+++ b`, `--- a`) get excluded so the counts
// match what tools like `git diff --shortstat` report.
func countAddedRemoved(diff string) (int, int) {
	var added, removed int
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"):
			// file header, skip
		case strings.HasPrefix(line, "---"):
			// file header, skip
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}
