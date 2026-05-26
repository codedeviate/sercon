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
func runWatchLoop(eng *scriptengine.Engine, scripts []string, scriptRoot string, verbose bool, out io.Writer) int {
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

	// Initial run. Always happens regardless of what the watcher
	// later picks up so the user sees output immediately rather
	// than after the first edit.
	runOnceForWatch(eng, scripts, verbose, out, "initial run")

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
			// debounce window elapsed since the last event — re-run.
			// The timer has already fired; clearing the pointer lets
			// the next event start a fresh window.
			debounceTimer = nil
			fmt.Fprintln(out, "")
			runOnceForWatch(eng, scripts, verbose, out, "re-run")
		case err, ok := <-watcher.Errors:
			if !ok {
				return exitOK
			}
			fmt.Fprintln(os.Stderr, "sercon: --watch error:", err)
		}
	}
}

// runOnceForWatch is the per-iteration body — does what `main`'s
// non-watch loop would do for one pass, then prints a separator so
// the next iteration's output is visually distinct. The `reason`
// label distinguishes the initial run from re-runs in the banner.
func runOnceForWatch(eng *scriptengine.Engine, scripts []string, verbose bool, out io.Writer, reason string) {
	fmt.Fprintf(out, "--- sercon %s @ %s ---\n", reason, time.Now().Format("15:04:05"))
	for _, s := range scripts {
		if err := runOne(eng, s, verbose); err != nil {
			label := s
			if s == "-" {
				label = "<stdin>"
			}
			fmt.Fprintf(out, "FAIL %s: %s\n", label, err)
		}
	}
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
