package scriptengine

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteReference_DocumentedMember(t *testing.T) {
	e := New(Options{})
	if err := e.RegisterNamespace("crypto", map[string]any{
		"sha256": func(string) string { return "" },
	}); err != nil {
		t.Fatalf("RegisterNamespace: %v", err)
	}
	e.SetMemberDocsStructured("crypto", map[string]MemberDoc{
		"sha256": {
			Summary: "SHA-256 hex digest of a UTF-8 input.",
			Params: []Param{
				{Name: "input", Type: "string", Desc: "UTF-8 input to hash"},
			},
			ReturnType: "string",
			Returns:    "hex digest",
			Errors:     "throws if input is not a string",
			Example:    `crypto.sha256("hello")`,
		},
	})

	var buf bytes.Buffer
	if err := e.WriteReference(&buf); err != nil {
		t.Fatalf("WriteReference: %v", err)
	}
	out := buf.String()

	wants := []string{
		"### crypto",
		"#### crypto.sha256",
		"sha256(input: string): string",
		"SHA-256 hex digest of a UTF-8 input.",
		"- `input` *(string)* — UTF-8 input to hash",
		"**Returns:** hex digest",
		"**Throws:** throws if input is not a string",
		"```ts\ncrypto.sha256(\"hello\")\n```",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}
}

func TestWriteReference_SummaryOnly(t *testing.T) {
	e := New(Options{})
	if err := e.RegisterNamespace("text", map[string]any{
		"upper": func(string) string { return "" },
	}); err != nil {
		t.Fatalf("RegisterNamespace: %v", err)
	}
	e.SetMemberDocsStructured("text", map[string]MemberDoc{
		"upper": {Summary: "Uppercase a string."},
	})

	var buf bytes.Buffer
	if err := e.WriteReference(&buf); err != nil {
		t.Fatalf("WriteReference: %v", err)
	}
	out := buf.String()

	for _, w := range []string{"#### text.upper", "Uppercase a string.", "upper(...args: unknown[])"} {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q\n---\n%s", w, out)
		}
	}
	for _, no := range []string{"**Parameters**", "**Returns:**", "**Throws:**", "```ts"} {
		if strings.Contains(out, no) {
			t.Errorf("summary-only output should not contain %q\n---\n%s", no, out)
		}
	}
}

func TestWriteReferenceNumbered(t *testing.T) {
	e := New(Options{})
	_ = e.RegisterNamespace("crypto", map[string]any{"sha256": func(string) string { return "" }})
	_ = e.RegisterNamespace("text", map[string]any{"upper": func(string) string { return "" }})
	e.SetMemberDocsStructured("crypto", map[string]MemberDoc{"sha256": {Summary: "SHA-256."}})
	e.SetMemberDocsStructured("text", map[string]MemberDoc{"upper": {Summary: "Uppercase."}})

	var buf bytes.Buffer
	if err := e.WriteReferenceNumbered(&buf, "17"); err != nil {
		t.Fatalf("WriteReferenceNumbered: %v", err)
	}
	out := buf.String()
	// crypto sorts before text → 17.1 / 17.2; members number within each.
	for _, w := range []string{"### 17.1 crypto", "#### 17.1.1 crypto.sha256", "### 17.2 text", "#### 17.2.1 text.upper"} {
		if !strings.Contains(out, w) {
			t.Errorf("numbered output missing %q\n---\n%s", w, out)
		}
	}

	// The unnumbered WriteReference must not carry section numbers.
	var plain bytes.Buffer
	if err := e.WriteReference(&plain); err != nil {
		t.Fatalf("WriteReference: %v", err)
	}
	if strings.Contains(plain.String(), "### 17.") {
		t.Errorf("WriteReference must be unnumbered:\n%s", plain.String())
	}
}

// TestWriteReference_OrphanChildren covers the runtime-handle case: a leaf
// member builds its real surface only at script-run time (like the
// cloud.<provider> callables), so the surface walk can't introspect its
// services/methods. Documented members nested under the leaf's dotted path
// must still be emitted — and numbered/levelled by their real dotted-path
// depth: the provider at H4/17.1.1, its service group at H5/17.1.1.1, and its
// methods at H6/17.1.1.1.N — a summary-only doc as a group heading, a
// signature-bearing doc as a full method entry, in sorted dotted-path order.
func TestWriteReference_OrphanChildren(t *testing.T) {
	e := New(Options{})
	if err := e.RegisterNamespace("cloud", map[string]any{
		"prov": func(...any) any { return nil }, // opaque runtime-handle factory
	}); err != nil {
		t.Fatalf("RegisterNamespace: %v", err)
	}
	e.SetMemberDocsStructured("cloud", map[string]MemberDoc{
		"prov":            {Summary: "Provider handle.", ReturnType: "{ svc(): { get(): Promise<unknown> } }"},
		"prov.svc":        {Summary: "A service group."},                                                 // container: heading + summary only
		"prov.svc.getOne": {Summary: "Fetch one.", ReturnType: "Promise<unknown>", Returns: "the thing"}, // method: full entry
		"prov.svc.putOne": {Summary: "Store one.", Params: []Param{{Name: "id", Type: "string"}}, Returns: "ok"},
	})

	var buf bytes.Buffer
	if err := e.WriteReferenceNumbered(&buf, "17"); err != nil {
		t.Fatalf("WriteReferenceNumbered: %v", err)
	}
	out := buf.String()

	// The provider leaf entry (walked, H4) carries its composite ReturnType; the
	// nested service group renders as a summary-only H5 heading; the nested
	// methods render as full H6 entries — numbers and heading levels both track
	// the dotted-path depth.
	for _, w := range []string{
		"#### 17.1.1 cloud.prov",
		"{ svc(): { get(): Promise<unknown> } }",
		"##### 17.1.1.1 cloud.prov.svc",
		"A service group.",
		"###### 17.1.1.1.1 cloud.prov.svc.getOne",
		"getOne(): Promise<unknown>",
		"**Returns:** the thing",
		"###### 17.1.1.1.2 cloud.prov.svc.putOne",
		"putOne(id: string): void",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("orphan-children output missing %q\n---\n%s", w, out)
		}
	}
	// The service-group container is summary-only: no signature fence directly
	// under its heading. Verify the heading is immediately followed by the
	// summary paragraph, not a fenced block.
	const svcHeading = "##### 17.1.1.1 cloud.prov.svc\n"
	if i := strings.Index(out, svcHeading); i >= 0 {
		if after := out[i+len(svcHeading):]; strings.HasPrefix(after, "\n```") {
			t.Errorf("service-group container must not emit a signature fence\n---\n%s", out)
		}
	}
}

// TestWriteReference_NoOrphansUnaffected guards byte-stability: a namespace
// whose leaves have no nested doc keys must emit exactly what it did before
// writeOrphanChildren existed — the orphan pass adds nothing.
func TestWriteReference_NoOrphansUnaffected(t *testing.T) {
	e := New(Options{})
	_ = e.RegisterNamespace("crypto", map[string]any{"sha256": func(string) string { return "" }})
	e.SetMemberDocsStructured("crypto", map[string]MemberDoc{
		"sha256": {Summary: "SHA-256.", Params: []Param{{Name: "in", Type: "string"}}, ReturnType: "string"},
	})
	var buf bytes.Buffer
	if err := e.WriteReference(&buf); err != nil {
		t.Fatalf("WriteReference: %v", err)
	}
	out := buf.String()
	// Exactly one member heading — no phantom entries from the orphan pass.
	if n := strings.Count(out, "#### "); n != 1 {
		t.Errorf("expected exactly 1 member heading, got %d\n---\n%s", n, out)
	}
}

func TestWriteReference_Deterministic(t *testing.T) {
	build := func() string {
		e := New(Options{})
		_ = e.RegisterNamespace("crypto", map[string]any{
			"sha256": func(string) string { return "" },
			"md5":    func(string) string { return "" },
		})
		_ = e.RegisterNamespace("text", map[string]any{
			"upper": func(string) string { return "" },
		})
		e.SetMemberDocsStructured("crypto", map[string]MemberDoc{
			"sha256": {Summary: "SHA-256.", Params: []Param{{Name: "in", Type: "string"}}, Returns: "hex"},
			"md5":    {Summary: "MD5."},
		})
		e.SetMemberDocsStructured("text", map[string]MemberDoc{
			"upper": {Summary: "Uppercase."},
		})
		var buf bytes.Buffer
		if err := e.WriteReference(&buf); err != nil {
			t.Fatalf("WriteReference: %v", err)
		}
		return buf.String()
	}
	if a, b := build(), build(); a != b {
		t.Errorf("WriteReference not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}
