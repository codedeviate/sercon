// Demonstrates api.tools.exec.shell — generic subprocess runner. Non-zero exits
// surface as exitCode + success:false (no throw). Spawn failures, timeouts,
// and context cancellation throw.

// String cmd → routes through the host shell so pipes / redirects / globs
// work as typed at the prompt.
api.runtime.log("=== shell metacharacters ===");
const piped = await api.tools.exec.shell("echo 'one two three' | tr ' ' ',' | wc -c");
api.runtime.log("output:", piped.stdout.trim(), "exit:", piped.exitCode);

// Array cmd → argv directly, no shell. Use this when args could be
// re-interpreted by the shell.
api.runtime.log("");
api.runtime.log("=== argv form (no shell expansion) ===");
const literal = await api.tools.exec.shell(["/bin/echo", "literal *"]);
api.runtime.log("output:", literal.stdout.trim());

// stdin pipe
api.runtime.log("");
api.runtime.log("=== stdin ===");
const fed = await api.tools.exec.shell(["/usr/bin/tr", "a-z", "A-Z"], {
  stdin: "hello, sercon!\n",
});
api.runtime.log("uppered:", fed.stdout.trim());

// Custom env var, custom cwd.
api.runtime.log("");
api.runtime.log("=== env + cwd ===");
const withEnv = await api.tools.exec.shell("echo \"$GREETING from $(pwd)\"", {
  env: { GREETING: "hi" },
  cwd: "/tmp",
});
api.runtime.log(withEnv.stdout.trim());

// Non-zero exit — no throw, surfaced via exitCode.
api.runtime.log("");
api.runtime.log("=== non-zero exit ===");
const failed = await api.tools.exec.shell("exit 3");
api.runtime.log("exit:", failed.exitCode, "success:", failed.success);

// Timeout — throws.
api.runtime.log("");
api.runtime.log("=== timeout ===");
try {
  await api.tools.exec.shell("sleep 5", { timeout: 200 });
} catch (e) {
  api.runtime.log("caught timeout:", String(e).slice(0, 70) + "…");
}
