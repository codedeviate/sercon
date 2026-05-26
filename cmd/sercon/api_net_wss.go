package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coder/websocket"
	"github.com/dop251/goja"
)

// wssProbe implements `api.net.wss(url, opts?)` — opens a WebSocket
// handshake against `url` (ws:// or wss://), optionally measures a
// ping/pong round-trip, and closes cleanly. A connectivity / liveness
// probe, not a streaming client; the connection doesn't outlive the
// call.
//
// Result:
//
//	{ url, connected: true, subprotocol, status, handshakeMs, pingMs }
//
// `status` is the HTTP status of the 101 upgrade response (101 on
// success). `pingMs` is the ping/pong RTT, or -1 when the ping was
// skipped / failed. Handshake failure (non-101, refused, bad URL)
// throws — unlike the TCP/SMTP probes there's no useful "partial"
// result for a failed WebSocket upgrade.
func wssProbe(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	url := call.Argument(0).String()
	if url == "" {
		return nil, errors.New("net.wss: url required")
	}
	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 10*time.Second)
	doPing := optBool(opts, "ping", true)

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	conn, resp, err := websocket.Dial(dialCtx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("net.wss: handshake: %w", err)
	}
	handshakeMs := float64(time.Since(start)) / float64(time.Millisecond)
	// CloseNow on the way out — we're a probe, a graceful close
	// handshake isn't worth the extra round-trip.
	defer func() { _ = conn.CloseNow() }()

	status := 0
	subprotocol := conn.Subprotocol()
	if resp != nil {
		status = resp.StatusCode
	}

	pingMs := -1.0
	if doPing {
		pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
		defer pingCancel()
		// coder/websocket's Ping waits for the pong, but the pong is only
		// processed while the read loop is being pumped. CloseRead starts
		// a background reader that discards data frames and handles
		// control frames (the pong), which is exactly what a one-shot
		// ping probe needs — we're not reading application messages.
		conn.CloseRead(pingCtx)
		pingStart := time.Now()
		if err := conn.Ping(pingCtx); err == nil {
			pingMs = float64(time.Since(pingStart)) / float64(time.Millisecond)
		}
		// A failed ping leaves pingMs at -1 — the server completed the
		// handshake but didn't answer control frames, which is a finding
		// worth surfacing rather than failing the whole probe.
	}

	return map[string]any{
		"url":         url,
		"connected":   true,
		"subprotocol": subprotocol,
		"status":      status,
		"handshakeMs": handshakeMs,
		"pingMs":      pingMs,
	}, nil
}
