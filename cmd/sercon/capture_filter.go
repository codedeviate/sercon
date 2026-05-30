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
//	port       [src|dst] port <number>
//	boolean    not > and > or, with parentheses
//	implicit   juxtaposed primaries imply 'and' (tcpdump-style:
//	           "tcp port 80" == "tcp and port 80")
//
// CIDR (net X/Y) and portrange are deliberately out of scope for this cycle.

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
	"host": true, "port": true, "src": true, "dst": true,
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
		switch {
		case r == '(' || r == ')':
			flush()
			toks = append(toks, string(r))
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
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
	case "(", "not", "tcp", "udp", "icmp", "ip", "ip6", "host", "port", "src", "dst":
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
	}

	if dir != "" {
		return nil, fmt.Errorf("capture filter: %q must be followed by 'host' or 'port'", dir)
	}
	return nil, fmt.Errorf("capture filter: unexpected token %q", tok)
}
