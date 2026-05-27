package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// watchableExt is the set of file extensions whose changes trigger a
// re-run. Anything outside this set is ignored — saving a stray
// screenshot or hidden editor swap file inside ScriptRoot shouldn't
// re-run the entry script. The dot is part of the entry; matching
// uses filepath.Ext which always returns ".ext" with a leading dot.
var watchableExt = map[string]bool{
	".ts":   true,
	".tsx":  true,
	".js":   true,
	".jsx":  true,
	".json": true,
	".d.ts": true, // filepath.Ext returns ".ts" for foo.d.ts but check the suffix below
}

// watchDebounce is the window we wait after the first file event
// before re-running. Editors and CI sync tools often fire several
// events in rapid succession (write → rename → chmod); collapsing
// them into one re-run keeps the loop responsive without thrashing.
// 150ms is the sweet spot in practice — short enough to feel
// interactive, long enough to absorb the typical save burst.
const watchDebounce = 150 * time.Millisecond

// runWatchLoop is the long-running entry point for `sercon --watch`.
// It does the initial run synchronously, then registers a recursive
// fsnotify watcher on ScriptRoot, and re-runs the entry scripts each
// time a `.ts` / `.tsx` / `.js` / `.jsx` / `.json` change is
// detected. SIGINT (Ctrl-C) and SIGTERM exit cleanly.
//
// The scriptengine doesn't need any teardown between runs — every
// Run already gets a fresh *goja.Runtime, so re-running is just
// re-invoking runOne on the same Engine. Re-registering bindings
// would double them; we keep the engine and let it serve repeated
// runs.
func runWatchLoop(eng *scriptengine.Engine, scripts []string, scriptRoot string, verbose bool, out io.Writer, userArgs []string) int {
	if scriptRoot == "" {
		fmt.Fprintln(os.Stderr, "sercon: --watch needs a script root")
		return exitUsage
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sercon: --watch:", err)
		return exitUsage
	}
	defer func() { _ = watcher.Close() }()

	added, err := addRecursive(watcher, scriptRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sercon: --watch:", err)
		return exitUsage
	}
	fmt.Fprintf(out, "sercon: --watch  root=%s  dirs=%d  Ctrl-C to exit\n", scriptRoot, added)

	// graphs maps each entry script to the set of absolute file paths it
	// resolved (its own file + every module it imported, transitively).
	// Built during each run via the engine's resolve hook; a file change
	// then re-runs only the entries whose graph includes it.
	graphs := map[string]map[string]bool{}

	// Initial run. Always happens regardless of what the watcher
	// later picks up so the user sees output immediately rather
	// than after the first edit. Builds the initial import graphs.
	runOnceForWatch(eng, scripts, graphs, verbose, out, "initial run", userArgs)

	// changed accumulates the absolute paths touched during a debounce
	// window, so the re-run can scope itself to the affected entries.
	changed := map[string]bool{}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var debounceTimer *time.Timer
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()
	// rerunReady fires once per debounce window — guarded by the
	// timer below — and triggers a re-run from the main loop. Buffered
	// so a stale tick from a timer we already stopped doesn't deadlock.
	rerunReady := make(chan struct{}, 1)

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(out, "\nsercon: --watch  exiting")
			return exitOK
		case ev, ok := <-watcher.Events:
			if !ok {
				return exitOK
			}
			// Newly-created directories must be picked up so files
			// underneath them are watched too. Editors sometimes
			// drop temp dirs (atomic-save staging) we don't want to
			// recurse into — the filter below handles the typical
			// dotfile patterns.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					if shouldWatchDir(ev.Name) {
						if _, err := addRecursive(watcher, ev.Name); err == nil {
							// Adding a directory doesn't itself merit a
							// re-run unless a watchable file appears in it.
							continue
						}
					}
					continue
				}
			}
			if !isWatchableFile(ev.Name) {
				continue
			}
			if abs, err := filepath.Abs(ev.Name); err == nil {
				changed[abs] = true
			}
			if debounceTimer == nil {
				debounceTimer = time.AfterFunc(watchDebounce, func() {
					select {
					case rerunReady <- struct{}{}:
					default:
					}
				})
			} else {
				debounceTimer.Reset(watchDebounce)
			}
		case <-rerunReady:
			// debounce window elapsed since the last event — re-run only
			// the entries whose import graph includes a changed file.
			// The timer has already fired; clearing the pointer lets the
			// next event start a fresh window.
			debounceTimer = nil
			affected := affectedEntries(scripts, graphs, changed)
			changed = map[string]bool{}
			if len(affected) == 0 {
				continue // nothing depends on the changed files
			}
			// Bust the module cache so edited imports are re-read; the
			// registry otherwise caches compiled bytecode across runs.
			eng.ResetModuleCache()
			fmt.Fprintln(out, "")
			runOnceForWatch(eng, affected, graphs, verbose, out, "re-run", userArgs)
		case err, ok := <-watcher.Errors:
			if !ok {
				return exitOK
			}
			fmt.Fprintln(os.Stderr, "sercon: --watch error:", err)
		}
	}
}

// runOnceForWatch runs the given entries, rebuilding each one's import
// graph as it goes (via the engine's resolve hook), then prints a
// separator. The `reason` label distinguishes the initial run from
// re-runs in the banner.
func runOnceForWatch(eng *scriptengine.Engine, scripts []string, graphs map[string]map[string]bool, verbose bool, out io.Writer, reason string, userArgs []string) {
	fmt.Fprintf(out, "--- sercon %s @ %s ---\n", reason, time.Now().Format("15:04:05"))
	for _, s := range scripts {
		// Capture the dep set this run resolves. The entry's own file is
		// seeded in so a change to it re-runs the entry too (the entry
		// itself isn't reported through the require hook).
		deps := map[string]bool{}
		if s != "-" {
			if abs, err := filepath.Abs(s); err == nil {
				deps[abs] = true
			}
		}
		eng.SetResolveHook(func(abs string) { deps[abs] = true })
		err := runOne(eng, s, verbose, userArgs)
		eng.SetResolveHook(nil)
		graphs[s] = deps

		if err != nil {
			label := s
			if s == "-" {
				label = "<stdin>"
			}
			fmt.Fprintf(out, "FAIL %s: %s\n", label, err)
		}
	}
}

// affectedEntries returns the entries that should re-run given the set
// of changed absolute paths. An entry re-runs when: it reads from stdin
// (can't be graphed, so always conservatively re-run), its graph is
// unknown (e.g. the initial run failed before recording it), or its
// import graph includes a changed file.
func affectedEntries(scripts []string, graphs map[string]map[string]bool, changed map[string]bool) []string {
	var out []string
	for _, s := range scripts {
		if s == "-" {
			out = append(out, s)
			continue
		}
		graph, ok := graphs[s]
		if !ok {
			out = append(out, s)
			continue
		}
		for path := range changed {
			if graph[path] {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// isWatchableFile reports whether `path` is a source file whose
// change should re-trigger sercon. Filtering at the binding layer
// (not inside the watcher) keeps editor swap files and screenshot
// drops in ScriptRoot from flooding the loop.
//
// `.d.ts` files are matched via suffix because filepath.Ext returns
// ".ts" for them; we want to catch them so a regenerated declaration
// file (from `make types` or an editor's auto-export) re-runs scripts
// that reference it.
func isWatchableFile(path string) bool {
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".d.ts") {
		return true
	}
	ext := filepath.Ext(base)
	return watchableExt[ext]
}

// shouldWatchDir filters directories that newly appear during the
// watch session. Hidden directories (`.git`, `.vscode`,
// `node_modules` if it appears) are excluded — they generate a
// LOT of events for changes that shouldn't trigger sercon, and
// recursing into them at startup would inflate the watcher's
// dir count for no gain.
func shouldWatchDir(path string) bool {
	base := filepath.Base(path)
	if strings.HasPrefix(base, ".") {
		return false
	}
	if base == "node_modules" {
		return false
	}
	return true
}

// addRecursive walks `root` and registers every non-hidden,
// non-node_modules directory with the watcher. Returns the number
// of directories registered. Symlinks aren't followed — they're a
// classic source of watch loops when a script directory links back
// up to its parent.
func addRecursive(watcher *fsnotify.Watcher, root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Permissions on individual sub-dirs shouldn't kill the
			// whole walk — log to stderr but continue.
			if errors.Is(err, fs.ErrPermission) {
				fmt.Fprintln(os.Stderr, "sercon: --watch  skip", path, "("+err.Error()+")")
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		// Symlinks-as-dirs would surface here too; WalkDir doesn't
		// follow them by default, but the entry itself is still
		// reported. Skipping anything that doesn't pass shouldWatchDir
		// covers both filtering needs.
		if path != root && !shouldWatchDir(path) {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("watch %s: %w", path, err)
		}
		count++
		return nil
	})
	return count, err
}
