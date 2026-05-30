package main

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// udpNamespace wires net.udp.* — UDP client sockets (connected + bound)
// with a push/callback read model (onMessage / onClose / onError + send +
// close).
func udpNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{} // open added in Task 4
}
