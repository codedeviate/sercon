package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// smtpProbe implements `net.probe.smtp(host, opts?)` — a capability
// probe, not a send pipeline. It opens the connection, reads the
// greeting banner, sends EHLO, and parses the advertised extensions
// (STARTTLS availability, AUTH mechanisms, SIZE limit, …). Then it
// QUITs cleanly. No mail is sent.
//
// We hand-roll the conversation over net/textproto rather than using
// net/smtp because net/smtp's Client doesn't expose the raw banner or
// the full extension list as data — only `Extension(name)` lookups
// against a private map. A probe wants to *report* what the server
// advertised, so we parse the EHLO response ourselves.
//
// Result:
//
//	{ host, port, banner, ehloDomain, extensions: string[],
//	  starttls: boolean, authMechanisms: string[], sizeLimit: number }
//
// Connection / protocol failures throw. A server that simply doesn't
// advertise STARTTLS or AUTH reports them as false / empty — that's a
// finding, not an error.
//
// smtpProbeResult is a json-tagged struct rather than a map[string]any so
// the resolved JS object has stable key order (goja enumerates struct
// fields in declaration order; a Go map would shuffle JSON.stringify
// output run-to-run — same pattern as tcpProbeResult in probe.go).
// Field order matches the documented result shape above.
type smtpProbeResult struct {
	Host           string   `json:"host"`
	Port           string   `json:"port"`
	Banner         string   `json:"banner"`
	EhloDomain     string   `json:"ehloDomain"`
	Extensions     []string `json:"extensions"`
	Starttls       bool     `json:"starttls"`
	AuthMechanisms []string `json:"authMechanisms"`
	SizeLimit      int64    `json:"sizeLimit"`
}

// smtpProbeArgs carries the on-loop-extracted arguments of net.probe.smtp.
type smtpProbeArgs struct {
	host     string
	port     string
	timeout  time.Duration
	ehloName string
}

func smtpProbeExtract(call goja.FunctionCall) (smtpProbeArgs, error) {
	host := call.Argument(0).String()
	if host == "" {
		return smtpProbeArgs{}, errors.New("net.smtp: host required")
	}
	opts := optsAsMap(call)
	return smtpProbeArgs{
		host:     host,
		port:     optString(opts, "port", "25"),
		timeout:  optMillis(opts, "timeout", 10*time.Second),
		ehloName: optString(opts, "ehloName", "localhost"),
	}, nil
}

func smtpProbe(ctx context.Context, args smtpProbeArgs) (smtpProbeResult, error) {
	host, port, timeout, ehloName := args.host, args.port, args.timeout, args.ehloName

	addr := net.JoinHostPort(host, port)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return smtpProbeResult{}, fmt.Errorf("net.smtp: dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	// Bound the whole conversation by the timeout so a server that
	// accepts the connection but never speaks can't hang the probe.
	_ = conn.SetDeadline(time.Now().Add(timeout))

	tp := textproto.NewConn(conn)
	defer func() { _ = tp.Close() }()

	// Greeting: a 220 line (possibly multi-line).
	_, banner, err := tp.ReadResponse(220)
	if err != nil {
		return smtpProbeResult{}, fmt.Errorf("net.smtp: read greeting: %w", err)
	}

	// EHLO — capture the multi-line 250 response. textproto's
	// ReadResponse joins continuation lines with newlines, which is
	// exactly the per-capability split we want.
	id, err := tp.Cmd("EHLO %s", ehloName)
	if err != nil {
		return smtpProbeResult{}, fmt.Errorf("net.smtp: send EHLO: %w", err)
	}
	tp.StartResponse(id)
	_, ehloMsg, err := tp.ReadResponse(250)
	tp.EndResponse(id)
	if err != nil {
		return smtpProbeResult{}, fmt.Errorf("net.smtp: read EHLO: %w", err)
	}

	// Best-effort QUIT; ignore errors (we already have what we need).
	if id, err := tp.Cmd("QUIT"); err == nil {
		tp.StartResponse(id)
		_, _, _ = tp.ReadResponse(221)
		tp.EndResponse(id)
	}

	result := parseEHLO(ehloMsg)
	result.Host = host
	result.Port = port
	result.Banner = strings.TrimSpace(banner)
	return result, nil
}

// parseEHLO turns the EHLO 250 response (greeting line + one line per
// extension) into the structured capability view. The first line is
// the server's domain greeting; the rest are extension keywords like
// `STARTTLS`, `AUTH PLAIN LOGIN`, `SIZE 35882577`, `8BITMIME`.
func parseEHLO(msg string) smtpProbeResult {
	lines := strings.Split(msg, "\n")
	// Empty (non-nil) slices so JSON.stringify yields [] rather than null.
	out := smtpProbeResult{
		Extensions:     []string{},
		AuthMechanisms: []string{},
	}
	if len(lines) == 0 {
		return out
	}
	// First line is the greeting (e.g. "mail.example.com at your service").
	out.EhloDomain = strings.TrimSpace(lines[0])

	exts := []string{}
	auth := []string{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		keyword := strings.ToUpper(fields[0])
		exts = append(exts, line)
		switch keyword {
		case "STARTTLS":
			out.Starttls = true
		case "AUTH":
			// Remaining fields are the mechanism names.
			for _, m := range fields[1:] {
				auth = append(auth, strings.ToUpper(m))
			}
		case "SIZE":
			if len(fields) > 1 {
				out.SizeLimit = parseInt64(fields[1])
			}
		}
	}
	out.Extensions = exts
	out.AuthMechanisms = auth
	return out
}

// parseInt64 is a tiny tolerant parser — non-numeric input yields 0
// rather than an error, since a malformed SIZE value shouldn't fail
// the whole probe.
func parseInt64(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
