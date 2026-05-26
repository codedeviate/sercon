// Demonstrates api.exec.shell — generic subprocess runner. Non-zero exits
// surface as exitCode + success:false (no throw). Spawn failures, timeouts,
// and context cancellation throw.

// String cmd → routes through the host shell so pipes / redirects / globs
// work as typed at the prompt.
api.log("=== shell metacharacters ===");
const piped = await api.exec.shell("echo 'one two three' | tr ' ' ',' | wc -c");
api.log("output:", piped.stdout.trim(), "exit:", piped.exitCode);

// Array cmd → argv directly, no shell. Use this when args could be
// re-interpreted by the shell.
api.log("");
api.log("=== argv form (no shell expansion) ===");
const literal = await api.exec.shell(["/bin/echo", "literal *"]);
api.log("output:", literal.stdout.trim());

// stdin pipe
api.log("");
api.log("=== stdin ===");
const fed = await api.exec.shell(["/usr/bin/tr", "a-z", "A-Z"], {
  stdin: "hello, sercon!\n",
});
api.log("uppered:", fed.stdout.trim());

// Custom env var, custom cwd.
api.log("");
api.log("=== env + cwd ===");
const withEnv = await api.exec.shell("echo \"$GREETING from $(pwd)\"", {
  env: { GREETING: "hi" },
  cwd: "/tmp",
});
api.log(withEnv.stdout.trim());

// Non-zero exit — no throw, surfaced via exitCode.
api.log("");
api.log("=== non-zero exit ===");
const failed = await api.exec.shell("exit 3");
api.log("exit:", failed.exitCode, "success:", failed.success);

// Timeout — throws.
api.log("");
api.log("=== timeout ===");
try {
  await api.exec.shell("sleep 5", { timeout: 200 });
} catch (e) {
  api.log("caught timeout:", String(e).slice(0, 70) + "…");
}
