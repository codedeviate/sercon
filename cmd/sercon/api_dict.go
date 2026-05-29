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
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// dictNamespace wires `db.dict.*` — an RFC 2229 DICT protocol client
// for dictionary-server word lookups. No popular pure-Go DICT client
// exists, so the protocol is hand-rolled over `net/textproto` (it's a
// simple line-based status-code protocol, much like SMTP). Two
// members: `define` (definitions of a word) and `match` (words that
// match under a strategy). Both are one-shot — connect, query, QUIT.
func dictNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"define": scriptengine.PromisifyAsync(vm, loop, dictDefine),
		"match":  scriptengine.PromisifyAsync(vm, loop, dictMatch),
	}
}

// dictDefine looks up `word` and returns its definitions:
//
//	{ word, found, definitions: [{ db, dbName, text }] }
//
// `opts.database` selects a specific dictionary (default `*` = all).
// A word with no definitions resolves with `found: false` and an
// empty list — "not in the dictionary" is data, not an error.
func dictDefine(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	host := call.Argument(0).String()
	word := call.Argument(1).String()
	if host == "" || word == "" {
		return nil, errors.New("dict.define: host and word required")
	}
	opts := optAt(call, 2)
	db := optString(opts, "database", "*")
	timeout := optMillis(opts, "timeout", 10*time.Second)

	tp, conn, err := dictConnect(ctx, host, opts, timeout)
	if err != nil {
		return nil, fmt.Errorf("dict.define: %w", err)
	}
	defer func() { _ = conn.Close() }()
	defer dictQuit(tp)

	id, err := tp.Cmd("DEFINE %s %s", db, dictQuote(word))
	if err != nil {
		return nil, fmt.Errorf("dict.define: send: %w", err)
	}
	tp.StartResponse(id)
	defer tp.EndResponse(id)
	code, msg, err := tp.ReadResponse(0)
	if err != nil {
		return nil, fmt.Errorf("dict.define: %w", err)
	}
	// 552 = no match (a normal "not found"); 550 = invalid database.
	if code == 552 {
		return map[string]any{"word": word, "found": false, "definitions": []map[string]any{}}, nil
	}
	if code != 150 {
		return nil, fmt.Errorf("dict.define: server %d: %s", code, msg)
	}

	defs := []map[string]any{}
	for {
		// Each definition: 151 "<word>" <db> "<name>" then text lines
		// terminated by a lone ".", then 250 at the end.
		dcode, dmsg, err := tp.ReadResponse(0)
		if err != nil {
			return nil, fmt.Errorf("dict.define: read def header: %w", err)
		}
		if dcode == 250 {
			break
		}
		if dcode != 151 {
			return nil, fmt.Errorf("dict.define: unexpected %d: %s", dcode, dmsg)
		}
		fields := dictFields(dmsg)
		dbName := ""
		if len(fields) >= 3 {
			dbName = fields[2]
		}
		text, err := tp.ReadDotLines()
		if err != nil {
			return nil, fmt.Errorf("dict.define: read def body: %w", err)
		}
		dbCode := ""
		if len(fields) >= 2 {
			dbCode = fields[1]
		}
		defs = append(defs, map[string]any{
			"db":     dbCode,
			"dbName": dbName,
			"text":   strings.Join(text, "\n"),
		})
	}
	return map[string]any{"word": word, "found": len(defs) > 0, "definitions": defs}, nil
}

// dictMatch returns the words that match `word` under a strategy
// (default `prefix`). Result: { word, matches: [{ db, word }] }.
func dictMatch(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	host := call.Argument(0).String()
	word := call.Argument(1).String()
	if host == "" || word == "" {
		return nil, errors.New("dict.match: host and word required")
	}
	opts := optAt(call, 2)
	db := optString(opts, "database", "*")
	strategy := optString(opts, "strategy", "prefix")
	timeout := optMillis(opts, "timeout", 10*time.Second)

	tp, conn, err := dictConnect(ctx, host, opts, timeout)
	if err != nil {
		return nil, fmt.Errorf("dict.match: %w", err)
	}
	defer func() { _ = conn.Close() }()
	defer dictQuit(tp)

	id, err := tp.Cmd("MATCH %s %s %s", db, strategy, dictQuote(word))
	if err != nil {
		return nil, fmt.Errorf("dict.match: send: %w", err)
	}
	tp.StartResponse(id)
	defer tp.EndResponse(id)
	code, msg, err := tp.ReadResponse(0)
	if err != nil {
		return nil, fmt.Errorf("dict.match: %w", err)
	}
	if code == 552 {
		return map[string]any{"word": word, "matches": []map[string]any{}}, nil
	}
	if code != 152 {
		return nil, fmt.Errorf("dict.match: server %d: %s", code, msg)
	}
	lines, err := tp.ReadDotLines()
	if err != nil {
		return nil, fmt.Errorf("dict.match: read matches: %w", err)
	}
	matches := []map[string]any{}
	for _, line := range lines {
		fields := dictFields(line)
		if len(fields) >= 2 {
			matches = append(matches, map[string]any{"db": fields[0], "word": fields[1]})
		}
	}
	// Trailing 250.
	_, _, _ = tp.ReadResponse(0)
	return map[string]any{"word": word, "matches": matches}, nil
}

// dictConnect dials the DICT server, reads the 220 banner, and sends
// the CLIENT announcement (politeness per the RFC). Default port 2628.
func dictConnect(ctx context.Context, host string, opts map[string]any, timeout time.Duration) (*textproto.Conn, net.Conn, error) {
	port := optString(opts, "port", "2628")
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tp := textproto.NewConn(conn)
	if _, _, err := tp.ReadResponse(220); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("banner: %w", err)
	}
	// CLIENT is advisory; ignore its response.
	if id, err := tp.Cmd("CLIENT sercon"); err == nil {
		tp.StartResponse(id)
		_, _, _ = tp.ReadResponse(0)
		tp.EndResponse(id)
	}
	return tp, conn, nil
}

func dictQuit(tp *textproto.Conn) {
	if id, err := tp.Cmd("QUIT"); err == nil {
		tp.StartResponse(id)
		_, _, _ = tp.ReadResponse(221)
		tp.EndResponse(id)
	}
}

// dictQuote wraps a word in double quotes if it contains spaces, per
// the DICT grammar. Simple words pass through unquoted.
func dictQuote(word string) string {
	if strings.ContainsAny(word, " \t") {
		return `"` + word + `"`
	}
	return word
}

// dictFields splits a status-message tail into fields, honouring
// double-quoted segments (DICT quotes db names and words that contain
// spaces). A tiny tokeniser — good enough for the well-formed output
// DICT servers produce.
func dictFields(s string) []string {
	var fields []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			fields = append(fields, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			if inQuote {
				flush()
			}
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return fields
}
