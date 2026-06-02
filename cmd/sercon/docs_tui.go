package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func tuiDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"layout": {
			Summary: "Declare the pane layout for this Run. Tree nodes: { name, title?, weight?, autoscroll? } (leaf), { rows: [...], weight? } (vertical split), { cols: [...], weight? } (horizontal split). The root node also accepts { mouse?: boolean }. Throws on duplicate names, empty rows/cols, unknown keys, or under --watch.",
			Params: []scriptengine.Param{
				{Name: "tree", Type: "{ name: string; title?: string; weight?: number } | { rows: object[]; weight?: number } | { cols: object[]; weight?: number }", Desc: "The root layout node. Exactly one of name / rows / cols must be set per node. A leaf (name) becomes a bordered pane addressable via tui.pane(name); name must be a non-empty string and unique across the whole tree. rows stacks children top-to-bottom; cols places them side-by-side; both arrays must be non-empty. weight (positive integer, default 1) sets the child's proportional share of its parent's space. title (string, leaf only) seeds the pane's border caption. Any other key is rejected. The tree is realised over the full terminal as a tview Flex when stdout is a TTY; otherwise it falls back to prefixed-line output. autoscroll (boolean, leaf only, default true) controls whether the pane follows the tail as new lines arrive; set false to keep it pinned at the top. mouse (boolean, root only, default false) enables mouse-wheel scrolling of panes — at the cost of the terminal's native click-drag text selection (use Shift/Option+drag to select while mouse mode is on)."},
			},
			ReturnType: "void",
			Returns:    "void — installs the layout and brings up the UI (TTY) or the fallback line writer (non-TTY); the controller is torn down automatically at Run end.",
			Errors:     "Throws if called under --watch; if layout was already called this Run; if the tree argument is missing/null/undefined; if any node violates the structure rules (not an object, more than one of name/rows/cols, missing all three, empty rows/cols, unknown key, non-string or empty name, duplicate name, title on a non-leaf, or non-positive/non-integer weight) — the error includes the tree path (e.g. \"rows[1].cols[0]\"); or if the terminal screen / fallback writer fails to start.",
			Example: `tui.layout({ cols: [{ name: "log", title: "Log" }, { name: "out", weight: 2 }] });
tui.pane("log").writeln("started");`,
		},
		"pane": {
			Summary: "Return a Pane handle for a declared pane. Throws if the name wasn't in the layout. Handle methods: write(text), writeln(text), clear(), title(text). services.exec.shell({pane}) streams subprocess I/O into a pane.",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "The leaf name declared in the tui.layout tree."},
			},
			ReturnType: "{ write(text: string): void; writeln(text: string): void; clear(): void; title(text: string): void }",
			Returns:    "A Pane handle. write(text) appends text (subprocess ANSI SGR colors are translated to pane colors); writeln(text) appends text followed by a newline; clear() empties the pane (no-op in the non-TTY fallback); title(text) updates the pane's border caption (no-op in fallback). All methods return undefined and are safe to call from any callback. The handle can also be passed as the pane option to services.exec.shell to stream a subprocess's stdout/stderr live into the pane.",
			Errors:     "Throws if tui.layout has not been called yet this Run, or if name was not declared as a leaf in the layout (the message lists the available pane names).",
			Example: `const p = tui.pane("out");
p.title("Output");
p.writeln("hello");`,
		},
		"onKey": {
			Summary: "Register a callback invoked on every keypress (TTY mode) except Ctrl-C. Returns an unsubscribe function. No-op in non-TTY (fallback) mode.",
			Params: []scriptengine.Param{
				{Name: "handler", Type: "(key: { name: string; rune: string; ctrl: boolean; alt: boolean; shift: boolean }) => void", Desc: "Called for each keypress with a key descriptor. name is the tcell key name (\"Enter\", \"Up\", \"Tab\", \"Ctrl-A\", \"F1\", ...) or \"Rune\" for a printable character (rune holds it). For Ctrl+letter combinations the name carries the combo (e.g. \"Ctrl-A\") and the ctrl flag may be false, so check name for control keys. Built-in navigation (Tab focus, PgUp/PgDn/arrows/Home/End scroll) still runs in addition to the handler (coexist model). Ctrl-C always aborts the script and is never delivered."},
			},
			ReturnType: "() => void",
			Returns:    "An unsubscribe function; call it to stop receiving keys. In non-TTY mode onKey registers nothing and returns a no-op unsubscribe. Note: registering onKey alone does not keep the Run alive — keep an outstanding await (e.g. waitKey or a sleep) if the script should stay open.",
			Errors:     "Throws if called before tui.layout, or if the argument is not a function.",
			Example: `tui.layout({ rows: [{ name: "log" }] });
const off = tui.onKey((k) => { if (k.name === "Rune" && k.rune === "q") off(); });`,
		},
		"waitKey": {
			Summary:    "Resolve with the next keypress (TTY mode). One-shot — await again for the next key. Rejects in non-TTY (fallback) mode.",
			Params:     []scriptengine.Param{},
			ReturnType: "Promise<{ name: string; rune: string; ctrl: boolean; alt: boolean; shift: boolean }>",
			Returns:    "A Promise resolving to the next key descriptor (same shape as onKey's argument). Concurrent waitKey calls resolve FIFO — one keypress resolves the oldest pending call. While a waitKey is pending the TUI stays open (this is the idiomatic \"press any key to close\" hold). Ctrl-C aborts and is never delivered.",
			Errors:     "Rejects if called before tui.layout, or in non-TTY mode (no interactive terminal), or if the TUI is closed while waiting.",
			Example: `tui.layout({ rows: [{ name: "log" }] });
tui.pane("log").writeln("Done. Press any key to close.");
await tui.waitKey();`,
		},
	}
}
