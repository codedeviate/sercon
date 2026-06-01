package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
	"time"

	"github.com/dop251/goja"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// tracerouteHop is one row of the traceroute result. Address is a pointer so
// an unanswered hop serializes as JSON null (not ""). RTTsMs is initialized to
// a non-nil slice so it serializes as [] rather than null.
type tracerouteHop struct {
	TTL     int       `json:"ttl"`
	Address *string   `json:"address"`
	RTTsMs  []float64 `json:"rttsMs"`
	Reached bool      `json:"reached"`
}

// parseQuotedProbe extracts a per-probe identifier from the bytes an ICMP
// time-exceeded / dest-unreachable quotes back (the original IPv4 header plus
// the first 8 bytes of its payload). The identifier is the ICMP echo seq, the
// UDP destination port, or the TCP source port — whatever uniquely tags the
// probe for the given protocol. Returns (id, false) if the blob is too short.
func parseQuotedProbe(quoted []byte, proto string) (uint16, bool) {
	if len(quoted) < 20 {
		return 0, false
	}
	ihl := int(quoted[0]&0x0f) * 4
	if ihl < 20 || len(quoted) < ihl+8 {
		return 0, false
	}
	p := quoted[ihl:]
	switch proto {
	case "icmp":
		return uint16(p[6])<<8 | uint16(p[7]), true // echo seq
	case "udp":
		return uint16(p[2])<<8 | uint16(p[3]), true // dst port
	case "tcp":
		return uint16(p[0])<<8 | uint16(p[1]), true // src port
	default:
		return 0, false
	}
}

const icmpRunID uint16 = 0x7e57 // tags our echo probes; seq is the real discriminator

// traceroute implements net.probe.traceroute(host, opts?). Needs root /
// CAP_NET_RAW. Sequential per TTL.
func traceroute(ctx context.Context, call goja.FunctionCall) ([]tracerouteHop, error) {
	host := call.Argument(0).String()
	if host == "" {
		return nil, errors.New("net.probe.traceroute: host required")
	}
	opts := optsAsMap(call)
	protocol := optString(opts, "protocol", "icmp")
	if protocol != "icmp" && protocol != "udp" && protocol != "tcp" {
		return nil, fmt.Errorf("net.probe.traceroute: protocol must be 'icmp', 'udp', or 'tcp', got %q", protocol)
	}
	maxHops := optInt(opts, "maxHops", 30)
	if maxHops <= 0 {
		maxHops = 30
	}
	timeout := optMillis(opts, "timeout", 2*time.Second)
	probes := optInt(opts, "probes", 3)
	if probes <= 0 {
		probes = 3
	}
	defaultPort := 80
	if protocol == "udp" {
		defaultPort = 33434
	}
	port := optInt(opts, "port", defaultPort)

	dst, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, fmt.Errorf("net.probe.traceroute: resolve %q: %w", host, err)
	}
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("net.probe.traceroute: %w (needs root / CAP_NET_RAW)", err)
	}
	defer func() { _ = conn.Close() }()

	tr := &tracer{conn: conn, v4: ipv4.NewPacketConn(conn), dst: dst, protocol: protocol, port: port, timeout: timeout, probes: probes}
	hops := make([]tracerouteHop, 0, maxHops)
	for ttl := 1; ttl <= maxHops; ttl++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("net.probe.traceroute: %w", err)
		}
		hop := tr.probeHop(ctx, ttl)
		hops = append(hops, hop)
		if hop.Reached {
			break
		}
	}
	return hops, nil
}

type tracer struct {
	conn     *icmp.PacketConn
	v4       *ipv4.PacketConn
	dst      *net.IPAddr
	protocol string
	port     int
	timeout  time.Duration
	probes   int
}

func (tr *tracer) probeHop(ctx context.Context, ttl int) tracerouteHop {
	hop := tracerouteHop{TTL: ttl, RTTsMs: []float64{}}
	for p := 0; p < tr.probes; p++ {
		id := uint16(ttl*256 + p)
		start := time.Now()
		addr, reached, ok := tr.sendAndAwait(ctx, ttl, id)
		if !ok {
			continue
		}
		rtt := float64(time.Since(start)) / float64(time.Millisecond)
		if hop.Address == nil {
			a := addr
			hop.Address = &a
		}
		hop.RTTsMs = append(hop.RTTsMs, rtt)
		if reached {
			hop.Reached = true
			// Stop probing this hop once the destination answers. Besides
			// being the natural end of the trace, this is load-bearing for
			// TCP mode: tcpProbe returns the instant its dial goroutine
			// reports "reached", leaving the sibling awaitICMPReply goroutine
			// still reading the shared raw ICMP socket. Breaking here prevents
			// the next probe from spawning a second concurrent reader on that
			// same socket (the orphan is reaped when traceroute closes conn).
			break
		}
	}
	return hop
}

// sendAndAwait sends one probe at ttl tagged with id and waits up to tr.timeout
// for a matching reply: (responder address, reached, ok).
func (tr *tracer) sendAndAwait(ctx context.Context, ttl int, id uint16) (string, bool, bool) {
	switch tr.protocol {
	case "icmp":
		if err := tr.sendICMP(ttl, id); err != nil {
			return "", false, false
		}
		return tr.awaitICMPReply(int(id))
	case "udp":
		if err := tr.sendUDP(ttl, id); err != nil {
			return "", false, false
		}
		return tr.awaitICMPReply(tr.port + int(id)) // matchID = the dst port we used
	case "tcp":
		return tr.tcpProbe(ctx, ttl, id)
	default:
		return "", false, false
	}
}

func (tr *tracer) sendICMP(ttl int, id uint16) error {
	if err := tr.v4.SetTTL(ttl); err != nil {
		return err
	}
	b, err := buildICMPMessage("ip4", icmpSendOpts{id: int(icmpRunID), seq: int(id), payload: []byte("sercon-traceroute")})
	if err != nil {
		return err
	}
	_, err = tr.conn.WriteTo(b, tr.dst)
	return err
}

func (tr *tracer) sendUDP(ttl int, id uint16) error {
	uc, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: tr.dst.IP, Port: tr.port + int(id)})
	if err != nil {
		return err
	}
	defer func() { _ = uc.Close() }()
	if err := ipv4.NewConn(uc).SetTTL(ttl); err != nil {
		return err
	}
	_, err = uc.Write([]byte("sercon-traceroute"))
	return err
}

// awaitICMPReply reads the raw ICMP socket until a message correlating to
// matchID arrives or the timeout elapses. time-exceeded (router) → (addr,
// false, true); echo-reply (icmp dest) / dest-unreachable (udp dest) → (addr,
// true, true); timeout → ("", false, false).
func (tr *tracer) awaitICMPReply(matchID int) (string, bool, bool) {
	deadline := time.Now().Add(tr.timeout)
	buf := make([]byte, 1500)
	for {
		if err := tr.conn.SetReadDeadline(deadline); err != nil {
			return "", false, false
		}
		n, peer, err := tr.conn.ReadFrom(buf)
		if err != nil {
			return "", false, false
		}
		msg, perr := icmp.ParseMessage(protoICMPv4, buf[:n])
		if perr != nil {
			continue
		}
		switch body := msg.Body.(type) {
		case *icmp.TimeExceeded:
			if mid, ok := parseQuotedProbe(body.Data, tr.protocol); ok && int(mid) == matchID {
				return peer.String(), false, true
			}
		case *icmp.DstUnreach:
			if tr.protocol == "udp" {
				if mid, ok := parseQuotedProbe(body.Data, tr.protocol); ok && int(mid) == matchID {
					return peer.String(), true, true
				}
			}
		case *icmp.Echo:
			if tr.protocol == "icmp" && body.Seq == matchID {
				return peer.String(), true, true
			}
		}
	}
}

// tcpProbe attempts a TTL-limited TCP connect, tagged by a unique source port
// so the raw ICMP listener can correlate the quoted time-exceeded. The connect
// outcome IS the final-hop signal (success=open, refused=closed → reached).
func (tr *tracer) tcpProbe(ctx context.Context, ttl int, id uint16) (string, bool, bool) {
	srcPort := 33000 + int(id)%30000
	type res struct {
		addr    string
		reached bool
		ok      bool
	}
	ch := make(chan res, 2)
	go func() {
		d := net.Dialer{Timeout: tr.timeout, LocalAddr: &net.TCPAddr{Port: srcPort}, Control: setIPTTL(ttl)}
		c, err := d.DialContext(ctx, "tcp4", net.JoinHostPort(tr.dst.IP.String(), strconv.Itoa(tr.port)))
		if err == nil {
			_ = c.Close()
			ch <- res{tr.dst.IP.String(), true, true}
			return
		}
		if isConnRefused(err) {
			ch <- res{tr.dst.IP.String(), true, true}
			return
		}
		ch <- res{"", false, false}
	}()
	go func() {
		addr, reached, ok := tr.awaitICMPReply(srcPort) // matchID = quoted src port
		ch <- res{addr, reached, ok}
	}()
	for i := 0; i < 2; i++ {
		if r := <-ch; r.ok {
			return r.addr, r.reached, true
		}
	}
	return "", false, false
}

// isConnRefused reports a TCP RST ("connection refused") — the destination was
// reached even though the port is closed.
func isConnRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
