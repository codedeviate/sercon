//go:build !windows

package main

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// setIPTTL sets IP_TTL on the connecting socket so the SYN goes out with the
// given TTL. Unix-only: the Windows RawConn fd is a Handle, not an int, and
// TCP traceroute there isn't supported (see traceroute_windows.go).
func setIPTTL(ttl int) func(network, address string, c syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_TTL, ttl)
		}); err != nil {
			return err
		}
		return serr
	}
}
