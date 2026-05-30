package main

import (
	"net/http"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// staticMarker carries an http.Handler that the route compiler unwraps
// and registers directly on the mux — bypassing the LoopCallable dispatch
// because file-serving is pure Go (no JS handler to call per request).
type staticMarker struct {
	handler http.Handler
}

// httpStaticBinding returns the JS function exposed as
// server.http.static / server.https.static. The function takes
// {dir, stripPrefix, index?, etag?} and returns a JS-visible value
// whose Export() yields a *staticMarker the route compiler recognises.
//
// opts.index / opts.etag are accepted but unused in v0.10.0 — stdlib
// http.FileServer handles index.html serving and emits Last-Modified
// + ETag by default. Future enhancement hooks recorded in
// OUT-OF-SCOPE.md.
func httpStaticBinding(vm *goja.Runtime, _ *eventloop.EventLoop) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		opts := call.Argument(0).ToObject(vm)
		dir := opts.Get("dir").String()
		strip := ""
		if v := opts.Get("stripPrefix"); v != nil && !goja.IsUndefined(v) {
			strip = v.String()
		}
		handler := http.FileServer(http.Dir(dir))
		if strip != "" {
			handler = http.StripPrefix(strip, handler)
		}
		return vm.ToValue(&staticMarker{handler: handler})
	}
}
