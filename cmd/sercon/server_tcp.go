package main

import (
	"fmt"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// tcpServerMembers builds the {listen} map exposed as server.tcp.
func tcpServerMembers(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"listen": func(call goja.FunctionCall) goja.Value {
			return tcpListen(vm, loop, eng, call)
		},
	}
}

// tcpListen implements `server.tcp.listen({ port, host?, readBuffer? }, conn => …)`.
// Mirrors httpListen's lifecycle: synchronous bind (so bind errors throw
// immediately), an accept loop in a background goroutine, one eng.HoldRun
// keeping the loop alive while bound, and a `{ address, close() }` handle.
// Each accepted connection is wrapped with the same buildTCPObject handle the
// net.tcp.connect client uses, and the per-connection handler is invoked on
// the loop via LoopCallable (which is the correct marshalling primitive for
// the off-loop accept goroutine).
func tcpListen(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, call goja.FunctionCall) goja.Value {
	opts := call.Argument(0)
	if opts == nil || goja.IsUndefined(opts) || goja.IsNull(opts) {
		panic(vm.NewTypeError("server.tcp.listen: options object required"))
	}

	host := ""
	bufSize := 64
	port := 0
	if m, ok := opts.Export().(map[string]any); ok {
		host = optString(m, "host", "")
		if n := optInt(m, "readBuffer", bufSize); n > 0 {
			bufSize = n
		}
		port = optInt(m, "port", 0)
	}

	fn, ok := goja.AssertFunction(call.Argument(1))
	if !ok {
		panic(vm.NewTypeError("server.tcp.listen: handler function required"))
	}
	handler := scriptengine.NewLoopCallable(loop, fn)

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.tcp.listen %s: %w", addr, err)))
	}
	if serveReadyWriter != nil {
		fmt.Fprintf(serveReadyWriter, "READY listening on tcp/%s\n", ln.Addr().String())
	}
	release := eng.HoldRun("server.tcp listen " + ln.Addr().String())

	// Accept loop (off-loop). For each connection, build the handle + invoke
	// the handler ON the loop via LoopCallable (buildTCPObject needs the vm
	// and must run on the loop).
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed → exit
			}
			_, _ = handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
				return []goja.Value{buildTCPObject(vm, loop, eng, conn, bufSize)}, nil
			})
		}
	}()

	handle := vm.NewObject()
	_ = handle.Set("address", fmt.Sprintf("tcp/%s", ln.Addr().String()))
	closeOnce := atomic.Bool{}
	_ = handle.Set("close", func(goja.FunctionCall) goja.Value {
		if !closeOnce.Swap(true) {
			_ = ln.Close()
			release()
		}
		return goja.Undefined()
	})
	return handle
}
