package main

import (
	"fmt"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// route.go backs net.capture.routes(): a synchronous snapshot of the host's
// IP routing table, each entry { destination, gateway, interface, family,
// metric }. Pure-Go, no cgo: Linux parses /proc/net/route + /proc/net/
// ipv6_route; the BSDs (incl. macOS) read the routing socket via
// golang.org/x/net/route. Windows would need the IP Helper API and is stubbed.
//
// The per-OS backends (route_linux.go / route_bsd.go / route_other.go) define
// hostRoutes, which returns the entries or an error.

// routeEntry is one host routing-table row, normalised across platforms.
type routeEntry struct {
	Destination string // CIDR ("0.0.0.0/0", "10.0.0.0/8", "::/0", "fe80::/64") or host IP
	Gateway     string // next-hop IP, or "" for a directly-connected / link route
	Interface   string // outgoing interface name (best-effort; may be "" if unresolved)
	Family      string // "ip" (IPv4) or "ip6" (IPv6)
	Metric      int    // route metric / priority (0 when the platform doesn't report one)
}

// captureRoutesFn implements net.capture.routes(): a synchronous list of the
// host's routing-table entries. Throws on enumeration failure or on an
// unsupported platform (Windows).
func captureRoutesFn(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		routes, err := hostRoutes()
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("net.capture.routes: %w", err)))
		}
		out := make([]*scriptengine.Ordered, 0, len(routes))
		for _, r := range routes {
			out = append(out, scriptengine.NewOrdered().
				Set("destination", r.Destination).
				Set("gateway", r.Gateway).
				Set("interface", r.Interface).
				Set("family", r.Family).
				Set("metric", r.Metric))
		}
		return scriptengine.OrderedToValue(vm, out)
	}
}
