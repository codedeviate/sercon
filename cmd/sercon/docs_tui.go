package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func tuiDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"layout": {
			Summary: "Declare the pane layout for this Run. Tree nodes: { name, title?, weight? } (leaf), { rows: [...], weight? } (vertical split), { cols: [...], weight? } (horizontal split). Throws on duplicate names, empty rows/cols, unknown keys, or under --watch.",
			Params: []scriptengine.Param{
				{Name: "tree", Type: "{ name: string; title?: string; weight?: number } | { rows: object[]; weight?: number } | { cols: object[]; weight?: number }", Desc: "The root layout node. Exactly one of name / rows / cols must be set per node. A leaf (name) becomes a bordered pane addressable via tui.pane(name); name must be a non-empty string and unique across the whole tree. rows stacks children top-to-bottom; cols places them side-by-side; both arrays must be non-empty. weight (positive integer, default 1) sets the child's proportional share of its parent's space. title (string, leaf only) seeds the pane's border caption. Any other key is rejected. The tree is realised over the full terminal as a tview Flex when stdout is a TTY; otherwise it falls back to prefixed-line output."},
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
	}
}
