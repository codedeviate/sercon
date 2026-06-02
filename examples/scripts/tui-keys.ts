#!/usr/bin/env sercon
// Demonstrates autoscroll (panes follow the tail) and tui.waitKey().
// TTY-only: under a non-TTY (make demo / CI) this falls back to prefixed
// lines and waitKey rejects, so we guard the wait.

tui.layout({
  mouse: true,
  rows: [
    { name: "log", title: "Log" },
    { name: "out", title: "Output", autoscroll: false, wrap: "off", color: false },
  ],
});

const log = tui.pane("log");
const out = tui.pane("out");

for (let i = 0; i < 50; i++) log.writeln("log line " + i);
out.writeln("this pane stays pinned at the top (autoscroll:false)");

tui.onKey((k) => log.writeln("key: " + (k.name === "Rune" ? k.rune : k.name)));

log.writeln("Press any key to close...");
try {
  await tui.waitKey();
} catch {
  // non-TTY: nothing to wait for.
}
