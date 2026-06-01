package main

import (
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
)

// openRawSend opens a raw IPv4 send socket with the kernel's IP header
// inclusion (IP_HDRINCL) handled by ipv4.RawConn — we supply the full IP
// header on every write. IPPROTO_RAW (255) makes the socket send-only and
// enables HDRINCL by default on both Linux and BSD/macOS; ipv4.RawConn
// normalizes the per-OS header byte-order differences. Receiving is done
// separately via the capture path, so this socket is never read. Needs
// root / CAP_NET_RAW.
func openRawSend() (*ipv4.RawConn, error) {
	c, err := net.ListenPacket("ip4:255", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("net.raw: open raw socket: %w (needs root / CAP_NET_RAW)", err)
	}
	rc, err := ipv4.NewRawConn(c)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("net.raw: raw conn: %w", err)
	}
	return rc, nil
}

// sendRaw writes one complete IPv4 packet (full bytes from buildPacket or a
// caller-supplied Uint8Array) via the HDRINCL raw conn. The leading IPv4
// header is parsed to split header from payload for ipv4.RawConn.WriteTo,
// which re-serializes the header (handling the macOS field byte-order quirk).
func sendRaw(rc *ipv4.RawConn, full []byte) error {
	if len(full) < ipv4.HeaderLen {
		return fmt.Errorf("net.raw.send: packet too short (%d bytes)", len(full))
	}
	h, err := ipv4.ParseHeader(full[:ipv4.HeaderLen])
	if err != nil {
		return fmt.Errorf("net.raw.send: parse IP header: %w", err)
	}
	if h.Len < ipv4.HeaderLen || h.Len > len(full) {
		return fmt.Errorf("net.raw.send: bad IHL %d", h.Len)
	}
	return rc.WriteTo(h, full[h.Len:], nil)
}
