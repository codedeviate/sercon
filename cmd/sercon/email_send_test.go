package main

import (
	"mime"
	"strings"
	"testing"
)

// extractHeaderValue pulls the raw value of a single-line header out of a
// composed MIME message, e.g. extractHeaderValue(s, "Content-Disposition").
// Composed messages can have the same header name appear multiple times
// (top-level multipart Content-Type, per-part Content-Type, ...); the last
// occurrence is always the one belonging to the trailing attachment part,
// which is what these tests care about.
func extractHeaderValue(t *testing.T, s, name string) string {
	t.Helper()
	prefix := name + ": "
	idx := strings.LastIndex(s, prefix)
	if idx == -1 {
		t.Fatalf("header %q not found in:\n%s", name, s)
	}
	rest := s[idx+len(prefix):]
	end := strings.IndexAny(rest, "\r\n")
	if end == -1 {
		t.Fatalf("header %q has no line terminator in:\n%s", name, s)
	}
	return rest[:end]
}

func TestComposeMIME_PlainText(t *testing.T) {
	body, err := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "Content-Type: text/plain; charset=utf-8") {
		t.Errorf("expected text/plain Content-Type; got:\n%s", s)
	}
	if !strings.Contains(s, "hello") {
		t.Errorf("body content missing")
	}
	if strings.Contains(s, "multipart") {
		t.Errorf("unexpected multipart in plain-text-only send")
	}
}

func TestComposeMIME_TextAndHTML(t *testing.T) {
	body, _ := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi",
		body: "plain", html: "<p>html</p>",
	})
	s := string(body)
	if !strings.Contains(s, "multipart/alternative") {
		t.Errorf("expected multipart/alternative; got:\n%s", s)
	}
	if !strings.Contains(s, "plain") || !strings.Contains(s, "<p>html</p>") {
		t.Errorf("both parts must appear in the body")
	}
}

func TestComposeMIME_WithAttachment(t *testing.T) {
	body, _ := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
		attachments: []sendAttachment{
			{Filename: "data.bin", ContentType: "application/octet-stream", Bytes: []byte("ABCDEF")},
		},
	})
	s := string(body)
	if !strings.Contains(s, "multipart/mixed") {
		t.Errorf("expected multipart/mixed; got:\n%s", s)
	}
	// mime.FormatMediaType leaves simple tokens like "data.bin" unquoted
	// (RFC 2045 doesn't require quoting when there are no special chars),
	// so assert semantic equivalence via round-trip parsing rather than an
	// exact quoted-string match.
	disp := extractHeaderValue(t, s, "Content-Disposition")
	_, params, err := mime.ParseMediaType(disp)
	if err != nil {
		t.Fatalf("Content-Disposition %q did not parse: %v", disp, err)
	}
	if params["filename"] != "data.bin" {
		t.Errorf("expected filename param %q, got %q (header: %q)", "data.bin", params["filename"], disp)
	}
	if !strings.Contains(s, "Content-Transfer-Encoding: base64") {
		t.Errorf("expected base64 transfer encoding for attachment")
	}
}

func TestComposeMIME_AttachmentHeaderInjection(t *testing.T) {
	body, err := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
		attachments: []sendAttachment{
			{
				Filename:    "evil\r\nBcc: attacker@evil.com",
				ContentType: "application/octet-stream\r\nBcc: attacker@evil.com",
				Bytes:       []byte("ABCDEF"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	// The real injection signal is "Bcc:" landing on its own header line
	// (preceded by a line break) — a bare substring match doesn't prove
	// anything since the sanitized filename still contains the literal
	// text "Bcc: attacker@evil.com", just glued onto the same line.
	if strings.Contains(s, "\r\nBcc") || strings.Contains(s, "\nBcc") {
		t.Errorf("injected header line survived sanitization; got:\n%s", s)
	}
}

// TestComposeMIME_AttachmentFilenameQuoteInjection covers a filename
// containing a literal `"` and `;`, which (before RFC-encoding via
// mime.FormatMediaType) could break out of the quoted filename="..."
// parameter and smuggle in an extra bogus parameter.
func TestComposeMIME_AttachmentFilenameQuoteInjection(t *testing.T) {
	malicious := `evil".pdf"; x="y`
	body, err := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
		attachments: []sendAttachment{
			{Filename: malicious, ContentType: "application/octet-stream", Bytes: []byte("ABCDEF")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	disp := extractHeaderValue(t, s, "Content-Disposition")
	_, params, err := mime.ParseMediaType(disp)
	if err != nil {
		t.Fatalf("Content-Disposition %q did not parse (injection broke the header): %v", disp, err)
	}
	if _, injected := params["x"]; injected {
		t.Errorf("malicious param %q survived as a real parameter; header: %q", "x", disp)
	}
	if params["filename"] != malicious {
		t.Errorf("expected filename param to round-trip to %q, got %q (header: %q)", malicious, params["filename"], disp)
	}
}

// TestComposeMIME_AttachmentFilenameNonASCII covers RFC 2231 encoding of a
// non-ASCII filename so it survives as a well-formed, round-trippable
// parameter instead of raw bytes dropped into a quoted string.
func TestComposeMIME_AttachmentFilenameNonASCII(t *testing.T) {
	original := "résumé 日本語.pdf"
	body, err := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
		attachments: []sendAttachment{
			{Filename: original, ContentType: "application/pdf", Bytes: []byte("ABCDEF")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	disp := extractHeaderValue(t, s, "Content-Disposition")
	_, params, err := mime.ParseMediaType(disp)
	if err != nil {
		t.Fatalf("Content-Disposition %q did not parse: %v", disp, err)
	}
	if params["filename"] != original {
		t.Errorf("expected filename param to round-trip to %q, got %q (header: %q)", original, params["filename"], disp)
	}
}

// TestComposeMIME_AttachmentContentTypeInjection covers a contentType
// value containing an injected `;`/`"` (contained, not smuggled as a new
// parameter) and a garbage contentType (falls back to
// application/octet-stream).
func TestComposeMIME_AttachmentContentTypeInjection(t *testing.T) {
	t.Run("injected parameter is contained", func(t *testing.T) {
		body, err := composeMIME(sendOpts{
			to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
			attachments: []sendAttachment{
				{Filename: "data.bin", ContentType: `application/octet-stream"; evil="x`, Bytes: []byte("ABCDEF")},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)
		ct := extractHeaderValue(t, s, "Content-Type")
		mt, params, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatalf("Content-Type %q did not parse (injection broke the header): %v", ct, err)
		}
		if mt != "application/octet-stream" {
			t.Errorf(`expected media type "application/octet-stream", got %q (header: %q)`, mt, ct)
		}
		if _, injected := params["evil"]; injected {
			t.Errorf("malicious param %q survived as a real parameter; header: %q", "evil", ct)
		}
	})

	t.Run("garbage content type falls back to octet-stream", func(t *testing.T) {
		body, err := composeMIME(sendOpts{
			to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
			attachments: []sendAttachment{
				{Filename: "data.bin", ContentType: "not a valid content type; ;;", Bytes: []byte("ABCDEF")},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		s := string(body)
		ct := extractHeaderValue(t, s, "Content-Type")
		mt, _, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatalf("Content-Type %q did not parse: %v", ct, err)
		}
		if mt != "application/octet-stream" {
			t.Errorf(`expected fallback media type "application/octet-stream", got %q`, mt)
		}
	})
}

// TestComposeMIME_AttachmentNormalUnchanged covers the no-regression case:
// a normal filename + normal contentType must still produce a valid,
// semantically-equivalent header (RFC-encoding is a no-op for plain ASCII
// tokens).
func TestComposeMIME_AttachmentNormalUnchanged(t *testing.T) {
	body, err := composeMIME(sendOpts{
		to: []string{"a@x.com"}, from: "b@y.com", subject: "hi", body: "see attached",
		attachments: []sendAttachment{
			{Filename: "report.pdf", ContentType: "application/pdf", Bytes: []byte("ABCDEF")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)

	disp := extractHeaderValue(t, s, "Content-Disposition")
	dt, dparams, err := mime.ParseMediaType(disp)
	if err != nil {
		t.Fatalf("Content-Disposition %q did not parse: %v", disp, err)
	}
	if dt != "attachment" || dparams["filename"] != "report.pdf" {
		t.Errorf("expected attachment; filename=report.pdf, got type=%q params=%v (header: %q)", dt, dparams, disp)
	}

	ct := extractHeaderValue(t, s, "Content-Type")
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("Content-Type %q did not parse: %v", ct, err)
	}
	if mt != "application/pdf" {
		t.Errorf(`expected media type "application/pdf", got %q`, mt)
	}
}
