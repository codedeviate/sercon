package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// cdpResult is one CDP response delivered to a waiting caller.
type cdpResult struct {
	result json.RawMessage
	err    error
}

// cdpConn is a browser-level Chrome DevTools Protocol connection over a single
// WebSocket. A read-pump goroutine correlates responses to callers by id;
// frames without an id (CDP events) are ignored. Commands are routed to a child
// target by including sessionId in the envelope. Safe for concurrent callers.
type cdpConn struct {
	conn    *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int
	pending map[int]chan cdpResult
	closed  bool
}

// dialCDP opens a CDP WebSocket to wsURL and starts its read pump. parent should
// be the session context so shutdown tears the connection down.
func dialCDP(parent context.Context, wsURL string) (*cdpConn, error) {
	ctx, cancel := context.WithCancel(parent)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	conn.SetReadLimit(64 << 20) // DOM.getDocument trees can be large
	c := &cdpConn{conn: conn, ctx: ctx, cancel: cancel, pending: map[int]chan cdpResult{}}
	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			c.failAll(err)
			return
		}
		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(data, &msg) != nil || msg.ID == 0 {
			continue // an event (no id) or unparseable frame — ignore
		}
		c.mu.Lock()
		ch := c.pending[msg.ID]
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		if len(msg.Error) > 0 {
			ch <- cdpResult{err: fmt.Errorf("cdp error: %s", string(msg.Error))}
		} else {
			ch <- cdpResult{result: msg.Result}
		}
	}
}

func (c *cdpConn) failAll(err error) {
	c.mu.Lock()
	c.closed = true
	for id, ch := range c.pending {
		ch <- cdpResult{err: err}
		delete(c.pending, id)
	}
	c.mu.Unlock()
}

// call sends one CDP command and waits for its response.
func (c *cdpConn) call(sessionID, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("webdriver: CDP connection closed")
	}
	c.nextID++
	id := c.nextID
	ch := make(chan cdpResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	env := map[string]any{"id": id, "method": method}
	if params != nil {
		env["params"] = params
	}
	if sessionID != "" {
		env["sessionId"] = sessionID
	}
	b, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = c.conn.Write(c.ctx, websocket.MessageText, b)
	c.writeMu.Unlock()
	if err != nil {
		// readLoop may have already removed this id; delete is then a no-op.
		// (The browser never got the command, so no real response can arrive.)
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case r := <-ch:
		return r.result, r.err
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

// callMap is call() decoding the result into a map.
func (c *cdpConn) callMap(sessionID, method string, params map[string]any) (map[string]any, error) {
	raw, err := c.call(sessionID, method, params)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	// An absent/null result (some commands return none) leaves m nil; a present
	// result must be a JSON object — surface a decode failure rather than hide it.
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("cdp: decoding %s result: %w", method, err)
		}
	}
	return m, nil
}

// close cancels the context and closes the socket; the read pump exits and fails
// any in-flight calls. Idempotent.
func (c *cdpConn) close() {
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	if already {
		return
	}
	c.cancel()
	_ = c.conn.Close(websocket.StatusNormalClosure, "")
}

// fetchBrowserWSURL queries a DevTools HTTP endpoint (host:port) for the
// browser-level webSocketDebuggerUrl. Split out for testability.
func fetchBrowserWSURL(addr string) (string, error) {
	resp, err := http.Get("http://" + addr + "/json/version")
	if err != nil {
		return "", fmt.Errorf("webdriver: querying CDP endpoint %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if json.Unmarshal(body, &v) != nil || v.WS == "" {
		return "", fmt.Errorf("webdriver: no webSocketDebuggerUrl from %s", addr)
	}
	return v.WS, nil
}
