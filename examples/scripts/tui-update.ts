// tui-update.ts — demonstrates api.ui.tui: a 3-pane layout where the
// orchestrator logs to one pane and two parallel subprocesses stream
// their output into the other two.
//
// Run manually:
//   ./sercon examples/scripts/tui-update.ts
//
// Tab / Shift-Tab cycles focus; PgUp/PgDn or arrow keys scroll the
// focused pane. Ctrl-C aborts the script.

api.ui.tui.layout({
  rows: [
    { name: "log", weight: 1, title: "Orchestrator" },
    {
      cols: [
        { name: "first", title: "first" },
        { name: "second", title: "second" },
      ],
      weight: 2,
    },
  ],
});

const log = api.ui.tui.pane("log");
const first = api.ui.tui.pane("first");
const second = api.ui.tui.pane("second");

log.writeln("Starting two parallel jobs…");

// We use printf so the demo works in CI / pipes too (each printf is a
// quick syscall; no real network or package-manager dependency).
const a = api.tools.exec.shell(
  ["/bin/sh", "-c", "for i in 1 2 3 4 5; do printf 'first  step %s\\n' \"$i\"; sleep 0.1; done"],
  { pane: first },
);
const b = api.tools.exec.shell(
  ["/bin/sh", "-c", "for i in 1 2 3 4 5; do printf 'second step %s\\n' \"$i\"; sleep 0.1; done"],
  { pane: second },
);

const [ra, rb] = await Promise.all([a, b]);
log.writeln(`first  finished: exit ${ra.exitCode}`);
log.writeln(`second finished: exit ${rb.exitCode}`);
log.writeln("Done. Press Ctrl-C to exit.");
