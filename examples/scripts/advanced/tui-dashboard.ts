// tui-dashboard.ts — a live multi-pane TUI dashboard.
//
// Run manually:
//   ./sercon examples/scripts/advanced/tui-dashboard.ts
//
// Panes:
//   header  — static title bar.
//   status  — a few periodic runtime.time ticks (5 iterations, ~0.4 s apart).
//   output  — streams a bounded subprocess (seq 1 10 or a portable echo loop).
//
// CRITICAL: the script performs a BOUNDED number of updates and exits on its
// own — it does NOT wait on keypresses and does NOT loop forever. In non-TTY
// (CI / pipes) tui.layout falls back to prefixed plain-text lines; the same
// bounded loop runs and the script exits 0.
//
// Tab / Shift-Tab cycles pane focus; PgUp/PgDn / arrows scroll; Ctrl-C aborts.

tui.layout({
  rows: [
    { name: "header", weight: 1, title: "Dashboard" },
    {
      cols: [
        { name: "status", weight: 1, title: "Status" },
        { name: "output", weight: 2, title: "Subprocess" },
      ],
      weight: 4,
    },
  ],
});

const header = tui.pane("header");
const status = tui.pane("status");
const output = tui.pane("output");

header.writeln("sercon tui-dashboard — bounded demo (5 ticks × 400 ms)");
status.writeln("Starting…");

// Stream a finite subprocess into the output pane.
// `seq 1 10` is POSIX-safe; fall back to a sh loop if seq is absent.
const subProc = services.exec.shell(
  ["/bin/sh", "-c",
   "for i in 1 2 3 4 5 6 7 8 9 10; do printf 'line %s\\n' \"$i\"; done"],
  { pane: output },
);

// Emit 5 status ticks while the subprocess runs.
const TICKS = 5;
for (let i = 1; i <= TICKS; i++) {
  const ts = runtime.time.format(runtime.time.nowMs(), "%H:%M:%S");
  status.writeln(`[${ts}] tick ${i}/${TICKS}`);
  await runtime.time.sleep(400);
}

// Wait for the subprocess to finish.
const result = await subProc;
status.writeln(`subprocess exit ${result.exitCode}`);
header.writeln("Done.");
