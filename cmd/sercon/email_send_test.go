package main

import (
	"strings"
	"testing"
)

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
	if !strings.Contains(s, `filename="data.bin"`) {
		t.Errorf(`expected filename="data.bin"`)
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
