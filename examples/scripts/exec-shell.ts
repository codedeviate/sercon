// Demonstrates services.exec.shell — generic subprocess runner. Non-zero exits
// surface as exitCode + success:false (no throw). Spawn failures, timeouts,
// and context cancellation throw.

// String cmd → routes through the host shell so pipes / redirects / globs
// work as typed at the prompt.
runtime.log("=== shell metacharacters ===");
const piped = await services.exec.shell("echo 'one two three' | tr ' ' ',' | wc -c");
runtime.log("output:", piped.stdout.trim(), "exit:", piped.exitCode);

// Array cmd → argv directly, no shell. Use this when args could be
// re-interpreted by the shell.
runtime.log("");
runtime.log("=== argv form (no shell expansion) ===");
const literal = await services.exec.shell(["/bin/echo", "literal *"]);
runtime.log("output:", literal.stdout.trim());

// stdin pipe
runtime.log("");
runtime.log("=== stdin ===");
const fed = await services.exec.shell(["/usr/bin/tr", "a-z", "A-Z"], {
  stdin: "hello, sercon!\n",
});
runtime.log("uppered:", fed.stdout.trim());

// Custom env var, custom cwd.
runtime.log("");
runtime.log("=== env + cwd ===");
const withEnv = await services.exec.shell("echo \"$GREETING from $(pwd)\"", {
  env: { GREETING: "hi" },
  cwd: "/tmp",
});
runtime.log(withEnv.stdout.trim());

// Non-zero exit — no throw, surfaced via exitCode.
runtime.log("");
runtime.log("=== non-zero exit ===");
const failed = await services.exec.shell("exit 3");
runtime.log("exit:", failed.exitCode, "success:", failed.success);

// Timeout — throws.
runtime.log("");
runtime.log("=== timeout ===");
try {
  await services.exec.shell("sleep 5", { timeout: 200 });
} catch (e) {
  runtime.log("caught timeout:", String(e).slice(0, 70) + "…");
}
