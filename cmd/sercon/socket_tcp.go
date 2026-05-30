package main

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// tcpNamespace wires net.tcp.* — TCP client sockets with a push/callback
// read model (onData / onClose / onError + write + close).
func tcpNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{} // connect added in Task 3
}
