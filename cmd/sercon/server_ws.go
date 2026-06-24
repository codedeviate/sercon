package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// wsMessage is one frame's worth of payload pushed from the reader
// goroutine onto the recv channel. err non-nil signals a read error or
// remote close; the iterator's next() resolves with {done: true} on
// that signal (and on channel close).
type wsMessage struct {
	msgType websocket.MessageType
	data    []byte
	err     error
}

// wsState tracks one upgraded WebSocket connection. release is the
// HoldRun release function (engine-level loop-alive sentinel); calling
// it twice is safe because Engine.HoldRun's release is idempotent.
type wsState struct {
	conn    *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	recv    chan wsMessage
	closed  atomic.Bool
	eng     *scriptengine.Engine
	release func()
	// Populated from the peer's close frame (if any) when the reader sees a
	// websocket.CloseError. closeCode is 0 until then (no valid WS code is 0).
	closeCode   atomic.Int64
	closeReason atomic.Value // string
}

// installAsyncIteratorProg is the compiled JS that installs the
// Symbol.asyncIterator method on the WebSocket object. *goja.Program is
// safe to share across runtimes, so we compile once at package init and
// run per-VM with vm.RunProgram. Per-Run execution is safe because the
// arrow function captures nothing — it only writes a fresh function
// onto the supplied object.
var installAsyncIteratorProg = goja.MustCompile("internal:install-async-iter",
	`(obj) => { obj[Symbol.asyncIterator] = function() { return this; }; }`, false)

// upgradeWebSocketImpl is invoked from res.upgradeWebSocket(opts?). It
// performs the HTTP upgrade via coder/websocket, spawns a reader
// goroutine, and returns a JS object that's both an AsyncIterable and
// carries send/close/remote.
//
// state.upgrade is set (under the state mutex) + state.markFinal() is
// called BEFORE Accept so the dispatcher's <-state.notify unblocks and
// writeResponse sees state.upgrade=true and skips header/body writes
// (the websocket library hijacks the connection and owns it from here).
func upgradeWebSocketImpl(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, state *responseState, w http.ResponseWriter, r *http.Request, opts goja.Value) goja.Value {
	bufSize := 64
	if opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
		obj := opts.ToObject(vm)
		if v := obj.Get("readBuffer"); v != nil && !goja.IsUndefined(v) {
			if n := int(v.ToInteger()); n > 0 {
				bufSize = n
			}
		}
	}

	// Mark the response as upgraded BEFORE Accept so the dispatcher
	// doesn't try to write headers after the websocket library hijacks
	// the connection.
	state.mu.Lock()
	state.upgrade = true
	state.mu.Unlock()
	state.markFinal()

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("upgradeWebSocket: %w", err)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	ws := &wsState{
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
		recv:    make(chan wsMessage, bufSize),
		eng:     eng,
		release: eng.HoldRun(fmt.Sprintf("websocket %s", r.RemoteAddr)),
	}

	// Reader goroutine: pull frames into recv until close/error. The
	// channel close serves as the "no more frames" signal to the
	// iterator's next() callback.
	go func() {
		defer close(ws.recv)
		for {
			mt, data, err := conn.Read(ctx)
			if err != nil {
				// Capture the peer's close code/reason if this was a clean
				// WebSocket close frame, so the script can read ws.closeCode /
				// ws.closeReason after the message iterator ends.
				var ce websocket.CloseError
				if errors.As(err, &ce) {
					ws.closeCode.Store(int64(ce.Code))
					ws.closeReason.Store(ce.Reason)
				}
				select {
				case ws.recv <- wsMessage{err: err}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case ws.recv <- wsMessage{msgType: mt, data: data}:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Build the JS WebSocket object.
	obj := vm.NewObject()
	_ = obj.Set("remote", r.RemoteAddr)

	// next() — returns a Promise<{value, done}>. Each call dequeues one
	// message from the recv channel off-loop, then resolves on the loop.
	// When the channel closes (reader exited) or the frame carries an
	// error, the iterator terminates with {done: true} and the HoldRun
	// sentinel is released.
	released := atomic.Bool{}
	releaseOnce := func() {
		if !released.Swap(true) {
			ws.release()
		}
	}
	_ = obj.Set("next", func(call goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			msg, ok := <-ws.recv
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if !ok || msg.err != nil {
					// Channel closed or read error — terminate iterator.
					// Surface the peer's close code/reason (when the close was
					// a clean WebSocket close frame) on the socket object so the
					// script can inspect them after the loop ends.
					if c := ws.closeCode.Load(); c != 0 {
						_ = obj.Set("closeCode", c)
						reason, _ := ws.closeReason.Load().(string)
						_ = obj.Set("closeReason", reason)
					}
					releaseOnce()
					result := vm.NewObject()
					_ = result.Set("done", true)
					_ = result.Set("value", goja.Undefined())
					_ = resolve(result)
					return
				}
				result := vm.NewObject()
				_ = result.Set("done", false)
				val := vm.NewObject()
				if msg.msgType == websocket.MessageText {
					_ = val.Set("type", "text")
					_ = val.Set("text", string(msg.data))
				} else {
					_ = val.Set("type", "binary")
					_ = val.Set("bytes", vm.ToValue(msg.data))
				}
				_ = result.Set("value", val)
				_ = resolve(result)
			})
		}()
		return vm.ToValue(promise)
	})

	// send(data) — text frame for string, binary frame for Uint8Array.
	_ = obj.Set("send", func(call goja.FunctionCall) goja.Value {
		if ws.closed.Load() {
			panic(vm.NewTypeError("ws.send: connection closed"))
		}
		arg := call.Argument(0)
		promise, resolve, reject := vm.NewPromise()
		// Snapshot the payload on the loop (goja Values aren't safe to
		// touch off-loop). Both Export() and String() happen here in the
		// callback, which is on the loop thread.
		var (
			text  string
			bytes []byte
		)
		exported := arg.Export()
		if bs, ok := exported.([]byte); ok {
			bytes = bs
		} else {
			text = arg.String()
		}
		go func() {
			var werr error
			if bytes != nil {
				werr = ws.conn.Write(ctx, websocket.MessageBinary, bytes)
			} else {
				werr = ws.conn.Write(ctx, websocket.MessageText, []byte(text))
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if werr != nil {
					_ = reject(vm.NewGoError(werr))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
		return vm.ToValue(promise)
	})

	// close(code?, reason?) — send a close frame, cancel the reader,
	// release the HoldRun sentinel. The iterator then yields {done:true}
	// on the next/current next() call as the recv channel closes.
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		ws.closed.Store(true)
		code := websocket.StatusNormalClosure
		reason := ""
		if len(call.Arguments) >= 1 && !goja.IsUndefined(call.Argument(0)) {
			code = websocket.StatusCode(int(call.Argument(0).ToInteger()))
		}
		if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) {
			reason = call.Argument(1).String()
		}
		promise, resolve, _ := vm.NewPromise()
		go func() {
			_ = ws.conn.Close(code, reason)
			cancel()
			loop.RunOnLoop(func(vm *goja.Runtime) {
				releaseOnce()
				_ = resolve(goja.Undefined())
			})
		}()
		return vm.ToValue(promise)
	})

	// Install Symbol.asyncIterator. The compiled program returns the
	// arrow function; we call it with the obj as the sole argument.
	installVal, err := vm.RunProgram(installAsyncIteratorProg)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("install async iterator: %w", err)))
	}
	installFn, ok := goja.AssertFunction(installVal)
	if !ok {
		panic(vm.NewGoError(fmt.Errorf("install async iterator: not callable")))
	}
	if _, err := installFn(goja.Undefined(), vm.ToValue(obj)); err != nil {
		panic(vm.NewGoError(fmt.Errorf("install async iterator: %w", err)))
	}

	return vm.ToValue(obj)
}
