package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// searchFixture builds a temp tree and returns its root (absolute).
//
//	root/
//	  a.txt  b.go  .hidden.txt
//	  sub/c.go  sub/d.md
//	  node_modules/skip.go
//	  .hiddendir/deep.go
//	  .gitignore   (contents: "node_modules/\n*.md\n")
func searchFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "alpha\n")
	write("b.go", "package main\n")
	write(".hidden.txt", "secret\n")
	write("sub/c.go", "package sub\n")
	write("sub/d.md", "# doc\n")
	write("node_modules/skip.go", "package skip\n")
	write(".hiddendir/deep.go", "package deep\n")
	write(".gitignore", "node_modules/\n*.md\n")
	return root
}

// collect runs fsSearchWalk and returns the sorted rel paths (relative to root).
func collect(t *testing.T, root string, o walkOptions) []string {
	t.Helper()
	o.roots = []string{root}
	var got []string
	err := fsSearchWalk(context.Background(), o, func(e walkEntry) error {
		r, _ := filepath.Rel(root, e.abs)
		got = append(got, filepath.ToSlash(r))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(got)
	return got
}

func TestFsSearchWalk_DefaultsRespectGitignoreAndHidden(t *testing.T) {
	root := searchFixture(t)
	got := collect(t, root, walkOptions{gitignore: true, types: map[string]bool{"file": true}})
	// .hidden.txt skipped (hidden), node_modules/* skipped (gitignore dir prune),
	// *.md skipped (gitignore). .gitignore itself is a dotfile -> hidden-skipped.
	want := []string{"a.txt", "b.go", "sub/c.go"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFsSearchWalk_PrunesHiddenDirectories(t *testing.T) {
	root := searchFixture(t)
	// Defaults (hidden:false) must prune the hidden directory entirely: its
	// child .hiddendir/deep.go is a NON-hidden file, so if the walker had
	// descended into .hiddendir instead of returning filepath.SkipDir, the
	// file would surface. Its absence proves the dir was pruned, not descended.
	got := collect(t, root, walkOptions{gitignore: false, types: map[string]bool{"file": true}})
	for _, g := range got {
		if g == ".hiddendir/deep.go" {
			t.Fatalf("hidden directory was descended into, not pruned: got %v", got)
		}
	}
	// Sanity: the visible tree is exactly what remains after the hidden prune
	// (gitignore off, so node_modules/skip.go and sub/d.md stay).
	want := []string{"a.txt", "b.go", "node_modules/skip.go", "sub/c.go", "sub/d.md"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFsSearchWalk_HiddenAndNoGitignore(t *testing.T) {
	root := searchFixture(t)
	got := collect(t, root, walkOptions{hidden: true, gitignore: false, types: map[string]bool{"file": true}})
	want := []string{".gitignore", ".hidden.txt", ".hiddendir/deep.go", "a.txt", "b.go", "node_modules/skip.go", "sub/c.go", "sub/d.md"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFsSearchWalk_ExtAndGlobFilters(t *testing.T) {
	root := searchFixture(t)
	got := collect(t, root, walkOptions{gitignore: false, exts: map[string]bool{"go": true}, types: map[string]bool{"file": true}})
	want := []string{"b.go", "node_modules/skip.go", "sub/c.go"}
	if !equalStrings(got, want) {
		t.Fatalf("ext filter: got %v want %v", got, want)
	}
	got = collect(t, root, walkOptions{gitignore: false, globs: []string{"sub/**"}, types: map[string]bool{"file": true}})
	want = []string{"sub/c.go", "sub/d.md"}
	if !equalStrings(got, want) {
		t.Fatalf("glob filter: got %v want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
