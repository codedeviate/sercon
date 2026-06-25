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
