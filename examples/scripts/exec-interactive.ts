// services.exec.interactive — run a child wired to sercon's own terminal.
//
// Unlike services.exec.shell (which captures stdout/stderr and only feeds a
// fixed string to stdin), interactive() inherits stdin/stdout/stderr so the
// child owns the terminal: -it containers, ssh, interactive REPLs, pagers, and
// full-screen TUIs all work. On Unix with a real TTY it allocates a pty and
// switches to raw mode (restored on exit); non-TTY or Windows inherits the
// handles. Nothing is captured — you get { exitCode, success, durationMs }.
//
// TTY-only demo: run it yourself in a real terminal. It is intentionally NOT in
// `make demo` (like hang.ts) because it needs an interactive terminal.
//
//   ./sercon examples/scripts/exec-interactive.ts
//
// This launches an interactive shell; type a few commands, then `exit`.

const shell = runtime.env.get("SHELL") ?? "/bin/sh";
runtime.log(`launching ${shell} — type 'exit' to return...`);

const r = await services.exec.interactive([shell, "-i"]);

runtime.log(`shell exited: code=${r.exitCode} success=${r.success} durationMs=${r.durationMs}`);
