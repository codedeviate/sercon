package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// findExtra holds fs.find options that are not part of the shared walkOptions.
type findExtra struct {
	nameRE   *regexp.Regexp
	fullPath bool
	absolute bool
	limit    int
	sortOut  bool
	stat     bool
	stream   bool
}

// stringOrSlice reads a key that may be a string or string[] into a []string.
func stringOrSlice(opts map[string]any, key string) []string {
	if v, ok := opts[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return []string{s}
		}
		if arr, ok := v.([]any); ok {
			var out []string
			for _, it := range arr {
				if s, ok := it.(string); ok && s != "" {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

// setOf lowercases a string/string[] key into a set (no leading dots for exts).
func setOf(opts map[string]any, key string, trimDot bool) map[string]bool {
	vals := stringOrSlice(opts, key)
	if len(vals) == 0 {
		return nil
	}
	m := map[string]bool{}
	for _, v := range vals {
		v = strings.ToLower(v)
		if trimDot {
			v = strings.TrimPrefix(v, ".")
		}
		m[v] = true
	}
	return m
}

// caseInsensitive resolves the case option ("smart"|"sensitive"|"insensitive")
// against a pattern for smart-case (insensitive unless pattern has uppercase).
func caseInsensitive(mode, pattern string) bool {
	switch mode {
	case "insensitive":
		return true
	case "sensitive":
		return false
	default: // smart
		return pattern == strings.ToLower(pattern)
	}
}

// parseWalkOptions builds the shared walkOptions from a raw opts map.
func parseWalkOptions(opts map[string]any, who string) (walkOptions, error) {
	o := walkOptions{
		globs:       stringOrSlice(opts, "glob"),
		excludes:    stringOrSlice(opts, "exclude"),
		types:       setOf(opts, "type", false),
		exts:        setOf(opts, "extension", true),
		hidden:      optBool(opts, "hidden", false),
		gitignore:   optBool(opts, "gitignore", true),
		followLinks: optBool(opts, "followSymlinks", false),
		maxDepth:    optInt(opts, "maxDepth", 0),
		minDepth:    optInt(opts, "minDepth", 0),
		strict:      optBool(opts, "strict", false),
	}
	roots := stringOrSlice(opts, "root")
	if len(roots) == 0 {
		roots = []string{"."}
	}
	o.roots = roots
	return o, nil
}

func parseFindExtra(opts map[string]any) (findExtra, error) {
	fe := findExtra{
		fullPath: optBool(opts, "fullPath", false),
		absolute: optBool(opts, "absolute", false),
		limit:    optInt(opts, "limit", 0),
		sortOut:  optBool(opts, "sort", false),
		stat:     optBool(opts, "stat", false),
		stream:   optBool(opts, "stream", false),
	}
	if rx := optString(opts, "regex", ""); rx != "" {
		mode := optString(opts, "case", "smart")
		expr := rx
		if caseInsensitive(mode, rx) {
			expr = "(?i)" + rx
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return fe, fmt.Errorf("fs.find: invalid regex: %w", err)
		}
		fe.nameRE = re
	}
	return fe, nil
}

// fsFindArgs is the plain-Go bundle for the find work goroutine. (fs-prefixed:
// webdriver_element.go owns the bare findArgs/findExtract names.)
type fsFindArgs struct {
	walk  walkOptions
	extra findExtra
}

func fsFindExtract(call goja.FunctionCall) (fsFindArgs, error) {
	opts := optsArgMap(call, 0)
	w, err := parseWalkOptions(opts, "find")
	if err != nil {
		return fsFindArgs{}, err
	}
	fe, err := parseFindExtra(opts)
	if err != nil {
		return fsFindArgs{}, err
	}
	return fsFindArgs{walk: w, extra: fe}, nil
}

// runFind walks and returns either []string (paths) or []map (stat entries).
func runFind(ctx context.Context, a fsFindArgs) ([]any, error) {
	var out []any
	err := fsSearchWalk(ctx, a.walk, func(e walkEntry) error {
		if a.extra.nameRE != nil {
			target := e.name
			if a.extra.fullPath {
				target = e.rel
			}
			if !a.extra.nameRE.MatchString(target) {
				return nil
			}
		}
		disp := relDisplay(e.abs, a.extra.absolute)
		if a.extra.stat {
			info, err := os.Lstat(e.abs)
			if err != nil {
				if a.walk.strict {
					return err
				}
				return nil
			}
			out = append(out, map[string]any{
				"path": disp, "type": e.typ,
				"size": info.Size(), "mtimeMs": info.ModTime().UnixMilli(),
			})
		} else {
			out = append(out, disp)
		}
		if a.extra.limit > 0 && len(out) >= a.extra.limit {
			return errStopWalk
		}
		return nil
	})
	if err != nil && err != errStopWalk {
		return nil, fmt.Errorf("fs.find: %w", err)
	}
	if a.extra.sortOut {
		sortFindResults(out, a.extra.stat)
	}
	return out, nil
}

// errStopWalk is an internal sentinel used to stop the walk early (limit hit).
var errStopWalk = fmt.Errorf("fs: stop walk")

func sortFindResults(out []any, stat bool) {
	sort.Slice(out, func(i, j int) bool {
		return findKey(out[i], stat) < findKey(out[j], stat)
	})
}

func findKey(v any, stat bool) string {
	if stat {
		m, _ := v.(map[string]any)
		s, _ := m["path"].(string)
		return s
	}
	s, _ := v.(string)
	return s
}

// fsFindBinding returns the goja-callable for fs.find. It dispatches to the
// streaming iterator when opts.stream is set (Task 5), else PromisifyAsync.
func fsFindBinding(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	async := scriptengine.PromisifyAsync(vm, loop, fsFindExtract, runFind)
	return func(call goja.FunctionCall) goja.Value {
		if optBool(optsArgMap(call, 0), "stream", false) {
			args, err := fsFindExtract(call) // on loop — safe (goja access stays here)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return fsSearchStream(vm, loop, eng, func(ctx context.Context, out chan<- any) error {
				return streamFind(ctx, args, out)
			})
		}
		return async.Func(call)
	}
}
