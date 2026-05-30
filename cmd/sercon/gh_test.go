package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// parsePRListJSON must flatten the author wrapper. The synthetic
// payload mirrors a real `gh pr list --json` response.
func TestParsePRListJSON_FlattensAuthor(t *testing.T) {
	raw := []byte(`[
		{"number":1,"title":"a","state":"OPEN","author":{"login":"alice","id":"A","name":"Alice"},
		 "headRefName":"feat","baseRefName":"main","url":"u","createdAt":"t","updatedAt":"t"},
		{"number":2,"title":"b","state":"OPEN","author":{"login":"bob","id":"B"},
		 "headRefName":"fix","baseRefName":"main","url":"u","createdAt":"t","updatedAt":"t"}
	]`)
	out, err := parsePRListJSON(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len: %d", len(out))
	}
	if out[0].ToMap()["author"].(string) != "alice" {
		t.Errorf("author 0: %v", out[0].ToMap()["author"])
	}
	if out[1].ToMap()["author"].(string) != "bob" {
		t.Errorf("author 1: %v", out[1].ToMap()["author"])
	}
}

// Author may be present-as-null on some legacy PRs (when the author
// account has been deleted). The flattener must leave that intact
// rather than panic.
func TestParsePRListJSON_NullAuthorPreserved(t *testing.T) {
	raw := []byte(`[
		{"number":3,"title":"c","state":"CLOSED","author":null,
		 "headRefName":"x","baseRefName":"main","url":"u","createdAt":"t","updatedAt":"t"}
	]`)
	out, err := parsePRListJSON(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out[0].ToMap()["author"] != nil {
		t.Errorf("author should stay nil, got %T %v", out[0].ToMap()["author"], out[0].ToMap()["author"])
	}
}

// parseRepoViewJSON flattens owner.login → owner and
// defaultBranchRef.name → defaultBranch.
func TestParseRepoViewJSON_FlattensOwnerAndDefaultBranch(t *testing.T) {
	raw := []byte(`{
		"name":"sercon","owner":{"login":"codedeviate","id":"X","type":"User"},
		"description":"d","url":"https://github.com/x/sercon",
		"defaultBranchRef":{"name":"master","prefix":"refs/heads/"},
		"visibility":"PUBLIC"
	}`)
	out, err := parseRepoViewJSON(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m := out.ToMap()
	if m["owner"].(string) != "codedeviate" {
		t.Errorf("owner: %v", m["owner"])
	}
	if m["defaultBranch"].(string) != "master" {
		t.Errorf("defaultBranch: %v", m["defaultBranch"])
	}
	if _, leftover := out.Get("defaultBranchRef"); leftover {
		t.Error("defaultBranchRef should have been removed after flattening")
	}
}

// Empty repos have `defaultBranchRef: null`. Parser must still set a
// sane `defaultBranch: ""` rather than leaving it undefined.
func TestParseRepoViewJSON_EmptyRepoNullDefaultBranch(t *testing.T) {
	raw := []byte(`{
		"name":"new","owner":{"login":"x"},
		"defaultBranchRef":null,"visibility":"PRIVATE"
	}`)
	out, err := parseRepoViewJSON(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.ToMap()["defaultBranch"].(string) != "" {
		t.Errorf("defaultBranch should be empty string, got %v", out.ToMap()["defaultBranch"])
	}
}

// Malformed JSON must surface a clear parse error.
func TestParsePRListJSON_Malformed(t *testing.T) {
	if _, err := parsePRListJSON([]byte("{not-json")); err == nil {
		t.Fatal("expected error")
	}
}

// authStatus is a probe — missing gh resolves with
// `authenticated: false` rather than throwing. Make sure that
// contract holds by exercising it on a PATH where gh is absent.
func TestGhAuthStatus_NoGhResolvesFalse(t *testing.T) {
	// Build a PATH-less command env so `exec.LookPath("gh")` fails.
	// We can't rewrite the global PATH without race risk, so instead
	// re-run authStatus after temporarily blanking PATH via t.Setenv.
	t.Setenv("PATH", "/nonexistent")
	out, err := authStatusViaGoja(t)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out.Authenticated {
		t.Error("authenticated: true on a PATH without gh")
	}
	if out.Raw != "gh not on PATH" {
		t.Errorf("raw: %q", out.Raw)
	}
}

func authStatusViaGoja(t *testing.T) (ghAuthStatusResult, error) {
	t.Helper()
	vm := goja.New()
	return ghAuthStatus(context.Background(), goja.FunctionCall{
		Arguments: []goja.Value{vm.ToValue(nil)},
	})
}

// Integration probe: when gh IS on PATH, authStatus's `authenticated`
// flag must match whether `gh auth status` exits cleanly. We don't
// pin the actual login (it varies per host) — just the boolean.
func TestGhAuthStatus_AgreesWithGhAuthStatus(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not on PATH")
	}
	// Ground truth: does `gh auth status` exit zero?
	wantAuthed := exec.Command("gh", "auth", "status").Run() == nil

	out, err := authStatusViaGoja(t)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := out.Authenticated; got != wantAuthed {
		t.Errorf("authenticated: %v (want %v); raw=%q", got, wantAuthed, out.Raw)
	}
	if wantAuthed && strings.TrimSpace(out.User) == "" {
		t.Error("user should be non-empty when authenticated")
	}
}
