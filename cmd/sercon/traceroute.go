package main

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
