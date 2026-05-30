package main

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// icmpNamespace wires net.icmp.* — raw ICMP send/receive with a
// push/callback read model (onMessage / onClose / onError + send +
// close). Requires raw-socket privileges (root / CAP_NET_RAW).
func icmpNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{} // open added in Task 5
}
