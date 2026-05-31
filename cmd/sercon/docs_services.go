package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func servicesDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"exec.shell":    {Summary: "Run a subprocess. String cmd → /bin/sh -c (or `cmd /C` on Windows); array cmd → argv. Non-zero exits resolve; spawn failures and timeouts throw."},
		"exec.http":     {Summary: "HTTP via recon (preferred) or curl (fallback). 4xx / 5xx resolve as status; transport errors and timeouts throw. opts.backend = 'auto' | 'recon' | 'curl'."},
		"git.branch":    {Summary: "Current branch (empty when HEAD is detached) plus the list of local branches."},
		"git.isClean":   {Summary: "True iff `git status --porcelain` is empty."},
		"git.revParse":  {Summary: "Full 40-char SHA for the given rev. Invalid refs throw."},
		"git.status":    {Summary: "Parsed `git status --porcelain` entries: { path, indexStatus, workingStatus }."},
		"git.add":       {Summary: "Stage one path (string) or several (string[])."},
		"git.commit":    {Summary: "Create a commit; returns the post-commit HEAD SHA. opts.allowEmpty toggles --allow-empty."},
		"git.log":       {Summary: "Recent commits as { sha, shortSha, author, email, timestamp, subject }. opts.limit / opts.revRange."},
		"git.diffStat":  {Summary: "Aggregate { files, insertions, deletions } from `git diff --shortstat`. Default revRange HEAD~1..HEAD."},
		"git.runText":   {Summary: "Escape hatch: run any `git <args>`, get { stdout, stderr, exitCode } — exitCode is data, not a throw."},
		"gh.authStatus": {Summary: "Probe gh's auth state. Missing gh / unauthenticated resolve with { authenticated: false, … } — only context cancellation throws."},
		"gh.prList":     {Summary: "List pull requests on the cwd's repo (or opts.cwd). Defaults: open state, limit 30. Filters: state / limit / author."},
		"gh.repoView":   {Summary: "Repo metadata. With no arg uses cwd's repo; pass 'owner/name' for any repo gh can see. owner + defaultBranch are pre-flattened."},
		"ai.providers":  {Summary: "Which of claude / codex / copilot / gemini are on PATH, in preference order."},
		"ai.send":       {Summary: "Run a one-shot prompt through a provider. opts { prompt (required), provider?, system?, context?, timeout? }. Returns { provider, output, exitCode }. Non-zero exit is data; no provider throws."},
	}
}
