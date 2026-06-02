package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func servicesDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"exec.shell": {
			Summary: "Run a subprocess and wait for it to exit. String cmd → /bin/sh -c (or `cmd /C` on Windows); array cmd → argv (no shell). Non-zero exits resolve normally; spawn failures and timeouts throw.",
			Params: []scriptengine.Param{
				{Name: "cmd", Type: "string | string[]", Desc: "A string is passed verbatim to the host shell (/bin/sh -c on Unix, cmd /C on Windows) so quoting, pipes, and redirects work. A string[] is treated as argv: argv[0] is run directly with no shell, so use this form when arguments contain whitespace or shell metacharacters you don't want re-interpreted."},
				{Name: "opts", Type: "{ timeout?: number, cwd?: string, stdin?: string, env?: Record<string, string>, pane?: string | Pane, pty?: boolean }", Optional: true, Desc: "timeout in ms (default 30000); on expiry the process tree is killed and the call throws. cwd sets the working directory. stdin is fed to the process's standard input. env entries are merged on top of the inherited environment (they do not replace it). pane (a tui.pane name or Pane handle) streams stdout+stderr live into a TUI pane — in that mode the result's stdout/stderr strings stay empty. pty (default false) runs the command under a pseudo-terminal so it believes it is a terminal and emits color/progress; with a pane the output is rendered there, without a pane it is captured into stdout (stderr stays empty since a pty merges both streams). Unix only — on Windows pty is ignored and the normal pipe path is used."},
			},
			Returns: "Promise<{ stdout: string, stderr: string, exitCode: number, success: boolean, durationMs: number }> — stdout/stderr are captured (empty when streamed to a pane); exitCode is 0 on success; success is exitCode === 0; durationMs is wall-clock spawn-to-exit time.",
			Errors:  "Throws if cmd is missing, an empty string, an empty array, or a non-string array element; if the host binary is not on PATH or fails to start; if the timeout (or context cancellation) fires before exit; or if a named pane is referenced without a prior tui.layout. A non-zero exit code does NOT throw — it resolves with success:false.",
			Example: `const r = await services.exec.shell("echo hi");
if (r.success) runtime.log(r.stdout.trim());`,
		},
		"exec.stream": {
			Summary: "Run a subprocess and stream its stdout/stderr to a callback line by line as output arrives (unlike exec.shell, which buffers). String cmd → /bin/sh -c (or `cmd /C` on Windows); array cmd → argv (no shell). Resolves { exitCode, success, durationMs } on exit; non-zero exits resolve normally; spawn failures and timeouts reject.",
			Params: []scriptengine.Param{
				{Name: "cmd", Type: "string | string[]", Desc: "A string is passed to the host shell (/bin/sh -c on Unix, cmd /C on Windows) so pipes, redirects, and globs work. A string[] is treated as argv: argv[0] is run directly with no shell."},
				{Name: "onLine", Type: "(line: string, stream: \"stdout\" | \"stderr\") => void", Desc: "Called once per output line as it arrives. line has its trailing newline stripped; stream is 'stdout' or 'stderr'. A final line without a trailing newline is still delivered. Required — a non-function throws synchronously."},
				{Name: "opts", Type: "{ cwd?: string, env?: Record<string, string>, stdin?: string, timeout?: number }", Optional: true, Desc: "cwd sets the working directory; env entries merge on top of the inherited environment; stdin is fed to the process. timeout is in ms with NO default (0 / absent = run until exit, unlike exec.shell's 30000); when set, the process tree is killed on expiry and the call rejects."},
			},
			ReturnType: "Promise<{ exitCode: number; success: boolean; durationMs: number }>",
			Returns:    "Promise<{ exitCode: number, success: boolean, durationMs: number }> — resolves on process exit. exitCode is 0 on success; success is exitCode === 0; durationMs is wall-clock spawn-to-exit time. Output is delivered via onLine, not captured into the result.",
			Errors:     "Throws synchronously if cmd is missing or onLine is not a function. The Promise rejects if the host binary is not on PATH or fails to start, or if the timeout (or context cancellation) fires before exit. A non-zero exit code does NOT reject — it resolves with success:false.",
			Example: `const r = await services.exec.stream("echo one; echo two", (line, stream) => {
  runtime.log(stream, line);
});
runtime.log("exit", r.exitCode);`,
		},
		"exec.http": {
			Summary: "Make an HTTP request by shelling out to recon (preferred) or curl (fallback). 4xx/5xx resolve as status; transport errors and timeouts throw. opts.backend = 'auto' | 'recon' | 'curl'.",
			Params: []scriptengine.Param{
				{Name: "method", Type: "string", Desc: "HTTP verb (GET, POST, PUT, DELETE, PATCH, HEAD); lower-case input is uppercased before forwarding."},
				{Name: "url", Type: "string", Desc: "Target URL; must be fully qualified (the backend requires a scheme + host)."},
				{Name: "opts", Type: "{ headers?: Record<string, string>, body?: string, timeout?: number, follow?: boolean, insecure?: boolean, backend?: \"auto\" | \"recon\" | \"curl\" }", Optional: true, Desc: "headers emits one -H \"Name: Value\" per entry. body is written to a temp file and sent via --data-binary so CR/LF stay intact. timeout in ms (default 30000). follow toggles -L to follow 3xx redirects. insecure toggles -k to skip TLS verification. backend picks the tool: 'auto' (default) prefers recon then curl; 'recon' or 'curl' require that specific binary on PATH."},
			},
			Returns: "Promise<{ status: number, headers: Record<string, string>, body: string, durationMs: number, backend: \"recon\" | \"curl\" }> — status is the final HTTP status code; headers have lower-cased names (last response block on a redirect chain); body is the UTF-8 decoded response body; backend is whichever tool ran.",
			Errors:  "Throws if method or url is missing/empty; if the requested backend (or, for 'auto', neither recon nor curl) is on PATH; on transport errors (DNS failure, connection refused, TLS handshake); on timeout or context cancellation; or if the response headers can't be parsed. HTTP 4xx/5xx do NOT throw — they resolve with that status.",
			Example: `const r = await services.exec.http("GET", "https://example.com");
runtime.log(r.status, r.backend);`,
		},
		"git.branch": {
			Summary: "Current branch (empty when HEAD is detached) plus the list of local branches.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout to inspect; defaults to the engine's working directory."},
			},
			Returns: "Promise<{ current: string, detached: boolean, all: string[] }> — current is the checked-out branch name (\"\" when detached); detached is true on a detached HEAD; all lists every local branch (refs/heads) by short name.",
			Errors:  "Throws if git is not on PATH, the directory is not a git repository, or the underlying git command fails. A detached HEAD is reported via detached:true, not a throw.",
			Example: `const b = await services.git.branch();
runtime.log(b.detached ? "(detached)" : b.current);`,
		},
		"git.isClean": {
			Summary: "True iff `git status --porcelain` is empty.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout; defaults to the engine's working directory."},
			},
			Returns: "Promise<boolean> — true when the working tree has no staged, unstaged, or untracked changes.",
			Errors:  "Throws if git is not on PATH, the directory is not a git repository, or git status exits non-zero.",
			Example: `if (await services.git.isClean()) runtime.log("clean");`,
		},
		"git.revParse": {
			Summary: "Full 40-char SHA for the given rev. Invalid refs throw.",
			Params: []scriptengine.Param{
				{Name: "rev", Type: "string", Desc: "Any revision git understands (branch, tag, HEAD, short SHA, HEAD~2, etc.)."},
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout; defaults to the engine's working directory."},
			},
			Returns: "Promise<string> — the full 40-character commit SHA the rev resolves to.",
			Errors:  "Throws if rev is missing or empty, git is not on PATH, the directory is not a git repository, or the rev cannot be resolved (git's own error message is included).",
			Example: `const sha = await services.git.revParse("HEAD");`,
		},
		"git.status": {
			Summary: "Parsed `git status --porcelain` entries: { path, indexStatus, workingStatus }.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout; defaults to the engine's working directory."},
			},
			Returns: "Promise<Array<{ path: string, indexStatus: string, workingStatus: string }>> — one entry per changed path; indexStatus / workingStatus are the porcelain v1 X / Y status characters (e.g. \"M\", \"A\", \"?\"). An empty array means a clean tree.",
			Errors:  "Throws if git is not on PATH, the directory is not a git repository, or git status exits non-zero.",
			Example: `for (const e of await services.git.status())
  runtime.log(e.indexStatus + e.workingStatus, e.path);`,
		},
		"git.add": {
			Summary: "Stage one path (string) or several (string[]).",
			Params: []scriptengine.Param{
				{Name: "paths", Type: "string | string[]", Desc: "Path or paths to stage. Passed after a `--` separator so paths that look like flags (-foo) are handled literally."},
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout; defaults to the engine's working directory."},
			},
			Returns: "Promise<{ paths: string[] }> — the list of paths that were staged.",
			Errors:  "Throws if paths is missing, an empty string, or contains a non-string array element; if git is not on PATH; or if git add exits non-zero (e.g. a pathspec matching nothing).",
			Example: `await services.git.add(["src/a.ts", "src/b.ts"]);`,
		},
		"git.commit": {
			Summary: "Create a commit; returns the post-commit HEAD SHA. opts.allowEmpty toggles --allow-empty.",
			Params: []scriptengine.Param{
				{Name: "message", Type: "string", Desc: "Commit message (passed as a single -m argument)."},
				{Name: "opts", Type: "{ cwd?: string, allowEmpty?: boolean }", Optional: true, Desc: "cwd selects the checkout. allowEmpty adds --allow-empty so a commit succeeds with no staged changes (release markers, etc.); defaults to false."},
			},
			Returns: "Promise<{ sha: string }> — sha is the full SHA of the newly created HEAD commit.",
			Errors:  "Throws if message is missing or blank, git is not on PATH, or git commit exits non-zero (e.g. nothing staged and allowEmpty is false).",
			Example: `const c = await services.git.commit("chore: bump", { allowEmpty: true });`,
		},
		"git.log": {
			Summary: "Recent commits as { sha, shortSha, author, email, timestamp, subject }. opts.limit / opts.revRange.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ cwd?: string, limit?: number, revRange?: string }", Optional: true, Desc: "cwd selects the checkout. limit caps the number of commits (default 50; must be positive). revRange selects the range/ref to walk (default \"HEAD\")."},
			},
			Returns: "Promise<Array<{ sha: string, shortSha: string, author: string, email: string, timestamp: number, subject: string }>> — newest first; timestamp is the author Unix epoch seconds; subject is the commit's first line.",
			Errors:  "Throws if limit is <= 0, git is not on PATH, the directory is not a git repository, or git log exits non-zero (e.g. an unknown revRange).",
			Example: `const log = await services.git.log({ limit: 5 });
runtime.log(log[0].subject);`,
		},
		"git.diffStat": {
			Summary: "Aggregate { files, insertions, deletions } from `git diff --shortstat`. Default revRange HEAD~1..HEAD.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ cwd?: string, revRange?: string }", Optional: true, Desc: "cwd selects the checkout. revRange is the diff range (default \"HEAD~1..HEAD\", the last commit)."},
			},
			Returns: "Promise<{ files: number, insertions: number, deletions: number }> — counters parsed from git diff --shortstat. An empty diff returns all zeros.",
			Errors:  "Throws if git is not on PATH, the directory is not a git repository, or git diff exits non-zero (e.g. a bad revRange).",
			Example: `const d = await services.git.diffStat();
runtime.log(d.files, d.insertions, d.deletions);`,
		},
		"git.runText": {
			Summary: "Escape hatch: run any `git <args>`, get { stdout, stderr, exitCode } — exitCode is data, not a throw.",
			Params: []scriptengine.Param{
				{Name: "args", Type: "string | string[]", Desc: "git arguments (without the leading \"git\"), e.g. [\"tag\", \"--list\"]. A bare string is treated as a single argument."},
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout; defaults to the engine's working directory."},
			},
			Returns: "Promise<{ stdout: string, stderr: string, exitCode: number }> — captured streams plus git's exit code, so callers can react to any exit status without try/catch.",
			Errors:  "Throws if args is missing, an empty string, or an empty array; if git is not on PATH; or on a spawn failure / context cancellation. A non-zero exit code does NOT throw — it is returned in exitCode.",
			Example: `const r = await services.git.runText(["tag", "--list"]);
if (r.exitCode === 0) runtime.log(r.stdout);`,
		},
		"gh.authStatus": {
			Summary: "Probe gh's auth state. Missing gh / unauthenticated resolve with { authenticated: false, … } — only context cancellation throws.",
			Returns: "Promise<{ authenticated: boolean, user: string, raw: string }> — authenticated is true only when `gh api user` succeeds; user is the resolved login (\"\" when not authenticated); raw is the login on success or the underlying gh error / \"gh not on PATH\" otherwise.",
			Errors:  "Throws only on context cancellation. A missing gh binary or an unauthenticated session resolve with authenticated:false rather than throwing.",
			Example: `const a = await services.gh.authStatus();
if (a.authenticated) runtime.log("logged in as", a.user);`,
		},
		"gh.prList": {
			Summary: "List pull requests on the cwd's repo (or opts.cwd). Defaults: open state, limit 30. Filters: state / limit / author.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ cwd?: string, state?: string, limit?: number, author?: string }", Optional: true, Desc: "cwd selects the repo (defaults to the engine's working directory, which gh uses to detect the repo). state filters by PR state (\"open\" default, \"closed\", \"merged\", \"all\"). limit caps results (default 30; must be positive). author filters to PRs opened by that login."},
			},
			Returns: "Promise<Array<{ number: number, title: string, state: string, author: string, headRefName: string, baseRefName: string, url: string, createdAt: string, updatedAt: string }>> — one object per PR; author is flattened from gh's { login } wrapper to the bare login string; createdAt/updatedAt are ISO 8601 timestamps.",
			Errors:  "Throws if limit is <= 0, gh is not on PATH, gh exits non-zero (not authenticated, not a GitHub repo, etc.), the JSON can't be parsed, or on context cancellation.",
			Example: `const prs = await services.gh.prList({ state: "open", limit: 5 });
for (const pr of prs) runtime.log(pr.number, pr.title);`,
		},
		"gh.repoView": {
			Summary: "Repo metadata. With no arg uses cwd's repo; pass 'owner/name' for any repo gh can see. owner + defaultBranch are pre-flattened.",
			Params: []scriptengine.Param{
				{Name: "repo", Type: "string", Optional: true, Desc: "\"owner/name\" of any repo gh can access. Omit (or pass opts as the first arg) to view the repo detected from cwd."},
				{Name: "opts", Type: "{ cwd?: string }", Optional: true, Desc: "cwd selects the checkout gh uses to detect the current repo when repo is omitted."},
			},
			Returns: "Promise<{ name: string, owner: string, description: string, url: string, defaultBranch: string, visibility: string }> — owner is flattened from gh's { login } wrapper to the bare login; defaultBranch is flattened from defaultBranchRef.name (\"\" if absent); key order matches gh's output.",
			Errors:  "Throws if gh is not on PATH, gh exits non-zero (repo not found, not authenticated, etc.), the JSON can't be parsed, or on context cancellation.",
			Example: `const r = await services.gh.repoView("cli/cli");
runtime.log(r.owner, r.defaultBranch);`,
		},
		"ai.providers": {
			Summary:    "Which of claude / codex / copilot / gemini are on PATH, in preference order.",
			ReturnType: "string[]",
			Returns:    "string[] — the subset of supported AI CLIs found on PATH, in preference order (claude, codex, copilot, gemini); an empty array when none are installed. Synchronous (not a Promise).",
			Errors:     "Never throws.",
			Example:    `const ps = services.ai.providers(); // e.g. ["claude", "gemini"]`,
		},
		"ai.send": {
			Summary: "Run a one-shot prompt through a provider. opts { prompt (required), provider?, system?, context?, timeout? }. Returns { provider, output, exitCode }. Non-zero exit is data; no provider throws.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ prompt: string, provider?: \"claude\" | \"codex\" | \"copilot\" | \"gemini\", system?: string, context?: string, timeout?: number }", Desc: "prompt is required. provider names the CLI to use; when omitted, the first provider on PATH (in preference order) is chosen. system and context are prepended to the prompt as \"System: …\" / \"Context: …\" blocks (a portable substitute for each CLI's own flags). timeout in ms (default 120000)."},
			},
			Returns: "Promise<{ provider: string, output: string, exitCode: number }> — provider is the CLI that ran; output is its trimmed stdout (or stderr when stdout is empty on a non-zero exit); exitCode is 0 on success.",
			Errors:  "Throws if opts.prompt is missing/empty, no provider is on PATH (when provider is unset), the named provider is unknown, the CLI fails to spawn, or on timeout / context cancellation. A non-zero exit code does NOT throw — it resolves with that exitCode.",
			Example: `const r = await services.ai.send({ prompt: "Say hi", provider: "claude" });
runtime.log(r.output);`,
		},
	}
}
