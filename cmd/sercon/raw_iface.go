package main

import (
	"fmt"
	"net"
)

// egressFor determines which local interface and source IPv4 address the
// kernel would use to reach dst. It "connects" a UDP socket (no packets are
// actually sent for UDP connect) so the kernel performs route selection, reads
// the chosen local address, then maps that address back to an interface name.
// Used to pick the interface to capture replies on and the default source IP.
func egressFor(dst net.IP) (iface string, src net.IP, err error) {
	// Port 9 (discard) is never actually contacted — a UDP connect only triggers
	// route selection and local-address binding, sending no packet.
	c, err := net.Dial("udp4", net.JoinHostPort(dst.String(), "9"))
	if err != nil {
		return "", nil, fmt.Errorf("route lookup to %s: %w", dst, err)
	}
	defer func() { _ = c.Close() }()

	la, ok := c.LocalAddr().(*net.UDPAddr)
	if !ok || la.IP == nil {
		return "", nil, fmt.Errorf("route lookup to %s: no local address", dst)
	}
	src = la.IP

	ifaces, ierr := net.Interfaces()
	if ierr != nil {
		return "", nil, fmt.Errorf("enumerate interfaces: %w", ierr)
	}
	for _, ifc := range ifaces {
		addrs, aerr := ifc.Addrs()
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(src) {
				return ifc.Name, src, nil
			}
		}
	}
	return "", src, fmt.Errorf("no interface found for source address %s", src)
}
