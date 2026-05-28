package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/dop251/goja"
)

// pingProbe implements `api.net.probe.ping(host, opts?)`. Two modes:
//
//   - "tcp" (default) — dials host:port `count` times and measures the
//     connect RTT. No special privileges; works everywhere. The
//     "reachability" it measures is "can I open a TCP connection",
//     which is what most liveness checks actually want.
//   - "icmp" — real ICMP echo via pro-bing. More faithful to `ping(8)`
//     but needs raw-socket privileges (root, or CAP_NET_RAW, or
//     SetPrivileged(false) + a permissive sysctl on Linux). Falls
//     outside what an unprivileged script can usually do, so it's
//     opt-in.
//
// Result shape (RTTs in milliseconds):
//
//	{ host, ip, mode, sent, received, lossPercent, minMs, avgMs, maxMs }
//
// A host that's completely unreachable resolves with received:0 and
// lossPercent:100 rather than throwing — "down" is a normal probe
// outcome. DNS-resolution failure and bad arguments throw.
func pingProbe(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	host := call.Argument(0).String()
	if host == "" {
		return nil, errors.New("net.ping: host required")
	}
	opts := optsAsMap(call)
	count := optInt(opts, "count", 4)
	if count <= 0 {
		count = 4
	}
	timeout := optMillis(opts, "timeout", 5*time.Second)
	mode := optString(opts, "mode", "tcp")

	switch mode {
	case "tcp":
		return tcpPing(ctx, host, optString(opts, "port", "80"), count, timeout)
	case "icmp":
		return icmpPing(ctx, host, count, timeout)
	default:
		return nil, fmt.Errorf("net.ping: mode must be 'tcp' or 'icmp', got %q", mode)
	}
}

// tcpPing opens `count` TCP connections to host:port and records each
// connect duration. Failed dials count as lost packets. This is the
// portable default — no privileges, works in containers / CI.
func tcpPing(ctx context.Context, host, port string, count int, timeout time.Duration) (map[string]any, error) {
	// Resolve once up front so a bad hostname errors clearly rather
	// than failing `count` times.
	addr := net.JoinHostPort(host, port)
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("net.ping: resolve %q: %w", host, err)
	}
	resolvedIP := ""
	if len(ips) > 0 {
		resolvedIP = ips[0].IP.String()
	}

	var rtts []time.Duration
	dialer := net.Dialer{Timeout: timeout}
	for i := 0; i < count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("net.ping: %w", err)
		}
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			continue // lost packet
		}
		rtts = append(rtts, time.Since(start))
		_ = conn.Close()
	}
	return pingStats(host, resolvedIP, "tcp", count, rtts), nil
}

// icmpPing runs a real ICMP echo session via pro-bing. Privileged
// mode is requested (the common case for ICMP on macOS / when run as
// root); on Linux unprivileged-ICMP via the datagram socket works
// when net.ipv4.ping_group_range permits, which SetPrivileged(false)
// would target — but the privileged path is the more portable
// default for a probe binding.
func icmpPing(ctx context.Context, host string, count int, timeout time.Duration) (map[string]any, error) {
	pinger, err := probing.NewPinger(host)
	if err != nil {
		return nil, fmt.Errorf("net.ping: %w", err)
	}
	pinger.Count = count
	pinger.Timeout = timeout
	pinger.SetPrivileged(true)

	if err := pinger.RunWithContext(ctx); err != nil {
		return nil, fmt.Errorf("net.ping: icmp run (needs raw-socket privileges?): %w", err)
	}
	st := pinger.Statistics()
	ip := ""
	if st.IPAddr != nil {
		ip = st.IPAddr.String()
	}
	return map[string]any{
		"host":        host,
		"ip":          ip,
		"mode":        "icmp",
		"sent":        st.PacketsSent,
		"received":    st.PacketsRecv,
		"lossPercent": st.PacketLoss,
		"minMs":       float64(st.MinRtt) / float64(time.Millisecond),
		"avgMs":       float64(st.AvgRtt) / float64(time.Millisecond),
		"maxMs":       float64(st.MaxRtt) / float64(time.Millisecond),
	}, nil
}

// pingStats computes the result map from a slice of successful RTTs.
// Shared shape between the TCP and ICMP paths (ICMP builds its own
// from pro-bing's Statistics, but the field names match).
func pingStats(host, ip, mode string, sent int, rtts []time.Duration) map[string]any {
	received := len(rtts)
	loss := 100.0
	if sent > 0 {
		loss = float64(sent-received) / float64(sent) * 100
	}
	var minRtt, maxRtt, sum time.Duration
	for i, rtt := range rtts {
		if i == 0 || rtt < minRtt {
			minRtt = rtt
		}
		if rtt > maxRtt {
			maxRtt = rtt
		}
		sum += rtt
	}
	var avg time.Duration
	if received > 0 {
		avg = sum / time.Duration(received)
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	return map[string]any{
		"host":        host,
		"ip":          ip,
		"mode":        mode,
		"sent":        sent,
		"received":    received,
		"lossPercent": loss,
		"minMs":       ms(minRtt),
		"avgMs":       ms(avg),
		"maxMs":       ms(maxRtt),
	}
}
