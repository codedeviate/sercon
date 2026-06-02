#!/usr/bin/env sercon
// PTY: run a tty-gated command under a pseudo-terminal so its color reaches
// the pane. Compare with the same command without pty (monochrome).
// Unix only; on Windows pty falls back to a pipe (no color).

tui.layout({
  rows: [
    { name: "pty",  title: "pty: true (colorized)" },
    { name: "pipe", title: "no pty (monochrome)" },
    { name: "log",  weight: 1 },
  ],
});

// A self-contained tty check: print a green word only when stdout is a tty.
const cmd =
  `sh -c 'if [ -t 1 ]; then printf "\\033[32mTTY: colorized\\033[0m\\n"; else printf "no tty\\n"; fi'`;

await services.exec.shell(cmd, { pane: tui.pane("pty"),  pty: true });
await services.exec.shell(cmd, { pane: tui.pane("pipe") });           // no pty

tui.pane("log").writeln("Top pane ran under a pty (sees a tty); bottom did not.");
