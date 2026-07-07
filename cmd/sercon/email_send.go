package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// sendRejection records one RCPT TO that the server accepted us to attempt
// but then refused. Distinct from transport-level errors which throw.
type sendRejection struct {
	Address string `json:"address"`
	Reason  string `json:"reason"`
}

// sendResult is the resolved value of net.email.send.
type sendResult struct {
	Accepted []string        `json:"accepted"`
	Rejected []sendRejection `json:"rejected"`
}

// sendOpts mirrors the JS options object passed to net.email.send.
type sendOpts struct {
	to          []string
	from        string
	subject     string
	body        string
	html        string
	attachments []sendAttachment
	headers     map[string]string
	serverHost  string
	serverPort  int
	authUser    string
	authPass    string
	tlsMode     string // "starttls" | "tls" | "none"
	timeout     time.Duration
}

type sendAttachment struct {
	Filename    string
	ContentType string
	Bytes       []byte
}

// emailSend returns the AsyncBinding wired into emailNamespace.
func emailSend(vm *goja.Runtime, loop *eventloop.EventLoop) scriptengine.AsyncBinding {
	return scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (sendResult, error) {
		opts, err := parseSendOpts(vm, call)
		if err != nil {
			return sendResult{}, err
		}
		if opts.timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, opts.timeout)
			defer cancel()
		}
		return sendMail(ctx, opts)
	})
}

// parseSendOpts extracts the JS opts into sendOpts.
func parseSendOpts(vm *goja.Runtime, call goja.FunctionCall) (sendOpts, error) {
	opts := sendOpts{
		serverPort: 587,
		tlsMode:    "starttls",
		timeout:    30 * time.Second,
		headers:    map[string]string{},
	}
	if len(call.Arguments) == 0 {
		return opts, errors.New("net.email.send: options object required")
	}
	o := call.Argument(0).ToObject(vm)
	if o == nil {
		return opts, errors.New("net.email.send: options must be an object")
	}
	if v := o.Get("to"); v != nil {
		switch x := v.Export().(type) {
		case string:
			opts.to = []string{x}
		case []any:
			for _, e := range x {
				if s, ok := e.(string); ok {
					opts.to = append(opts.to, s)
				}
			}
		}
	}
	if len(opts.to) == 0 {
		return opts, errors.New("net.email.send: `to` is required (string or string[])")
	}
	if v := o.Get("from"); v != nil {
		opts.from = v.String()
	}
	if opts.from == "" {
		return opts, errors.New("net.email.send: `from` is required")
	}
	if v := o.Get("subject"); v != nil {
		opts.subject = v.String()
	}
	if v := o.Get("body"); v != nil {
		opts.body = v.String()
	}
	if v := o.Get("html"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		opts.html = v.String()
	}
	if v := o.Get("attachments"); v != nil && !goja.IsUndefined(v) {
		if arr, ok := v.Export().([]any); ok {
			for _, e := range arr {
				m, ok := e.(map[string]any)
				if !ok {
					continue
				}
				att := sendAttachment{}
				if s, ok := m["filename"].(string); ok {
					att.Filename = s
				}
				if s, ok := m["contentType"].(string); ok {
					att.ContentType = s
				}
				// Uint8Array exports as []byte; ArrayBuffer as goja.ArrayBuffer.
				// Accept both so callers aren't surprised by a silently-empty
				// attachment (matches jsArgToBytes's accepted types).
				switch b := m["bytes"].(type) {
				case []byte:
					att.Bytes = b
				case goja.ArrayBuffer:
					att.Bytes = b.Bytes()
				}
				if att.ContentType == "" {
					att.ContentType = "application/octet-stream"
				}
				opts.attachments = append(opts.attachments, att)
			}
		}
	}
	if v := o.Get("headers"); v != nil && !goja.IsUndefined(v) {
		if m, ok := v.Export().(map[string]any); ok {
			for k, val := range m {
				if s, ok := val.(string); ok {
					opts.headers[k] = s
				}
			}
		}
	}
	if v := o.Get("server"); v != nil && !goja.IsUndefined(v) {
		so := v.ToObject(vm)
		opts.serverHost = so.Get("host").String()
		if pv := so.Get("port"); pv != nil && !goja.IsUndefined(pv) {
			opts.serverPort = int(pv.ToInteger())
		}
		if av := so.Get("auth"); av != nil && !goja.IsUndefined(av) && !goja.IsNull(av) {
			ao := av.ToObject(vm)
			opts.authUser = ao.Get("username").String()
			opts.authPass = ao.Get("password").String()
		}
		if tv := so.Get("tls"); tv != nil && !goja.IsUndefined(tv) && !goja.IsNull(tv) {
			opts.tlsMode = tv.String()
		}
	}
	if opts.serverHost == "" {
		return opts, errors.New("net.email.send: `server.host` is required")
	}
	if v := o.Get("timeout"); v != nil && !goja.IsUndefined(v) {
		opts.timeout = time.Duration(v.ToInteger()) * time.Millisecond
	}
	return opts, nil
}

// sendMail is the Go-side worker. One TCP connection per call; one MAIL
// FROM, multiple RCPT TOs (per-recipient acceptance recorded), one DATA.
func sendMail(ctx context.Context, opts sendOpts) (sendResult, error) {
	addr := net.JoinHostPort(opts.serverHost, fmt.Sprintf("%d", opts.serverPort))
	var conn net.Conn
	var err error
	d := net.Dialer{Timeout: opts.timeout}
	switch opts.tlsMode {
	case "tls":
		// DialWithDialer (not tls.Dial) so opts.timeout caps the implicit-TLS
		// connect — tls.Dial takes no Dialer and would ignore the timeout.
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: opts.serverHost, MinVersion: tls.VersionTLS12})
	default:
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return sendResult{}, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, opts.serverHost)
	if err != nil {
		return sendResult{}, fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Hello("localhost"); err != nil {
		return sendResult{}, fmt.Errorf("HELO: %w", err)
	}

	if opts.tlsMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return sendResult{}, errors.New("STARTTLS requested but server doesn't advertise it")
		}
		if err := client.StartTLS(&tls.Config{ServerName: opts.serverHost, MinVersion: tls.VersionTLS12}); err != nil {
			return sendResult{}, fmt.Errorf("STARTTLS: %w", err)
		}
	}

	if opts.authUser != "" && opts.tlsMode != "none" {
		auth := smtp.PlainAuth("", opts.authUser, opts.authPass, opts.serverHost)
		if err := client.Auth(auth); err != nil {
			return sendResult{}, fmt.Errorf("AUTH: %w", err)
		}
	}

	if err := client.Mail(opts.from); err != nil {
		return sendResult{}, fmt.Errorf("MAIL FROM: %w", err)
	}

	result := sendResult{Accepted: []string{}, Rejected: []sendRejection{}}
	for _, rcpt := range opts.to {
		if err := client.Rcpt(rcpt); err != nil {
			result.Rejected = append(result.Rejected, sendRejection{Address: rcpt, Reason: err.Error()})
			continue
		}
		result.Accepted = append(result.Accepted, rcpt)
	}
	if len(result.Accepted) == 0 {
		_ = client.Quit()
		return result, nil
	}

	body, err := composeMIME(opts)
	if err != nil {
		return result, fmt.Errorf("compose MIME: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return result, fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		_ = w.Close()
		return result, fmt.Errorf("DATA write: %w", err)
	}
	if err := w.Close(); err != nil {
		return result, fmt.Errorf("DATA close: %w", err)
	}
	_ = client.Quit()
	return result, nil
}

// sanitizeHeaderValue strips CR/LF and other ASCII control characters from a
// value bound for a MIME header (top-level or per-part). Without this, a
// script-supplied From/To/Subject/custom-header/attachment-filename/
// attachment-contentType could inject arbitrary extra header lines (e.g. a
// crafted filename containing "\r\nBcc: attacker@evil.com").
func sanitizeHeaderValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || (r < 0x20 && r != '\t') || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// attachmentContentDisposition builds an RFC-compliant
// `Content-Disposition: attachment; filename=...` header value for an
// attachment filename. mime.FormatMediaType quotes/escapes any embedded `"`
// so a filename can't break out of the filename="..." parameter and inject
// a second bogus parameter, and RFC-2231-encodes non-ASCII filenames
// (filename*=utf-8''...). sanitizeHeaderValue runs first as a
// belt-and-suspenders pass — FormatMediaType already refuses to encode
// values containing CR/LF, but stripping them upfront keeps that guarantee
// independent of the mime package's internals. If FormatMediaType can't
// format the value at all (e.g. it still round-trips to something
// pathological), fall back to a bare "attachment" with no filename rather
// than emit a malformed or attacker-controlled header.
func attachmentContentDisposition(filename string) string {
	clean := sanitizeHeaderValue(filename)
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": clean}); v != "" {
		return "Content-Disposition: " + v
	}
	return "Content-Disposition: attachment"
}

// attachmentContentType validates/normalizes a script-supplied attachment
// content type via mime.ParseMediaType + mime.FormatMediaType so an
// injected `;` or `"` can't smuggle in extra Content-Type parameters and so
// a malformed value can't produce a broken header. A garbage or empty
// content type (ParseMediaType error, or FormatMediaType refusing to
// re-encode the parsed result) falls back to application/octet-stream.
// Legitimate values like "application/pdf" or "text/plain; charset=utf-8"
// round-trip unchanged (parameter quoting style aside).
func attachmentContentType(contentType string) string {
	const fallback = "application/octet-stream"
	clean := sanitizeHeaderValue(strings.TrimSpace(contentType))
	if clean == "" {
		return fallback
	}
	mediatype, params, err := mime.ParseMediaType(clean)
	if err != nil {
		return fallback
	}
	if v := mime.FormatMediaType(mediatype, params); v != "" {
		return v
	}
	return fallback
}

// composeMIME assembles the message bytes.
//
//	text only         → text/plain
//	text + html       → multipart/alternative
//	any attachments   → multipart/mixed wrapping the above
func composeMIME(opts sendOpts) ([]byte, error) {
	var msg bytes.Buffer

	writeHeader := func(name, val string) {
		fmt.Fprintf(&msg, "%s: %s\r\n", name, sanitizeHeaderValue(val))
	}
	writeHeader("From", opts.from)
	writeHeader("To", strings.Join(opts.to, ", "))
	if opts.subject != "" {
		writeHeader("Subject", mime.QEncoding.Encode("utf-8", opts.subject))
	}
	writeHeader("Date", time.Now().UTC().Format(time.RFC1123Z))
	writeHeader("Message-ID", fmt.Sprintf("<%s@%s>", randomID(), hostFromFrom(opts.from)))
	writeHeader("MIME-Version", "1.0")
	for k, v := range opts.headers {
		writeHeader(k, v)
	}

	hasHTML := opts.html != ""
	hasAtt := len(opts.attachments) > 0

	switch {
	case !hasHTML && !hasAtt:
		writeHeader("Content-Type", "text/plain; charset=utf-8")
		writeHeader("Content-Transfer-Encoding", "7bit")
		msg.WriteString("\r\n")
		msg.WriteString(opts.body)
	case hasHTML && !hasAtt:
		boundary := randomBoundary()
		writeHeader("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
		msg.WriteString("\r\n")
		writeAltParts(&msg, boundary, opts.body, opts.html)
	case !hasHTML && hasAtt:
		boundary := randomBoundary()
		writeHeader("Content-Type", fmt.Sprintf(`multipart/mixed; boundary="%s"`, boundary))
		msg.WriteString("\r\n")
		fmt.Fprintf(&msg, "--%s\r\n", boundary)
		fmt.Fprintf(&msg, "Content-Type: text/plain; charset=utf-8\r\n")
		fmt.Fprintf(&msg, "Content-Transfer-Encoding: 7bit\r\n\r\n")
		msg.WriteString(opts.body)
		msg.WriteString("\r\n")
		writeAttachmentParts(&msg, boundary, opts.attachments)
		fmt.Fprintf(&msg, "--%s--\r\n", boundary)
	default: // hasHTML && hasAtt
		outerBoundary := randomBoundary()
		innerBoundary := randomBoundary()
		writeHeader("Content-Type", fmt.Sprintf(`multipart/mixed; boundary="%s"`, outerBoundary))
		msg.WriteString("\r\n")
		fmt.Fprintf(&msg, "--%s\r\n", outerBoundary)
		fmt.Fprintf(&msg, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", innerBoundary)
		writeAltParts(&msg, innerBoundary, opts.body, opts.html)
		writeAttachmentParts(&msg, outerBoundary, opts.attachments)
		fmt.Fprintf(&msg, "--%s--\r\n", outerBoundary)
	}
	return msg.Bytes(), nil
}

func writeAltParts(msg *bytes.Buffer, boundary, text, html string) {
	fmt.Fprintf(msg, "--%s\r\n", boundary)
	fmt.Fprintf(msg, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(msg, "Content-Transfer-Encoding: 7bit\r\n\r\n")
	msg.WriteString(text)
	msg.WriteString("\r\n")
	fmt.Fprintf(msg, "--%s\r\n", boundary)
	fmt.Fprintf(msg, "Content-Type: text/html; charset=utf-8\r\n")
	fmt.Fprintf(msg, "Content-Transfer-Encoding: 7bit\r\n\r\n")
	msg.WriteString(html)
	msg.WriteString("\r\n")
	fmt.Fprintf(msg, "--%s--\r\n", boundary)
}

func writeAttachmentParts(msg *bytes.Buffer, boundary string, atts []sendAttachment) {
	for _, a := range atts {
		fmt.Fprintf(msg, "--%s\r\n", boundary)
		fmt.Fprintf(msg, "Content-Type: %s\r\n", attachmentContentType(a.ContentType))
		fmt.Fprintf(msg, "%s", attachmentContentDisposition(a.Filename))
		fmt.Fprintf(msg, "\r\nContent-Transfer-Encoding: base64\r\n\r\n")
		b64 := base64.StdEncoding.EncodeToString(a.Bytes)
		for i := 0; i < len(b64); i += 76 {
			end := i + 76
			if end > len(b64) {
				end = len(b64)
			}
			msg.WriteString(b64[i:end])
			msg.WriteString("\r\n")
		}
	}
}

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func randomBoundary() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func hostFromFrom(from string) string {
	if at := strings.LastIndex(from, "@"); at >= 0 && at < len(from)-1 {
		return from[at+1:]
	}
	return "localhost"
}
