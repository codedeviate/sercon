// runtime.termSize — terminal columns/rows for scripts that format their own
// output (tables, progress bars, banners) outside the full-screen `tui`.
const size = runtime.termSize();
runtime.assert.ok(size.columns > 0 && size.rows > 0, "positive dimensions");
runtime.assert.equal(typeof size.tty, "boolean", "tty flag present");

// Draw a rule sized to the terminal — under `make demo` (non-TTY) this uses the
// 80-column fallback and reports tty:false.
runtime.log("=".repeat(Math.min(size.columns, 40)));
runtime.log(`terminal: ${size.columns}x${size.rows} (tty=${size.tty})`);
