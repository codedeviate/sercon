//go:build darwin || freebsd || netbsd || openbsd

package main

import (
	"fmt"
	"net"
	"syscall"

	"golang.org/x/net/route"
)

// Routing-socket address slot indices (RTAX_*). x/net/route doesn't export
// these, so we name the two we read: destination and gateway. The netmask
// slot (2) is consumed positionally below.
const (
	rtaxDst     = 0
	rtaxGateway = 1
	rtaxNetmask = 2
)

// hostRoutes reads the BSD/macOS routing table from the routing socket via
// golang.org/x/net/route (pure-Go, no cgo). It fetches the RIB for both
// address families and normalises each RouteMessage into a routeEntry.
func hostRoutes() ([]routeEntry, error) {
	rib, err := route.FetchRIB(syscall.AF_UNSPEC, route.RIBTypeRoute, 0)
	if err != nil {
		return nil, err
	}
	msgs, err := route.ParseRIB(route.RIBTypeRoute, rib)
	if err != nil {
		return nil, err
	}

	ifNames := map[int]string{}
	nameFor := func(idx int) string {
		if idx <= 0 {
			return ""
		}
		if n, ok := ifNames[idx]; ok {
			return n
		}
		n := ""
		if iface, err := net.InterfaceByIndex(idx); err == nil {
			n = iface.Name
		}
		ifNames[idx] = n
		return n
	}

	var out []routeEntry
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || len(rm.Addrs) <= rtaxDst {
			continue
		}
		dstAddr := rm.Addrs[rtaxDst]
		if dstAddr == nil {
			continue
		}
		var maskAddr route.Addr
		if len(rm.Addrs) > rtaxNetmask {
			maskAddr = rm.Addrs[rtaxNetmask]
		}
		var gwAddr route.Addr
		if len(rm.Addrs) > rtaxGateway {
			gwAddr = rm.Addrs[rtaxGateway]
		}

		dst, family, ok := addrIP(dstAddr)
		if !ok {
			continue
		}
		prefix := maskLen(maskAddr, family)
		gw := ""
		if ip, _, ok := addrIP(gwAddr); ok {
			gw = ip.String()
		}
		out = append(out, routeEntry{
			Destination: fmt.Sprintf("%s/%d", dst.String(), prefix),
			Gateway:     gw,
			Interface:   nameFor(rm.Index),
			Family:      family,
			Metric:      0, // BSD route messages don't carry a portable metric here
		})
	}
	return out, nil
}

// addrIP extracts a net.IP and family ("ip"/"ip6") from a route.Addr. Returns
// ok=false for link-layer or nil addresses (e.g. a directly-connected route's
// gateway, which is a LinkAddr).
func addrIP(a route.Addr) (ip net.IP, family string, ok bool) {
	switch v := a.(type) {
	case *route.Inet4Addr:
		b := v.IP
		return net.IPv4(b[0], b[1], b[2], b[3]), "ip", true
	case *route.Inet6Addr:
		ip := make(net.IP, net.IPv6len)
		copy(ip, v.IP[:])
		return ip, "ip6", true
	default:
		return nil, "", false
	}
}

// maskLen returns the prefix length for a route's netmask slot. A missing or
// non-IP mask (host route) yields the family's full length (/32 or /128).
func maskLen(a route.Addr, family string) int {
	full := 32
	if family == "ip6" {
		full = 128
	}
	switch v := a.(type) {
	case *route.Inet4Addr:
		ones, _ := net.IPMask(v.IP[:]).Size()
		return ones
	case *route.Inet6Addr:
		ones, _ := net.IPMask(v.IP[:]).Size()
		return ones
	default:
		return full
	}
}
