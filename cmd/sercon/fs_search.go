package main

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/charlievieth/fastwalk"
	gitignore "github.com/monochromegane/go-gitignore"
)

// walkOptions is the parsed, plain-Go traversal request shared by find and grep.
type walkOptions struct {
	roots       []string        // absolute; default [cwd]
	globs       []string        // include globs (doublestar, slash paths); empty = all
	excludes    []string        // exclude globs
	types       map[string]bool // "file"/"dir"/"symlink"; empty = any
	exts        map[string]bool // lowercase, no leading dot; empty = any
	hidden      bool            // include dotfiles/dirs
	gitignore   bool            // honor .gitignore and .ignore files
	followLinks bool
	maxDepth    int // 0 = unlimited
	minDepth    int
	strict      bool // true = return first traversal error; false = skip
}

// walkEntry is one filesystem entry passed to the walk callback. Consumers
// needing os.FileInfo (e.g. `stat: true` mode) call os.Lstat(e.abs)
// themselves rather than have fsSearchWalk stat every entry unconditionally.
type walkEntry struct {
	abs   string // absolute path
	rel   string // path relative to cwd (slash-normalized for display via relDisplay)
	name  string // basename
	typ   string // "file" | "dir" | "symlink"
	depth int    // depth below the root (root children = 1)
}

// ignoreStack lazily loads and caches the .gitignore / .ignore matchers for a
// directory and tests an entry against every matcher from a search root down to
// the entry's own directory. Safe for fastwalk's concurrent callbacks.
type ignoreStack struct {
	root string
	mu   sync.Mutex
	// cache maps an absolute directory to its own matchers (nil = none / loaded).
	cache map[string][]gitignore.IgnoreMatcher
}

func newIgnoreStack(root string) *ignoreStack {
	return &ignoreStack{root: root, cache: map[string][]gitignore.IgnoreMatcher{}}
}

// matchersFor returns (and caches) the ignore matchers declared directly in dir.
func (s *ignoreStack) matchersFor(dir string) []gitignore.IgnoreMatcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.cache[dir]; ok {
		return m
	}
	var ms []gitignore.IgnoreMatcher
	for _, name := range []string{".gitignore", ".ignore"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		// base=dir: gitignore.Match(path, isDir) computes path relative to
		// dir internally (filepath.Rel(g.path, path)), so the path passed to
		// Match must share the same root as dir (i.e. be absolute, like dir).
		if gi, err := gitignore.NewGitIgnore(p, dir); err == nil {
			ms = append(ms, gi)
		}
	}
	s.cache[dir] = ms
	return ms
}

// ignored reports whether abs (a file or dir under root) is ignored by any
// .gitignore/.ignore from root down to abs's parent directory.
func (s *ignoreStack) ignored(abs string, isDir bool) bool {
	dir := filepath.Dir(abs)
	// Walk ancestor dirs from the entry's parent up to (and including) root.
	for d := dir; ; d = filepath.Dir(d) {
		for _, m := range s.matchersFor(d) {
			if m.Match(abs, isDir) {
				return true
			}
		}
		if d == s.root || !strings.HasPrefix(d, s.root) {
			break
		}
	}
	return false
}

// matchGlobs reports whether rel (slash path) matches any include glob. An empty
// list matches everything.
func matchGlobs(rel string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	for _, g := range globs {
		if ok, _ := doublestar.Match(g, rel); ok {
			return true
		}
	}
	return false
}

// relDisplay converts an absolute path to a cwd-relative slash path (or returns
// abs unchanged when absolute output is requested).
func relDisplay(abs string, absolute bool) string {
	if absolute {
		return abs
	}
	if cwd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(cwd, abs); err == nil {
			return filepath.ToSlash(r)
		}
	}
	return filepath.ToSlash(abs)
}

func entryType(d fs.DirEntry, isDir bool) string {
	switch {
	case d.Type()&os.ModeSymlink != 0:
		return "symlink"
	case isDir:
		return "dir"
	default:
		return "file"
	}
}

// fsSearchWalk walks roots with fastwalk, prunes hidden/ignored dirs, applies
// type/ext/glob/depth filters, and calls fn for each surviving NON-pruned entry.
// fn returning a non-nil error stops the walk with that error. Honors ctx.
func fsSearchWalk(ctx context.Context, o walkOptions, fn func(walkEntry) error) error {
	conf := &fastwalk.Config{Follow: o.followLinks}
	// fastwalk calls its walkFn concurrently from multiple goroutines (see
	// fastwalk.Walk docs); fn is typically a simple, non-thread-safe
	// accumulator (append to a slice, write to a channel expecting order,
	// etc.), so serialize calls to it here rather than pushing that burden
	// onto every caller.
	var fnMu sync.Mutex
	for _, root := range o.roots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			if o.strict {
				return err
			}
			continue
		}
		ign := newIgnoreStack(rootAbs)
		walkErr := fastwalk.Walk(conf, rootAbs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if o.strict {
					return err
				}
				return nil // skip unreadable entry
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if path == rootAbs {
				return nil // don't emit the root itself
			}
			name := d.Name()
			isDir := d.IsDir()

			// Depth relative to root (root children = depth 1).
			rel, _ := filepath.Rel(rootAbs, path)
			depth := len(strings.Split(filepath.ToSlash(rel), "/"))

			// Hidden: dotfile/dir. Prune hidden dirs entirely.
			if !o.hidden && strings.HasPrefix(name, ".") {
				if isDir {
					return filepath.SkipDir
				}
				return nil
			}
			// gitignore: prune ignored dirs, skip ignored files.
			if o.gitignore && ign.ignored(path, isDir) {
				if isDir {
					return filepath.SkipDir
				}
				return nil
			}
			// maxDepth: stop descending past the limit.
			if o.maxDepth > 0 && depth >= o.maxDepth && isDir {
				return filepath.SkipDir
			}

			if isDir {
				// Directories still flow to fn only when the type filter wants them.
				if !typeWanted(o.types, "dir") {
					return nil
				}
			}
			relSlash := filepath.ToSlash(rel)
			typ := entryType(d, isDir)

			// Emit-level filters (do not affect descent).
			if o.minDepth > 0 && depth < o.minDepth {
				return nil
			}
			if !typeWanted(o.types, typ) {
				return nil
			}
			if len(o.exts) > 0 {
				ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
				if !o.exts[ext] {
					return nil
				}
			}
			if !matchGlobs(relSlash, o.globs) {
				return nil
			}
			if len(o.excludes) > 0 && matchGlobs(relSlash, o.excludes) {
				return nil
			}
			fnMu.Lock()
			defer fnMu.Unlock()
			return fn(walkEntry{abs: path, rel: relSlash, name: name, typ: typ, depth: depth})
		})
		if walkErr != nil {
			return walkErr
		}
	}
	return nil
}

// typeWanted reports whether typ passes the type filter (empty filter = any).
func typeWanted(types map[string]bool, typ string) bool {
	if len(types) == 0 {
		return true
	}
	return types[typ]
}
