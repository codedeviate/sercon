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

		// agentBrowser namespace docs
		"agentBrowser.available": {
			Summary: "True when the agent-browser CLI is on PATH. Sync boolean, resolved once per Run. Gate calls on this; every binding throws a clean error when the CLI is absent.",
			Returns: "boolean — true if `agent-browser` is on PATH.",
			Example: `if (!services.agentBrowser.available) runtime.log("install agent-browser first");`,
		},
		"agentBrowser.version": {
			Summary: "The agent-browser CLI version string.",
			Returns: "Promise<string> — the version reported by `agent-browser --version`.",
			Errors:  "Throws if the agent-browser CLI is not on PATH.",
			Example: `runtime.log(await services.agentBrowser.version());`,
		},
		"agentBrowser.launch": {
			Summary: "Allocate a browser session and return a handle. Synchronous (no browser starts until the first command). Pass opts.session to name the session; otherwise a unique id is generated. Launch flags (headed, profile, proxy, userAgent, device, colorScheme, ignoreHttpsErrors, engine, executablePath, enable, args) are threaded into every call the handle makes. Sessions the script does not close() are best-effort closed when the Run ends.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ session?: string, headed?: boolean, profile?: string, proxy?: string, userAgent?: string, device?: string, colorScheme?: string, ignoreHttpsErrors?: boolean, engine?: string, executablePath?: string, enable?: string, args?: string, timeout?: number }", Optional: true, Desc: "Launch flags captured for the lifetime of the handle and threaded into every subprocess call. session names the agent-browser session (auto-generated when omitted). timeout is the per-call agent-browser subprocess timeout in milliseconds (default 30000, 0 disables); when a call exceeds the limit the Promise rejects with a clear 'timed out' error instead of hanging forever."},
			},
			ReturnType: "{ session: string; open(url: string, opts?: object): Promise<any>; back(): Promise<any>; forward(): Promise<any>; reload(): Promise<any>; wait(selOrMs: string | number): Promise<any>; connect(target: string): Promise<any>; click(sel: string): Promise<any>; dblclick(sel: string): Promise<any>; hover(sel: string): Promise<any>; focus(sel: string): Promise<any>; check(sel: string): Promise<any>; uncheck(sel: string): Promise<any>; scrollIntoView(sel: string): Promise<any>; fill(sel: string, text: string): Promise<any>; type(sel: string, text: string): Promise<any>; press(key: string): Promise<any>; select(sel: string, ...values: string[]): Promise<any>; scroll(dir: string, px?: number): Promise<any>; drag(src: string, dst: string): Promise<any>; upload(sel: string, files: string | string[]): Promise<any>; download(sel: string, path: string): Promise<any>; keyboard: { type(text: string): Promise<any>; insertText(text: string): Promise<any> }; mouse: { move(x: number, y: number): Promise<any>; down(button?: string): Promise<any>; up(button?: string): Promise<any>; wheel(dy: number, dx?: number): Promise<any> }; get(what: string, sel?: string): Promise<any>; isVisible(sel: string): Promise<any>; isEnabled(sel: string): Promise<any>; isChecked(sel: string): Promise<any>; eval(code: string): Promise<any>; snapshot(opts?: object): Promise<any>; console(opts?: object): Promise<any>; errors(opts?: object): Promise<any>; highlight(sel: string): Promise<any>; find(locator: string, value: string, opts: { action: string, text?: string }): Promise<any>; locator(spec: object | string, value?: string): object; set: { viewport(w: number, h: number, scale?: number): Promise<any>; device(name: string): Promise<any>; geo(lat: number, lng: number): Promise<any>; offline(on?: boolean): Promise<any>; headers(headers: Record<string, string>): Promise<any>; credentials(user: string, pass: string): Promise<any>; media(scheme?: \"dark\" | \"light\", reducedMotion?: boolean): Promise<any> }; record: { start(path: string, url?: string): Promise<any>; stop(): Promise<any> }; screenshot(path?: string, opts?: { selector?: string, full?: boolean, annotate?: boolean, format?: \"png\" | \"jpeg\", quality?: number }): Promise<{ path?: string, size?: number, bytes?: number[], format: string }>; pdf(path?: string): Promise<{ path?: string, size?: number, bytes?: number[], format: string }>; network: { route(url: string, opts?: { abort?: boolean, body?: unknown, resourceType?: string }): Promise<any>; unroute(url?: string): Promise<any>; requests(opts?: { clear?: boolean, filter?: string, type?: string, method?: string, status?: string }): Promise<any>; request(id: string): Promise<any>; har: { start(path?: string): Promise<any>; stop(path?: string): Promise<any> } }; cookies: { get(): Promise<any>; set(name: string, value: string, opts?: { url?: string, domain?: string, path?: string, httpOnly?: boolean, secure?: boolean, sameSite?: \"Strict\" | \"Lax\" | \"None\", expires?: number }): Promise<any>; clear(): Promise<any> }; storage: { local: { get(key?: string): Promise<any>; set(key: string, value: string): Promise<any>; clear(): Promise<any> }; session: { get(key?: string): Promise<any>; set(key: string, value: string): Promise<any>; clear(): Promise<any> } }; tabs: { list(): Promise<any>; new(url?: string, opts?: { label?: string }): Promise<any>; close(ref?: string): Promise<any>; select(ref: string): Promise<any> }; diff: { snapshot(opts?: { baseline?: string, selector?: string, compact?: boolean, depth?: number }): Promise<any>; screenshot(opts: { baseline: string, output?: string, threshold?: number }): Promise<any>; url(url1: string, url2: string): Promise<any> }; trace: { start(): Promise<any>; stop(path?: string): Promise<any> }; profiler: { start(opts?: { categories?: string }): Promise<any>; stop(path?: string): Promise<any> }; inspect(): Promise<any>; clipboard(op: \"read\" | \"write\" | \"copy\" | \"paste\", text?: string): Promise<any>; vitals(url?: string): Promise<any>; pushstate(url: string): Promise<any>; react: { tree(): Promise<any>; inspect(id: string): Promise<any>; renders: { start(): Promise<any>; stop(): Promise<any> }; suspense(opts?: { onlyDynamic?: boolean }): Promise<any> }; stream: { enable(opts?: { port?: number }): Promise<any>; disable(): Promise<any>; status(): Promise<any> }; chat(message: string, opts?: { model?: string }): Promise<any>; cmd(command: string, ...args: string[]): Promise<any>; batch(cmds: string[], opts?: { bail?: boolean }): Promise<any>; auth: { login(name: string): Promise<any> }; close(): Promise<any> }",
			Returns:    "A handle object with a read-only session string and methods: open, back, forward, reload, wait, connect, click, dblclick, hover, focus, fill, type, press, check, uncheck, select, scroll, scrollIntoView, drag, upload, download, keyboard.{type,insertText}, mouse.{move,down,up,wheel}, get, isVisible, isEnabled, isChecked, eval, snapshot, console, errors, highlight, find, locator, close. Every async method resolves to an agent-browser envelope { success: boolean, data: object, error: string|null }; drill into .data for the actual values.",
			Errors:     "launch() itself does not throw for a missing CLI (it allocates only); the first method call throws if agent-browser is not on PATH.",
			Example: `const b = services.agentBrowser.launch({ headed: false });
await b.open("https://example.com");
const r = await b.get("title");
runtime.log(r.data?.title);
await b.close();`,
		},
		"agentBrowser.open": {
			Summary: "Navigate the browser session to a URL. The URL may be http/https, a data: URI, or any scheme the browser supports.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "The URL to navigate to. Required."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope; data typically contains { url, title } after navigation.",
			Errors:  "Throws if url is missing, if agent-browser is not on PATH, or if navigation fails.",
			Example: `const b = services.agentBrowser.launch();
await b.open("https://example.com");
await b.close();`,
		},
		"agentBrowser.click": {
			Summary: "Click an element matching a CSS selector.",
			Params: []scriptengine.Param{
				{Name: "selector", Type: "string", Desc: "CSS selector for the element to click. Required."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if selector is missing, element not found, or agent-browser is not on PATH.",
			Example: `await b.click("#submit-btn");`,
		},
		"agentBrowser.fill": {
			Summary: "Fill an input element with text.",
			Params: []scriptengine.Param{
				{Name: "selector", Type: "string", Desc: "CSS selector for the input element. Required."},
				{Name: "text", Type: "string", Desc: "Text to fill into the element."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if selector is missing, element not found, or agent-browser is not on PATH.",
			Example: `await b.fill("#search", "typescript");`,
		},
		"agentBrowser.get": {
			Summary: "Get a page property. what is one of: text, html, value, attr, title, url, count, box, styles, cdp-url. Pass selector as the second argument for element-scoped queries.",
			Params: []scriptengine.Param{
				{Name: "what", Type: "string", Desc: "Property name: text, html, value, attr, title, url, count, box, styles, cdp-url. Required."},
				{Name: "selector", Type: "string", Optional: true, Desc: "CSS selector to scope the query to a specific element."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser wraps results in this envelope; the requested value lives under data (e.g. get(\"title\") → { success, data: { title }, error }, get(\"value\", \"#sel\") → { success, data: { value }, error }).",
			Errors:  "Throws if what is missing, the selector finds no element, or agent-browser is not on PATH.",
			Example: `const r = await b.get("title");
runtime.log(r.data?.title);

const t = await b.get("text", "#main");
runtime.log(t.data?.text);`,
		},
		"agentBrowser.handle.snapshot": {
			Summary: "Return an accessibility tree snapshot of the current page. Useful for reading page structure without CSS selectors.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ interactive?: boolean, compact?: boolean, depth?: number, selector?: string }", Optional: true, Desc: "interactive: include interactive-only elements (-i). compact: compact output (-c). depth: max tree depth (-d). selector: scope to a subtree (-s selector)."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope; data contains the accessibility tree.",
			Errors:  "Throws if agent-browser is not on PATH or the snapshot fails.",
			Example: `const snap = await b.snapshot({ interactive: true });
runtime.log("snapshot keys:", Object.keys(snap).join(", "));`,
		},
		"agentBrowser.find": {
			Summary: "Locate an element using a semantic locator (role, text, label, etc.) and perform an action in one shot.",
			Params: []scriptengine.Param{
				{Name: "locator", Type: "string", Desc: "Locator type: role, text, label, placeholder, alt, title, testid. Required."},
				{Name: "value", Type: "string", Desc: "Value to match for the given locator type. Required."},
				{Name: "opts", Type: "{ action: string, text?: string }", Desc: "action is required (e.g. click, fill, hover, check). text is passed as the fill text when action is 'fill' or 'type'."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if locator, value, or opts.action is missing, or if agent-browser is not on PATH.",
			Example: `await b.find("role", "button", { action: "click" });
await b.find("text", "Search", { action: "fill", text: "query" });`,
		},
		"agentBrowser.close": {
			Summary: "Close the browser session. Idempotent: a second close is a no-op.",
			Returns: "Promise<{ closed: boolean }> — { closed: true } on the first call; an empty object on subsequent calls.",
			Errors:  "Throws if the close command fails (e.g. agent-browser error).",
			Example: `await b.close();`,
		},

		// Phase 2 — defaults bag
		"agentBrowser.defaultOptions": {
			Summary: "Return a shallow copy of the current namespace-level launch defaults. These are merged (under per-call opts) into every subsequent launch().",
			Returns: "object — a plain-object copy of the current defaults map. Empty object when no defaults have been set.",
			Errors:  "Never throws.",
			Example: `services.agentBrowser.setDefaultOptions({ headed: false });
runtime.log(JSON.stringify(services.agentBrowser.defaultOptions())); // {"headed":false}`,
		},
		"agentBrowser.setDefaultOptions": {
			Summary: "Replace the namespace-level launch defaults with the supplied object. Merged (under per-call opts) into every subsequent launch(). Affects only the current Run.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "object", Desc: "A plain object of launch option key/value pairs. The entire defaults map is replaced (not merged) with this object."},
			},
			Returns: "void",
			Errors:  "Never throws.",
			Example: `services.agentBrowser.setDefaultOptions({ headed: false, proxy: "http://proxy:3128" });
const b = services.agentBrowser.launch(); // headed:false + proxy inherited`,
		},
		"agentBrowser.clearDefaultOptions": {
			Summary: "Reset the namespace-level launch defaults to an empty object, removing any values set by setDefaultOptions.",
			Returns: "void",
			Errors:  "Never throws.",
			Example: `services.agentBrowser.clearDefaultOptions();
runtime.log(JSON.stringify(services.agentBrowser.defaultOptions())); // {}`,
		},

		// Phase 2 — namespace-level one-shot shortcuts
		"agentBrowser.screenshot": {
			Summary: "One-shot shortcut: launch an ephemeral session, open url, capture a screenshot, and close. Equivalent to launch()+open(url)+screenshot(path?,opts?)+close() but in a single call.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "URL to open (http/https or data: URI). Required."},
				{Name: "path", Type: "string", Optional: true, Desc: "Output file path. When supplied the screenshot is written there and the result has { path, size, format }; when omitted the image bytes are returned as a number[] (byte-value array) in { bytes, format }."},
				{Name: "opts", Type: "{ selector?: string, full?: boolean, annotate?: boolean, format?: \"png\" | \"jpeg\", quality?: number }", Optional: true, Desc: "Capture options. selector scopes the capture to an element. full captures the full page. format defaults to png. quality (0–100) applies to jpeg."},
			},
			ReturnType: "Promise<{ path: string, size: number, format: string } | { bytes: number[], format: string }>",
			Returns:    "Promise — path given: { path: string, size: number, format: string }; no path: { bytes: number[], format: string } where bytes is a plain JS number[] (byte-value array); wrap with new Uint8Array(bytes) to get a typed array.",
			Errors:     "Throws if url is missing; if agent-browser is not on PATH; on navigation, capture, or I/O failure.",
			Example: `const shot = await services.agentBrowser.screenshot("data:text/html,<h1>Hi</h1>");
runtime.log(new Uint8Array(shot.bytes).length, shot.format); // e.g. 12345 png`,
		},
		"agentBrowser.pdf": {
			Summary: "One-shot shortcut: launch an ephemeral session, open url, capture a PDF, and close.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "URL to open. Required."},
				{Name: "path", Type: "string", Optional: true, Desc: "Output file path. When supplied the PDF is written there and the result has { path, size, format }; when omitted the PDF bytes are returned as a number[] in { bytes, format }."},
			},
			ReturnType: "Promise<{ path: string, size: number, format: string } | { bytes: number[], format: string }>",
			Returns:    "Promise — path given: { path: string, size: number, format: string }; no path: { bytes: number[], format: string }.",
			Errors:     "Throws if url is missing; if agent-browser is not on PATH; on navigation, capture, or I/O failure.",
			Example: `const pdf = await services.agentBrowser.pdf("data:text/html,<h1>Hi</h1>");
runtime.log(new Uint8Array(pdf.bytes).length, pdf.format); // e.g. 5678 pdf`,
		},
		"agentBrowser.snapshot": {
			Summary: "One-shot shortcut: launch an ephemeral session, open url, take an accessibility-tree snapshot, and close.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "URL to open. Required."},
				{Name: "opts", Type: "{ interactive?: boolean, compact?: boolean, depth?: number, selector?: string }", Optional: true, Desc: "Snapshot options forwarded to the handle's snapshot() method."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope; data contains the accessibility tree.",
			Errors:  "Throws if url is missing; if agent-browser is not on PATH; on navigation or snapshot failure.",
			Example: `const snap = await services.agentBrowser.snapshot("data:text/html,<h1>Hi</h1>", { compact: true });
runtime.log(JSON.stringify(snap.data).slice(0, 100));`,
		},
		"agentBrowser.eval": {
			Summary: "One-shot shortcut: launch an ephemeral session, open url, evaluate a JS expression in the page, and close.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "URL to open. Required."},
				{Name: "js", Type: "string", Desc: "JavaScript expression to evaluate in the page context. Required."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope; data.result holds the serialised return value.",
			Errors:  "Throws if url or js is missing; if agent-browser is not on PATH; on navigation or evaluation failure.",
			Example: `const r = await services.agentBrowser.eval("data:text/html,<title>Hi</title>", "document.title");
runtime.log(r.data?.result); // "Hi"`,
		},

		// Phase 3 — handle-level network/cookies/storage/tabs/diff
		"agentBrowser.network": {
			Summary: "Handle sub-object for network interception and monitoring: network.route, network.unroute, network.requests, network.request, network.har.start, network.har.stop.",
			Returns: "object — the network namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>hi</h1>");
await b.network.route("**/api/*", { abort: true });
const reqs = await b.network.requests({ clear: true });
runtime.log("requests:", JSON.stringify(reqs.data));
await b.close();`,
		},
		"agentBrowser.cookies": {
			Summary: "Handle sub-object for cookie management: cookies.get(), cookies.set(name, value, opts?), cookies.clear().",
			Returns: "object — the cookies namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>hi</h1>");
await b.cookies.set("sid", "abc123", { sameSite: "Lax", httpOnly: true });
const jar = await b.cookies.get();
runtime.log("cookies:", JSON.stringify(jar.data));
await b.cookies.clear();
await b.close();`,
		},
		"agentBrowser.storage": {
			Summary: "Handle sub-object for web storage: storage.local.{get,set,clear} and storage.session.{get,set,clear}.",
			Returns: "object — the storage namespace with local and session sub-objects. Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>hi</h1>");
await b.storage.local.set("theme", "dark");
const theme = await b.storage.local.get("theme");
runtime.log("theme:", JSON.stringify(theme.data));
await b.storage.session.set("token", "xyz");
await b.close();`,
		},
		"agentBrowser.tabs": {
			Summary: "Handle sub-object for tab management: tabs.list(), tabs.new(url?, opts?), tabs.close(ref?), tabs.select(ref). Tab refs are t1/t2/… or user labels.",
			Returns: "object — the tabs namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH, the session is closed, or a tab ref is required but missing.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>tab1</h1>");
await b.tabs.new("data:text/html,<h1>tab2</h1>", { label: "second" });
const list = await b.tabs.list();
runtime.log("tabs:", JSON.stringify(list.data));
await b.tabs.select("t1");
await b.tabs.close("second");
await b.close();`,
		},
		"agentBrowser.diff": {
			Summary: "Handle sub-object for page diffing: diff.snapshot(opts?), diff.screenshot(opts), diff.url(url1, url2).",
			Returns: "object — the diff namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH, the session is closed, or a required option (e.g. baseline) is missing.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>hello</h1>");
const snap = await b.diff.snapshot();
runtime.log("diff.snapshot ok:", snap.success);
// Compare two URLs side-by-side (no open() needed for diff.url):
const cmp = await b.diff.url("data:text/html,<h1>a</h1>", "data:text/html,<h1>b</h1>");
runtime.log("diff.url ok:", cmp.success);
await b.close();`,
		},

		// Phase 4 — handle-level debug/perf groups
		"agentBrowser.trace": {
			Summary: "Handle sub-object for Chrome DevTools tracing: trace.start() begins a trace, trace.stop(path?) stops it and optionally saves to a file.",
			Returns: "object — the trace namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>hi</h1>");
await b.trace.start();
// ... interact ...
await b.trace.stop("/tmp/trace.json");
await b.close();`,
		},
		"agentBrowser.profiler": {
			Summary: "Handle sub-object for V8 CPU profiling: profiler.start(opts?) begins profiling (opts.categories narrows the V8/Blink categories), profiler.stop(path?) stops it.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ categories?: string }", Optional: true, Desc: "categories is a comma-separated list of V8/Blink profiling categories (e.g. 'v8,blink')."},
			},
			Returns: "object — the profiler namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>hi</h1>");
await b.profiler.start({ categories: "v8,blink" });
// ... interact ...
await b.profiler.stop("/tmp/profile.json");
await b.close();`,
		},
		"agentBrowser.inspect": {
			Summary: "Open the Chrome DevTools inspector on the current session and return the DevTools URL.",
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope; data typically contains { url } for the DevTools connection.",
			Errors:  "Throws if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("https://example.com");
const r = await b.inspect();
runtime.log("DevTools:", r.data?.url);
await b.close();`,
		},
		"agentBrowser.clipboard": {
			Summary: "Read from or write to the system clipboard. op is one of 'read', 'write', 'copy', 'paste'.",
			Params: []scriptengine.Param{
				{Name: "op", Type: "\"read\" | \"write\" | \"copy\" | \"paste\"", Desc: "Clipboard operation. Required."},
				{Name: "text", Type: "string", Optional: true, Desc: "Text to write (only used when op is 'write')."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if op is missing or agent-browser is not on PATH.",
			Example: `await b.clipboard("write", "hello");
const r = await b.clipboard("read");
runtime.log(r.data?.text);`,
		},
		"agentBrowser.vitals": {
			Summary: "Collect Core Web Vitals (LCP, FID, CLS, TTFB, etc.) for the current page or a given URL.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Optional: true, Desc: "URL to navigate to before measuring. When omitted, vitals are measured on the currently loaded page."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope; data contains the Core Web Vitals metrics.",
			Errors:  "Throws if agent-browser is not on PATH or the session is closed.",
			Example: `const v = await b.vitals();
runtime.log("LCP:", v.data?.lcp);`,
		},
		"agentBrowser.pushstate": {
			Summary: "Perform a client-side SPA navigation using history.pushState without a full page reload.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "The URL to push into the browser history. Required."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if url is missing or agent-browser is not on PATH.",
			Example: `await b.pushstate("/app/dashboard");`,
		},

		// Phase 4 — handle-level React DevTools
		"agentBrowser.react": {
			Summary: "Handle sub-object for React DevTools integration: react.tree(), react.inspect(id), react.renders.start/stop(), react.suspense(opts?). Requires the session launched with launch({ enable: 'react-devtools' }).",
			Returns: "object — the react namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>. agent-browser returns a clear error when react-devtools was not enabled at launch time.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH, the session is closed, or react-devtools was not enabled at launch.",
			Example: `const b = services.agentBrowser.launch({ enable: "react-devtools" });
await b.open("https://react-app.example.com");
const tree = await b.react.tree();
runtime.log(JSON.stringify(tree.data).slice(0, 200));
const suspense = await b.react.suspense({ onlyDynamic: true });
runtime.log("suspense ok:", suspense.success);
await b.close();`,
		},

		// Phase 4 — handle-level stream/chat/cmd/batch
		"agentBrowser.stream": {
			Summary: "Handle sub-object for live streaming of browser events: stream.enable(opts?), stream.disable(), stream.status(). Streaming makes page events available over a local WebSocket.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port?: number }", Optional: true, Desc: "port selects the streaming port (default chosen by agent-browser)."},
			},
			Returns: "object — the stream namespace (not callable itself; use sub-methods). Each sub-method returns Promise<{ success: boolean, data: object, error: string|null }>.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `await b.stream.enable({ port: 9229 });
const status = await b.stream.status();
runtime.log("streaming:", status.data?.enabled);
await b.stream.disable();`,
		},
		"agentBrowser.chat": {
			Summary: "Send a single natural-language instruction to the browser session's AI gateway. The AI interprets the message and drives the page. Requires an AI gateway configured in agent-browser; errors cleanly if not configured.",
			Params: []scriptengine.Param{
				{Name: "message", Type: "string", Desc: "Natural-language instruction for the AI to execute. Required."},
				{Name: "opts", Type: "{ model?: string }", Optional: true, Desc: "model selects the AI model (uses the agent-browser default when omitted)."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if message is missing, if agent-browser is not on PATH, or if no AI gateway is configured.",
			Example: `// Requires agent-browser to have an AI gateway configured.
const r = await b.chat("Click the Login button");
runtime.log("chat ok:", r.success);`,
		},
		"agentBrowser.cmd": {
			Summary: "Generic escape hatch: run any agent-browser command with the current session context and return the parsed envelope. Use this for subcommands sercon doesn't model yet.",
			Params: []scriptengine.Param{
				{Name: "command", Type: "string", Desc: "The agent-browser subcommand to run (e.g. 'get', 'scroll'). Required."},
				{Name: "args", Type: "string[]", Desc: "Additional arguments to pass to the subcommand (spread after the command)."},
			},
			Returns: "Promise<{ success: boolean, data: object, error: string|null }> — agent-browser envelope.",
			Errors:  "Throws if command is missing or agent-browser is not on PATH.",
			Example: `const r = await b.cmd("get", "title");
runtime.log("title via cmd:", r.data?.title);`,
		},
		"agentBrowser.batch": {
			Summary: "Run multiple agent-browser command strings in a single round-trip. Each element of cmds is a full command string (e.g. 'get title'). Returns a JSON array of per-command results, not the usual envelope.",
			Params: []scriptengine.Param{
				{Name: "cmds", Type: "string[]", Desc: "Array of full command strings to execute sequentially. Required."},
				{Name: "opts", Type: "{ bail?: boolean }", Optional: true, Desc: "bail: stop on the first failed command and return results up to that point."},
			},
			Returns: "Promise<Array<{ success: boolean, data: object, error: string|null }>> — a JSON array of per-command result envelopes (not the usual single envelope).",
			Errors:  "Throws if cmds is missing/not an array or agent-browser is not on PATH.",
			Example: `const results = await b.batch(["get title", "get url"], { bail: false });
for (const r of results) runtime.log(r.success, r.data);`,
		},

		// Phase 4 — namespace-level auth vault
		"agentBrowser.auth": {
			Summary: "Namespace object for the auth vault (session-independent): auth.save, auth.list, auth.show, auth.delete. Passwords are never placed in argv — auth.save sends the password via stdin (--password-stdin).",
			Returns: "object — the auth namespace (not callable itself; use sub-methods). auth.list returns an array of profile names; auth.show/delete return an envelope; auth.save returns an envelope. Handle-level auth.login(name) is a separate method on the session handle.",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH, or required fields (name, url, username, password) are missing.",
			Example: `// Save a login profile (password sent via stdin, never via argv).
await services.agentBrowser.auth.save("prod", {
  url: "https://app.example.com/login",
  username: "admin",
  password: "s3cret",
  usernameSelector: "#user",
  passwordSelector: "#pass",
});

// List all saved profiles.
const profiles = await services.agentBrowser.auth.list();
runtime.log(JSON.stringify(profiles.data));

// Log in using a saved profile (handle method — requires an open session).
const b = services.agentBrowser.launch();
await b.open("https://app.example.com/login");
await b.auth.login("prod");
await b.close();`,
		},

		// Phase 2 — handle-level set.* and record.*
		"agentBrowser.set": {
			Summary: "Namespace object with browser-settings sub-methods on an open handle: set.viewport, set.device, set.geo, set.offline, set.headers, set.credentials, set.media.",
			Returns: "object — the set namespace (not callable itself; use sub-methods).",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("data:text/html,<h1>Test</h1>");
await b.set.viewport(1920, 1080);
await b.set.device("iPhone 12");
await b.close();`,
		},
		"agentBrowser.record": {
			Summary: "Namespace object for video recording on an open handle: record.start(path, url?) and record.stop().",
			Returns: "object — the record namespace (not callable itself; use sub-methods).",
			Errors:  "Never throws directly; sub-methods throw if agent-browser is not on PATH or the session is closed.",
			Example: `const b = services.agentBrowser.launch();
await b.open("https://example.com");
await b.record.start("/tmp/clip.webm");
// ... interact ...
await b.record.stop();
await b.close();`,
		},

		// webdriver namespace docs
		"webdriver.available": {
			Summary: "True when a W3C WebDriver binary (chromedriver or geckodriver) is on PATH. Sync boolean, resolved once per Run. Gate calls on this before using probe or connect.",
			Returns: "boolean — true if chromedriver or geckodriver is found on PATH.",
			Example: `if (!services.webdriver.available) {
  runtime.log("install chromedriver or geckodriver first");
}`,
		},
		"webdriver.probe": {
			Summary: "Check whether a WebDriver endpoint responds at opts.url/status. Returns { ready, status } on HTTP success or { ready: false, error } on transport failure. Does not throw on network errors.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ url: string }", Desc: "url is required — the base URL of a running WebDriver server (e.g. 'http://127.0.0.1:9515')."},
			},
			ReturnType: "Promise<{ ready: boolean; status?: number; error?: string }>",
			Returns:    "Promise<{ ready, status? } | { ready: false, error }> — ready is true when the endpoint returns HTTP 200; status is the HTTP status code when a response was received; error is the transport error message when the request failed entirely.",
			Errors:     "Throws if opts.url is missing or empty. Transport failures resolve with { ready: false, error } rather than throwing.",
			Example: `const r = await services.webdriver.probe({ url: "http://127.0.0.1:9515" });
runtime.log("ready:", r.ready);`,
		},
		"webdriver.connect": {
			Summary: "Connect to a running WebDriver server (opts.url) or start an installed local chromedriver/geckodriver and dial it. Returns a session handle whose methods drive the browser. Sessions are quit on Run end if the script does not call quit() explicitly.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ browser?: \"chrome\" | \"firefox\", headless?: boolean, url?: string, args?: string[], capabilities?: object, commandTimeout?: number }", Optional: true, Desc: "browser selects the driver binary (default 'chrome'). headless defaults to true. url, if given, dials an already-running driver at that base URL instead of starting one. args appends extra browser flags. capabilities is an escape hatch for raw W3C capability overrides merged last. commandTimeout (ms, default 30000) bounds each low-level WebDriver request so a driver blocked behind an open alert or an unreachable endpoint can't hang the call; 0 or negative disables the per-command deadline. quit()/Run-end also cancel any in-flight command."},
			},
			ReturnType: "Promise<{ get(url: string): Promise<{ ok: true }>; url(): Promise<string>; title(): Promise<string>; back(): Promise<{ ok: true }>; forward(): Promise<{ ok: true }>; refresh(): Promise<{ ok: true }>; find(by: string, value: string): Promise<{ click(): Promise<{ ok: true }>; sendKeys(text: string): Promise<{ ok: true }>; clear(): Promise<{ ok: true }>; submit(): Promise<{ ok: true }>; text(): Promise<string>; getAttribute(name: string): Promise<string>; cssValue(name: string): Promise<string>; tagName(): Promise<string>; isDisplayed(): Promise<boolean>; isEnabled(): Promise<boolean>; isSelected(): Promise<boolean>; find(by: string, value: string): Promise<any>; findAll(by: string, value: string): Promise<any[]>; screenshot(path?: string): Promise<{ path?: string; size?: number; bytes?: number[]; format: \"png\" }> }>; findAll(by: string, value: string): Promise<Array<{ click(): Promise<{ ok: true }>; sendKeys(text: string): Promise<{ ok: true }>; clear(): Promise<{ ok: true }>; submit(): Promise<{ ok: true }>; text(): Promise<string>; getAttribute(name: string): Promise<string>; cssValue(name: string): Promise<string>; tagName(): Promise<string>; isDisplayed(): Promise<boolean>; isEnabled(): Promise<boolean>; isSelected(): Promise<boolean>; find(by: string, value: string): Promise<any>; findAll(by: string, value: string): Promise<any[]>; screenshot(path?: string): Promise<{ path?: string; size?: number; bytes?: number[]; format: \"png\" }> }>>; source(): Promise<string>; screenshot(path?: string): Promise<{ path?: string; size?: number; bytes?: number[]; format: \"png\" }>; executeScript(js: string, args?: unknown[]): Promise<unknown>; executeScriptAsync(js: string, args?: unknown[]): Promise<unknown>; cookies(): Promise<object[]>; setCookie(c: { name: string; value: string; path?: string; domain?: string; secure?: boolean; httpOnly?: boolean; expiry?: number }): Promise<{ ok: true }>; deleteCookie(name: string): Promise<{ ok: true }>; deleteAllCookies(): Promise<{ ok: true }>; setImplicitWait(ms: number): Promise<{ ok: true }>; waitFor(by: string, value: string, opts?: { timeout?: number; visible?: boolean }): Promise<any>; windowHandles(): Promise<string[]>; currentWindow(): Promise<string>; switchToWindow(handle: string): Promise<{ ok: true }>; newWindow(type?: \"tab\" | \"window\"): Promise<{ handle: string; type: string }>; closeWindow(): Promise<string[]>; switchToFrame(target: number | object): Promise<{ ok: true }>; switchToParentFrame(): Promise<{ ok: true }>; switchToDefaultContent(): Promise<{ ok: true }>; acceptAlert(): Promise<{ ok: true }>; dismissAlert(): Promise<{ ok: true }>; alertText(): Promise<string>; sendAlertText(text: string): Promise<{ ok: true }>; maximize(): Promise<{ x: number; y: number; width: number; height: number }>; minimize(): Promise<{ x: number; y: number; width: number; height: number }>; fullscreen(): Promise<{ x: number; y: number; width: number; height: number }>; setWindowRect(rect: { width?: number; height?: number; x?: number; y?: number }): Promise<{ x: number; y: number; width: number; height: number }>; getWindowRect(): Promise<{ x: number; y: number; width: number; height: number }>; hover(el: object): Promise<{ ok: true }>; dragAndDrop(src: object, dst: object): Promise<{ ok: true }>; keyChord(...keys: string[]): Promise<{ ok: true }>; performActions(sequence: unknown[]): Promise<{ ok: true }>; releaseActions(): Promise<{ ok: true }>; quit(): Promise<{ closed: true }> }>",
			Returns:    "Promise resolving to a session handle with methods: get(url), url(), title(), back(), forward(), refresh(), find(by, value) → element handle, findAll(by, value) → element handle[], source(), screenshot(path?), executeScript(js, args?), executeScriptAsync(js, args?), cookies(), setCookie(c), deleteCookie(name), deleteAllCookies(), setImplicitWait(ms), waitFor(by, value, opts?), quit(). Phase 2 session methods: windowHandles(), currentWindow(), switchToWindow(handle), newWindow(type?), closeWindow(), switchToFrame(indexOrElement), switchToParentFrame(), switchToDefaultContent(), acceptAlert(), dismissAlert(), alertText(), sendAlertText(text), maximize(), minimize(), fullscreen(), setWindowRect({width?,height?,x?,y?}), getWindowRect(), hover(el), dragAndDrop(src,dst), keyChord(...keys), performActions(sequence), releaseActions(). Element handles also expose hover() and dragTo(target) and carry an elementId string. executeScript/executeScriptAsync return element handles when the script returns an element (or a top-level array of elements). Locator strategies: css, xpath, id, name, tag, className, linkText, partialLinkText.",
			Errors:     "Throws if no url is given and the driver binary for the selected browser is not on PATH; if dialing the driver fails (driver not running, wrong port); or if the browser launch fails. Subsequent session method calls throw if the session is already closed.",
			Example: `if (!services.webdriver.available) {
  runtime.log("no driver on PATH — skip");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get("data:text/html,<title>hi</title><h1 id=h>Hello</h1>");
    runtime.log("title:", await d.title());
    const el = await d.find("id", "h");
    runtime.log("text:", await el.text());
    await d.executeScript("return 1+2", []);
  } finally {
    await d.quit();
  }
}`,
		},

		// typst namespace docs
		"typst.available": {
			Summary:    "True when the typst CLI is on PATH. Sync boolean, resolved once per Run. Gate calls on this — every other typst binding throws a clean error when the CLI is absent.",
			ReturnType: "boolean",
			Returns:    "boolean — true if `typst` is found on PATH.",
			Errors:     "Never throws.",
			Example: `if (!services.typst.available) {
  runtime.log("install typst (brew install typst) first");
}`,
		},
		"typst.version": {
			Summary:    "The typst CLI version string (from `typst --version`).",
			ReturnType: "Promise<string>",
			Returns:    "Promise<string> — the trimmed version line reported by `typst --version` (e.g. \"typst 0.12.0 (…)\").",
			Errors:     "Throws if typst is not on PATH or on timeout / context cancellation.",
			Example:    `runtime.log(await services.typst.version());`,
		},
		"typst.fonts": {
			Summary:    "List the font families typst can see (from `typst fonts`), de-duplicated and sorted.",
			ReturnType: "Promise<string[]>",
			Returns:    "Promise<string[]> — sorted, unique font family names available to typst.",
			Errors:     "Throws if typst is not on PATH or on timeout / context cancellation.",
			Example: `const families = await services.typst.fonts();
runtime.log("fonts:", families.length);`,
		},
		"typst.compile": {
			Summary: "Compile a Typst document to PDF/PNG/SVG. Provide exactly one of `input` (a .typ path) or `source` (inline Typst). With no `output`, a PDF is compiled to a temp file and returned as bytes (PDF only); with an `output` path the result is written there and `format` is inferred from the extension (png/svg require an output path).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ input?: string, source?: string, output?: string, format?: \"pdf\" | \"png\" | \"svg\", root?: string, inputs?: Record<string, string>, ppi?: number, fontPaths?: string[], timeout?: number }", Desc: "Provide exactly one of input (a path to a .typ file) or source (inline Typst markup) — passing both, or neither, throws. output is the destination path; when omitted a PDF is produced in a temp dir and returned as bytes (PDF only — png/svg require an output path). format (pdf|png|svg) is inferred from output's extension when omitted, defaulting to pdf when there is no output. root sets the project root for absolute imports. inputs are sys.inputs key/value pairs passed as --input k=v (deterministic order). ppi sets PNG resolution (png only). fontPaths adds --font-path search dirs. timeout in ms (default 60000). Inline source caveat: source is written to a temp main.typ, so relative imports/reads resolve against that temp dir, not your cwd — use input (or root) when the document imports or reads sibling files."},
			},
			ReturnType: "Promise<{ format: string; bytes?: Uint8Array; path?: string }>",
			Returns:    "Promise<{ format, bytes?, path? }> — format echoes the chosen format. With no output: { format, bytes } where bytes is the compiled PDF as a Uint8Array. With an output path: { format, path } where path is the written file.",
			Errors:     "Throws if neither or both of input/source are given; if format is not pdf/png/svg; if png/svg is requested without an output path; if the output extension can't be mapped to a format; if typst is not on PATH; if typst exits non-zero (the trimmed stderr is included); or on timeout / context cancellation.",
			Example: `// Inline source → PDF bytes.
const pdf = await services.typst.compile({ source: "= Hi\nBody." });
runtime.log(pdf.format, pdf.bytes.length);

// .typ file → PNG on disk.
await services.typst.compile({ input: "report.typ", output: "/tmp/report.png", ppi: 144 });`,
		},
		"typst.query": {
			Summary: "Query a compiled Typst document for elements matching a selector and return the result as parsed JSON. Provide exactly one of `input` or `source`; `selector` is required.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ selector: string, input?: string, source?: string, field?: string, one?: boolean, root?: string, inputs?: Record<string, string>, timeout?: number }", Desc: "selector is required — a Typst query selector (e.g. \"<label>\", \"heading\", \"figure\"). Provide exactly one of input (a .typ path) or source (inline Typst). field extracts a single field from each match (--field). one returns just the first match instead of an array (--one). root sets the project root; inputs are --input k=v sys.inputs pairs; timeout in ms (default 60000). Same inline-source caveat as compile: source is written to a temp file, so relative imports/reads resolve there."},
			},
			ReturnType: "Promise<unknown>",
			Returns:    "Promise<unknown> — the parsed JSON typst emits: an array of matched elements (or matched field values when field is set), or a single value when one is set.",
			Errors:     "Throws if selector is missing; if neither or both of input/source are given; if typst is not on PATH; if typst exits non-zero (the trimmed stderr is included); if the output isn't valid JSON; or on timeout / context cancellation.",
			Example: `const v = await services.typst.query({
  source: "#metadata(42) <answer>",
  selector: "<answer>", field: "value", one: true,
});
runtime.log(v); // 42`,
		},
	}
}
