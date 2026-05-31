package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func tuiDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"layout": {Summary: "Declare the pane layout for this Run. Tree nodes: { name, title?, weight? } (leaf), { rows: [...], weight? } (vertical split), { cols: [...], weight? } (horizontal split). Throws on duplicate names, empty rows/cols, unknown keys, or under --watch."},
		"pane":   {Summary: "Return a Pane handle for a declared pane. Throws if the name wasn't in the layout. Handle methods: write(text), writeln(text), clear(), title(text). services.exec.shell({pane}) streams subprocess I/O into a pane."},
	}
}
