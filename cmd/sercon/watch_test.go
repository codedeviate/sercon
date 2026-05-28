package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

// isWatchableFile drives the per-event filter in the watch loop:
// editor swap / lock / image files in ScriptRoot shouldn't trigger
// re-runs, but every script-source extension should.
func TestIsWatchableFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Watchable script sources
		{"main.ts", true},
		{"a/b/foo.ts", true},
		{"component.tsx", true},
		{"helper.js", true},
		{"react.jsx", true},
		{"config.json", true},
		{"package.json", true},
		{"api.d.ts", true}, // double-extension; matched via suffix
		// Non-script files in the project tree
		{"README.md", false},
		{"image.png", false},
		{"binary.bin", false},
		{".main.ts.swp", false}, // vim swap files have no .ts extension after the .swp
		{"main.ts~", false},     // emacs backup
		// No extension
		{"Makefile", false},
		// Empty
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := isWatchableFile(tc.path); got != tc.want {
				t.Errorf("isWatchableFile(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// shouldWatchDir filters directories at recursive-add time. Hidden
// dirs (`.git`, `.vscode`) and `node_modules` are excluded to keep
// the watcher count manageable and avoid event floods.
func TestShouldWatchDir(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"src", true},
		{"a/b/c", true},
		{".", false}, // current dir treated as hidden by the dotfile rule
		{".git", false},
		{".vscode", false},
		{".hidden", false},
		{"node_modules", false},
		{"app/node_modules", false}, // base name matches even when nested
		{"some/.git", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := shouldWatchDir(tc.path); got != tc.want {
				t.Errorf("shouldWatchDir(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// addRecursive walks the tree and registers every non-hidden,
// non-node_modules directory with the watcher. The count returned
// matches what filepath.WalkDir sees minus the filtered subtrees.
func TestAddRecursive_FiltersHiddenAndNodeModules(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string) {
		t.Helper()
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mk("src")
	mk("src/components")
	mk("examples")
	mk(".git")
	mk(".git/objects")
	mk("node_modules")
	mk("node_modules/foo")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	count, err := addRecursive(w, root)
	if err != nil {
		t.Fatalf("addRecursive: %v", err)
	}
	// Expected watched: root + src + src/components + examples = 4.
	// .git and node_modules subtrees skipped entirely.
	want := 4
	if count != want {
		t.Errorf("watched dirs: %d, want %d", count, want)
	}
}

// Inviting addRecursive with a non-existent root surfaces the
// underlying os error rather than a panic.
func TestAddRecursive_MissingRootErrors(t *testing.T) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	_, err = addRecursive(w, filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing root")
	}
}

// affectedEntries scopes re-runs to entries whose import graph
// includes a changed file. Entry's own file and transitive deps both
// count; stdin and ungraphed entries always re-run (conservative).
func TestAffectedEntries(t *testing.T) {
	graphs := map[string]map[string]bool{
		"a.ts": {"/p/a.ts": true, "/p/helper.ts": true},
		"b.ts": {"/p/b.ts": true},
	}
	cases := []struct {
		name    string
		scripts []string
		changed map[string]bool
		want    []string
	}{
		{"shared dep hits only importer", []string{"a.ts", "b.ts"},
			map[string]bool{"/p/helper.ts": true}, []string{"a.ts"}},
		{"entry's own file", []string{"a.ts", "b.ts"},
			map[string]bool{"/p/b.ts": true}, []string{"b.ts"}},
		{"unrelated change → none", []string{"a.ts", "b.ts"},
			map[string]bool{"/p/zzz.ts": true}, nil},
		{"stdin always re-runs", []string{"-"},
			map[string]bool{"/p/anything.ts": true}, []string{"-"}},
		{"ungraphed entry always re-runs", []string{"new.ts"},
			map[string]bool{"/p/x.ts": true}, []string{"new.ts"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := affectedEntries(tc.scripts, graphs, tc.changed)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}
