package scriptengine

import (
	"bytes"
	"strings"
	"testing"
)

// A rest parameter (encoded with a leading "..." on the type) must render
// as TS `...name: type`, not the invalid `name: ...type`.
func TestSigFromParams_RestParam(t *testing.T) {
	got := sigFromParams([]Param{{Name: "args", Type: "...unknown[]"}}, "void")
	want := "(...args: unknown[]): void"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Regular params are unchanged, including the optional `?`.
func TestSigFromParams_RegularAndOptional(t *testing.T) {
	got := sigFromParams([]Param{
		{Name: "url", Type: "string"},
		{Name: "opts", Type: "object", Optional: true},
	}, "Promise<number>")
	want := "(url: string, opts?: object): Promise<number>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A doc string containing "*/" (e.g. "RS*/PS*/ES*") must be escaped so it
// does not terminate the JSDoc block early and turn the rest of the file
// into garbage. After escaping, the only "*/" in the output is the block
// terminator.
func TestWriteJSDoc_EscapesCommentTerminator(t *testing.T) {
	for _, doc := range []string{
		"keys for RS*/PS*/ES* algorithms",             // single-line path
		"line one with media:*/dc:* tokens\nline two", // multi-line path
	} {
		var buf bytes.Buffer
		w := &errWriter{w: &buf}
		writeJSDoc(w, doc, 0)
		out := buf.String()
		if n := strings.Count(out, "*/"); n != 1 {
			t.Errorf("doc %q: expected exactly one */ (the terminator), got %d in:\n%s", doc, n, out)
		}
	}
}

// The structured-member JSDoc writer must escape "*/" in the summary,
// @param descriptions, and @returns text too.
func TestWriteMemberJSDoc_EscapesCommentTerminator(t *testing.T) {
	var buf bytes.Buffer
	w := &errWriter{w: &buf}
	doc := MemberDoc{
		Summary: "signs with RS*/PS*/ES*",
		Params:  []Param{{Name: "secret", Desc: "PEM for RS*/PS*"}},
		Returns: "token/*not*/really",
	}
	writeMemberJSDoc(w, doc, 0)
	out := buf.String()
	if n := strings.Count(out, "*/"); n != 1 {
		t.Errorf("expected exactly one */ (the terminator), got %d in:\n%s", n, out)
	}
}
