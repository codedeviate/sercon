package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// grepTypePresets maps rg-style type names to extension sets.
var grepTypePresets = map[string][]string{
	"ts":   {"ts", "tsx"},
	"js":   {"js", "jsx", "mjs", "cjs"},
	"go":   {"go"},
	"md":   {"md", "markdown"},
	"json": {"json"},
	"py":   {"py"},
	"rs":   {"rs"},
	"c":    {"c", "h"},
}

// grepExtra holds grep-only options beyond walkOptions.
type grepExtra struct {
	matcher    *grepMatcher
	before     int
	after      int
	maxMatches int
	maxResults int
	includeBin bool
	absolute   bool
	sortOut    bool
	stream     bool
}

type grepArgs struct {
	walk  walkOptions
	paths []string // explicit files/dirs (overrides walk roots when set)
	extra grepExtra
}

func fsGrepExtract(call goja.FunctionCall) (grepArgs, error) {
	opts := optsArgMap(call, 0)
	pattern := optString(opts, "pattern", "")
	if pattern == "" {
		return grepArgs{}, fmt.Errorf("fs.grep: pattern is required")
	}
	w, err := parseWalkOptions(opts, "grep")
	if err != nil {
		return grepArgs{}, err
	}
	// grep's `type` is rg-style (ts/js/go/…), an extension preset — NOT
	// fs.find's file/dir/symlink KIND filter. parseWalkOptions parsed it into
	// w.types with kind semantics, which would make typeWanted() reject every
	// "file" entry. Discard it: grep only ever wants files (grepEachFile
	// already hard-filters e.typ != "file"), and the real preset is merged
	// into w.exts just below.
	w.types = nil
	// Merge type presets into the extension filter.
	for _, tn := range stringOrSlice(opts, "type") {
		if exts, ok := grepTypePresets[tn]; ok {
			if w.exts == nil {
				w.exts = map[string]bool{}
			}
			for _, e := range exts {
				w.exts[e] = true
			}
		}
	}
	ic := caseInsensitive(optString(opts, "case", "smart"), pattern)
	m, err := compileGrepMatcher(pattern,
		optBool(opts, "fixed", false),
		optBool(opts, "word", false),
		ic,
		optBool(opts, "multiline", false))
	if err != nil {
		return grepArgs{}, fmt.Errorf("fs.grep: %w", err)
	}
	m.invert = optBool(opts, "invert", false)
	ctxLines := optInt(opts, "context", 0)
	fe := grepExtra{
		matcher:    m,
		before:     firstNonZero(optInt(opts, "before", 0), ctxLines),
		after:      firstNonZero(optInt(opts, "after", 0), ctxLines),
		maxMatches: optInt(opts, "maxMatches", 0),
		maxResults: optInt(opts, "maxResults", 0),
		includeBin: optBool(opts, "includeBinary", false),
		absolute:   optBool(opts, "absolute", false),
		sortOut:    optBool(opts, "sort", false),
		stream:     optBool(opts, "stream", false),
	}
	return grepArgs{walk: w, paths: stringOrSlice(opts, "paths"), extra: fe}, nil
}

func firstNonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

// grepEachFile walks (or iterates explicit paths) and calls fn per matching file
// with that file's matches and total count. Honors ctx + maxResults.
func grepEachFile(ctx context.Context, a grepArgs, fn func(matches []grepMatch, count int) error) error {
	total := 0
	handle := func(abs, disp string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		data, err := os.ReadFile(abs) //nolint:gosec // scripts choose the root
		if err != nil {
			if a.walk.strict {
				return err
			}
			return nil
		}
		if !a.extra.includeBin && isBinary(data) {
			return nil
		}
		matches, count := grepFile(disp, data, a.extra.matcher, a.extra.before, a.extra.after, a.extra.maxMatches)
		if count == 0 {
			return nil
		}
		total += len(matches)
		if err := fn(matches, count); err != nil {
			return err
		}
		if a.extra.maxResults > 0 && total >= a.extra.maxResults {
			return errStopWalk
		}
		return nil
	}
	if len(a.paths) > 0 {
		for _, p := range a.paths {
			abs, _ := absPath(p)
			if err := handle(abs, relDisplay(abs, a.extra.absolute)); err != nil {
				if err == errStopWalk {
					return nil
				}
				return err
			}
		}
		return nil
	}
	err := fsSearchWalk(ctx, a.walk, func(e walkEntry) error {
		if e.typ != "file" {
			return nil
		}
		return handle(e.abs, relDisplay(e.abs, a.extra.absolute))
	})
	if err == errStopWalk {
		return nil
	}
	return err
}

// absPath resolves p to an absolute path for read + display consistency,
// falling back to the original (relative) input if resolution fails.
func absPath(p string) (string, error) {
	if abs, err := filepath.Abs(p); err == nil {
		return abs, nil
	}
	return p, nil
}

func runGrep(ctx context.Context, a grepArgs) ([]any, error) {
	var out []any
	err := grepEachFile(ctx, a, func(matches []grepMatch, _ int) error {
		for _, m := range matches {
			out = append(out, grepMatchToMap(m))
			if a.extra.maxResults > 0 && len(out) >= a.extra.maxResults {
				return errStopWalk
			}
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return nil, fmt.Errorf("fs.grep: %w", err)
	}
	if a.extra.sortOut {
		sort.Slice(out, func(i, j int) bool { return grepLess(out[i], out[j]) })
	}
	return out, nil
}

func runGrepFiles(ctx context.Context, a grepArgs) ([]any, error) {
	// grepFiles: stop at first hit per file (perf). Cap per-file matches to 1.
	a.extra.maxMatches = 1
	var out []any
	seen := map[string]bool{}
	err := grepEachFile(ctx, a, func(matches []grepMatch, _ int) error {
		if len(matches) > 0 && !seen[matches[0].Path] {
			seen[matches[0].Path] = true
			out = append(out, matches[0].Path)
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return nil, fmt.Errorf("fs.grepFiles: %w", err)
	}
	if a.extra.sortOut {
		sort.Slice(out, func(i, j int) bool {
			si, _ := out[i].(string)
			sj, _ := out[j].(string)
			return si < sj
		})
	}
	return out, nil
}

func runGrepCount(ctx context.Context, a grepArgs) ([]any, error) {
	a.extra.maxMatches = 0 // count everything
	var out []any
	err := grepEachFile(ctx, a, func(matches []grepMatch, count int) error {
		if count > 0 {
			out = append(out, map[string]any{"path": matches[0].Path, "count": count})
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return nil, fmt.Errorf("fs.grepCount: %w", err)
	}
	return out, nil
}

func grepMatchToMap(m grepMatch) map[string]any {
	r := map[string]any{
		"path": m.Path, "line": m.Line, "column": m.Column,
		"match": m.Match, "text": m.Text,
	}
	if len(m.Before) > 0 {
		r["before"] = m.Before
	}
	if len(m.After) > 0 {
		r["after"] = m.After
	}
	return r
}

func grepLess(a, b any) bool {
	ma, _ := a.(map[string]any)
	mb, _ := b.(map[string]any)
	pa, _ := ma["path"].(string)
	pb, _ := mb["path"].(string)
	if pa != pb {
		return pa < pb
	}
	la, _ := ma["line"].(int)
	lb, _ := mb["line"].(int)
	return la < lb
}

func fsGrepBinding(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	async := scriptengine.PromisifyAsync(vm, loop, fsGrepExtract, runGrep)
	return func(call goja.FunctionCall) goja.Value {
		if optBool(optsArgMap(call, 0), "stream", false) {
			args, err := fsGrepExtract(call) // on loop — safe (goja access stays here)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return fsSearchStream(vm, loop, eng, func(ctx context.Context, out chan<- any) error {
				return streamGrep(ctx, args, out)
			})
		}
		return async.Func(call)
	}
}

func fsGrepFilesBinding(vm *goja.Runtime, loop *eventloop.EventLoop) any {
	return scriptengine.PromisifyAsync(vm, loop, fsGrepExtract, runGrepFiles)
}

func fsGrepCountBinding(vm *goja.Runtime, loop *eventloop.EventLoop) any {
	return scriptengine.PromisifyAsync(vm, loop, fsGrepExtract, runGrepCount)
}
