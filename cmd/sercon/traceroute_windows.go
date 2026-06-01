//go:build windows

package main

import "syscall"

// setIPTTL is a no-op on Windows: setting IP_TTL via RawConn.Control needs a
// syscall.Handle (not int), and raw-ICMP traceroute already requires
// privileges this build doesn't target on Windows. TCP traceroute hops will
// not be TTL-limited here; the stub keeps the cross-compile green.
func setIPTTL(_ int) func(network, address string, c syscall.RawConn) error {
	return func(_, _ string, _ syscall.RawConn) error { return nil }
}
