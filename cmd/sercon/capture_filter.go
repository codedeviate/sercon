package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// capture_filter.go is a pure-Go, tcpdump-SYNTAX filter compiler for
// net.capture. It is a userspace post-decode predicate, NOT a kernel BPF
// program: compileFilter turns an expression like "tcp and port 80" into a
// predicate AST, and (*captureFilter).match evaluates it against an
// already-decoded gopacket.Packet. The supported subset:
//
//	proto      tcp | udp | icmp | ip | ip6
//	host       [src|dst] host <ip>        (IPv4 or IPv6)
//	net        [src|dst] net <cidr>       (IPv4 or IPv6 prefix, e.g. 10.0.0.0/8)
//	port       [src|dst] port <number>
//	portrange  [src|dst] portrange <lo>-<hi>
//	boolean    not > and > or, with parentheses
//	implicit   juxtaposed primaries imply 'and' (tcpdump-style:
//	           "tcp port 80" == "tcp and port 80")

// captureFilter wraps the root predicate of a compiled filter expression.
type captureFilter struct {
	root predicate
}

// match reports whether pkt satisfies the compiled filter.
func (f *captureFilter) match(pkt gopacket.Packet) bool {
	if f == nil || f.root == nil {
		return true
	}
	return f.root.match(pkt)
}

// predicate is one node of the filter AST.
type predicate interface {
	match(pkt gopacket.Packet) bool
}

// protoPred matches on the presence of a protocol layer. kind is one of
// "tcp", "udp", "icmp", "ip", "ip6".
type protoPred struct{ kind string }

func (p protoPred) match(pkt gopacket.Packet) bool {
	switch p.kind {
	case "tcp":
		return pkt.Layer(layers.LayerTypeTCP) != nil
	case "udp":
		return pkt.Layer(layers.LayerTypeUDP) != nil
	case "icmp":
		return pkt.Layer(layers.LayerTypeICMPv4) != nil || pkt.Layer(layers.LayerTypeICMPv6) != nil
	case "ip":
		return pkt.Layer(layers.LayerTypeIPv4) != nil
	case "ip6":
		return pkt.Layer(layers.LayerTypeIPv6) != nil
	}
	return false
}

// hostPred matches a source/destination IP. dir is "" (either), "src", or
// "dst".
type hostPred struct {
	dir string
	ip  net.IP
}

func (p hostPred) match(pkt gopacket.Packet) bool {
	src, dst, ok := packetIPs(pkt)
	if !ok {
		return false
	}
	switch p.dir {
	case "src":
		return src.Equal(p.ip)
	case "dst":
		return dst.Equal(p.ip)
	default:
		return src.Equal(p.ip) || dst.Equal(p.ip)
	}
}

// portPred matches a TCP/UDP source/destination port. dir is "" (either),
// "src", or "dst".
type portPred struct {
	dir  string
	port int
}

func (p portPred) match(pkt gopacket.Packet) bool {
	src, dst, ok := packetPorts(pkt)
	if !ok {
		return false
	}
	switch p.dir {
	case "src":
		return src == p.port
	case "dst":
		return dst == p.port
	default:
		return src == p.port || dst == p.port
	}
}

// netPred matches a source/destination IP against a CIDR prefix. dir is ""
// (either), "src", or "dst".
type netPred struct {
	dir   string
	ipnet *net.IPNet
}

func (p netPred) match(pkt gopacket.Packet) bool {
	src, dst, ok := packetIPs(pkt)
	if !ok {
		return false
	}
	switch p.dir {
	case "src":
		return p.ipnet.Contains(src)
	case "dst":
		return p.ipnet.Contains(dst)
	default:
		return p.ipnet.Contains(src) || p.ipnet.Contains(dst)
	}
}

// portRangePred matches a TCP/UDP source/destination port against an
// inclusive [lo, hi] range. dir is "" (either), "src", or "dst".
type portRangePred struct {
	dir    string
	lo, hi int
}

func (p portRangePred) match(pkt gopacket.Packet) bool {
	src, dst, ok := packetPorts(pkt)
	if !ok {
		return false
	}
	in := func(n int) bool { return n >= p.lo && n <= p.hi }
	switch p.dir {
	case "src":
		return in(src)
	case "dst":
		return in(dst)
	default:
		return in(src) || in(dst)
	}
}

// andPred / orPred / notPred are the boolean combinators.
type andPred struct{ a, b predicate }

func (p andPred) match(pkt gopacket.Packet) bool { return p.a.match(pkt) && p.b.match(pkt) }

type orPred struct{ a, b predicate }

func (p orPred) match(pkt gopacket.Packet) bool { return p.a.match(pkt) || p.b.match(pkt) }

type notPred struct{ p predicate }

func (p notPred) match(pkt gopacket.Packet) bool { return !p.p.match(pkt) }

// packetIPs extracts source/destination IPs from the IPv4 or IPv6 layer.
// ok is false when the packet has no recognised network layer.
func packetIPs(pkt gopacket.Packet) (src, dst net.IP, ok bool) {
	if l, _ := pkt.Layer(layers.LayerTypeIPv4).(*layers.IPv4); l != nil {
		return l.SrcIP, l.DstIP, true
	}
	if l, _ := pkt.Layer(layers.LayerTypeIPv6).(*layers.IPv6); l != nil {
		return l.SrcIP, l.DstIP, true
	}
	return nil, nil, false
}

// packetPorts extracts source/destination ports from the TCP or UDP layer.
// ok is false when the packet has no recognised transport layer.
func packetPorts(pkt gopacket.Packet) (src, dst int, ok bool) {
	if l, _ := pkt.Layer(layers.LayerTypeTCP).(*layers.TCP); l != nil {
		return int(l.SrcPort), int(l.DstPort), true
	}
	if l, _ := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP); l != nil {
		return int(l.SrcPort), int(l.DstPort), true
	}
	return 0, 0, false
}

// --- lexer -----------------------------------------------------------------

// keywords is the set of reserved words. Any token not in this set is treated
// as a value (an IP or a number operand for host/port).
var keywords = map[string]bool{
	"tcp": true, "udp": true, "icmp": true, "ip": true, "ip6": true,
	"host": true, "net": true, "port": true, "portrange": true,
	"src": true, "dst": true,
	"and": true, "or": true, "not": true,
	"(": true, ")": true,
}

// lex splits expr into tokens on whitespace, treating '(' and ')' as their own
// tokens even when adjacent to a word ("(tcp" -> "(", "tcp"; "tcp)" -> "tcp",
// ")").
func lex(expr string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range expr {
		switch r {
		case '(', ')':
			flush()
			toks = append(toks, string(r))
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

// --- parser (recursive descent, precedence not > and > or) -----------------

type parser struct {
	toks []string
	pos  int
}

func (p *parser) peek() string {
	if p.pos >= len(p.toks) {
		return ""
	}
	return p.toks[p.pos]
}

func (p *parser) next() string {
	t := p.peek()
	if p.pos < len(p.toks) {
		p.pos++
	}
	return t
}

func (p *parser) eof() bool { return p.pos >= len(p.toks) }

// startsPrimary reports whether the current token can begin a primary — used
// to detect implicit 'and' between juxtaposed primaries.
func startsPrimary(tok string) bool {
	switch tok {
	case "(", "not", "tcp", "udp", "icmp", "ip", "ip6",
		"host", "net", "port", "portrange", "src", "dst":
		return true
	}
	return false
}

// compileFilter parses a tcpdump-syntax expression into a *captureFilter.
// A malformed expression (unexpected/trailing token, missing operand,
// non-IP host, non-numeric port, unbalanced parens, unknown keyword) returns
// a non-nil error.
func compileFilter(expr string) (*captureFilter, error) {
	toks := lex(expr)
	if len(toks) == 0 {
		return nil, fmt.Errorf("capture filter: empty expression")
	}
	p := &parser{toks: toks}
	root, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, fmt.Errorf("capture filter: unexpected token %q", p.peek())
	}
	return &captureFilter{root: root}, nil
}

// parseOr := parseAnd ('or' parseAnd)*
func (p *parser) parseOr() (predicate, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "or" {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orPred{a: left, b: right}
	}
	return left, nil
}

// parseAnd := parseNot (('and')? parseNot)*  — stops at 'or', ')', EOF.
func (p *parser) parseAnd() (predicate, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		tok := p.peek()
		if tok == "and" {
			p.next()
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			left = andPred{a: left, b: right}
			continue
		}
		// Implicit 'and' between juxtaposed primaries; stop at or/)/EOF.
		if startsPrimary(tok) {
			right, err := p.parseNot()
			if err != nil {
				return nil, err
			}
			left = andPred{a: left, b: right}
			continue
		}
		break
	}
	return left, nil
}

// parseNot := 'not' parseNot | parsePrimary
func (p *parser) parseNot() (predicate, error) {
	if p.peek() == "not" {
		p.next()
		inner, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return notPred{p: inner}, nil
	}
	return p.parsePrimary()
}

// parsePrimary := '(' parseOr ')' | proto | dir? 'host' value | dir? 'port' number
func (p *parser) parsePrimary() (predicate, error) {
	tok := p.peek()
	if tok == "" {
		return nil, fmt.Errorf("capture filter: unexpected end of expression")
	}

	if tok == "(" {
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("capture filter: missing closing ')'")
		}
		p.next()
		return inner, nil
	}

	switch tok {
	case "tcp", "udp", "icmp", "ip", "ip6":
		p.next()
		return protoPred{kind: tok}, nil
	}

	// Optional direction prefix for host/port.
	dir := ""
	if tok == "src" || tok == "dst" {
		dir = tok
		p.next()
		tok = p.peek()
	}

	switch tok {
	case "host":
		p.next()
		val := p.next()
		if val == "" || keywords[val] {
			return nil, fmt.Errorf("capture filter: 'host' requires an IP operand")
		}
		ip := net.ParseIP(val)
		if ip == nil {
			return nil, fmt.Errorf("capture filter: invalid host IP %q", val)
		}
		return hostPred{dir: dir, ip: ip}, nil
	case "net":
		p.next()
		val := p.next()
		if val == "" || keywords[val] {
			return nil, fmt.Errorf("capture filter: 'net' requires a CIDR operand")
		}
		_, ipnet, err := net.ParseCIDR(val)
		if err != nil {
			return nil, fmt.Errorf("capture filter: invalid CIDR %q", val)
		}
		return netPred{dir: dir, ipnet: ipnet}, nil
	case "port":
		p.next()
		val := p.next()
		if val == "" || keywords[val] {
			return nil, fmt.Errorf("capture filter: 'port' requires a numeric operand")
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("capture filter: invalid port %q", val)
		}
		return portPred{dir: dir, port: n}, nil
	case "portrange":
		p.next()
		val := p.next()
		if val == "" || keywords[val] {
			return nil, fmt.Errorf("capture filter: 'portrange' requires a LOW-HIGH operand")
		}
		lo, hi, err := parsePortRange(val)
		if err != nil {
			return nil, err
		}
		return portRangePred{dir: dir, lo: lo, hi: hi}, nil
	}

	if dir != "" {
		return nil, fmt.Errorf("capture filter: %q must be followed by 'host', 'net', 'port', or 'portrange'", dir)
	}
	return nil, fmt.Errorf("capture filter: unexpected token %q", tok)
}

// parsePortRange parses a "LOW-HIGH" operand (e.g. "80-443") into an
// inclusive integer range. Both bounds must be present and numeric, and
// low must not exceed high.
func parsePortRange(s string) (lo, hi int, err error) {
	dash := strings.IndexByte(s, '-')
	if dash <= 0 || dash == len(s)-1 {
		return 0, 0, fmt.Errorf("capture filter: invalid portrange %q (want LOW-HIGH)", s)
	}
	lo, err = strconv.Atoi(s[:dash])
	if err != nil {
		return 0, 0, fmt.Errorf("capture filter: invalid portrange low %q", s[:dash])
	}
	hi, err = strconv.Atoi(s[dash+1:])
	if err != nil {
		return 0, 0, fmt.Errorf("capture filter: invalid portrange high %q", s[dash+1:])
	}
	if lo > hi {
		return 0, 0, fmt.Errorf("capture filter: portrange low %d exceeds high %d", lo, hi)
	}
	return lo, hi, nil
}
