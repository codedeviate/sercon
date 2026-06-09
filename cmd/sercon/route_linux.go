//go:build linux

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// hostRoutes reads the Linux routing table from procfs: /proc/net/route for
// IPv4 and /proc/net/ipv6_route for IPv6. Both are pure text; no cgo, no
// netlink. A missing IPv6 file (IPv6 disabled) is not an error — IPv4 routes
// are still returned.
func hostRoutes() ([]routeEntry, error) {
	v4, err := readProcRoute4("/proc/net/route")
	if err != nil {
		return nil, err
	}
	v6, err := readProcRoute6("/proc/net/ipv6_route")
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return append(v4, v6...), nil
}

// readProcRoute4 parses /proc/net/route. Columns (tab/space separated):
//
//	Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT
//
// Destination, Gateway and Mask are little-endian hex IPv4 words.
func readProcRoute4(path string) ([]routeEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []routeEntry
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first { // header row
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 8 {
			continue
		}
		dst, ok1 := leHexIPv4(fields[1])
		gw, ok2 := leHexIPv4(fields[2])
		mask, ok3 := leHexIPv4(fields[7])
		if !ok1 || !ok2 || !ok3 {
			continue
		}
		ones, _ := net.IPMask(mask.To4()).Size()
		metric, _ := strconv.Atoi(fields[6])
		gwStr := ""
		if !gw.Equal(net.IPv4zero) {
			gwStr = gw.String()
		}
		out = append(out, routeEntry{
			Destination: fmt.Sprintf("%s/%d", dst.String(), ones),
			Gateway:     gwStr,
			Interface:   fields[0],
			Family:      "ip",
			Metric:      metric,
		})
	}
	return out, sc.Err()
}

// readProcRoute6 parses /proc/net/ipv6_route. Columns are space separated:
//
//	dest(32hex) destPrefixLen(hex) src(32hex) srcPrefixLen(hex) nextHop(32hex)
//	metric(hex) refcnt(hex) use(hex) flags(hex) iface
func readProcRoute6(path string) ([]routeEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []routeEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		dst, ok1 := hexIPv6(fields[0])
		nh, ok2 := hexIPv6(fields[4])
		if !ok1 || !ok2 {
			continue
		}
		plen, err := strconv.ParseInt(fields[1], 16, 0)
		if err != nil {
			continue
		}
		metric64, _ := strconv.ParseInt(fields[5], 16, 0)
		gwStr := ""
		if !nh.Equal(net.IPv6zero) {
			gwStr = nh.String()
		}
		out = append(out, routeEntry{
			Destination: fmt.Sprintf("%s/%d", dst.String(), plen),
			Gateway:     gwStr,
			Interface:   fields[9],
			Family:      "ip6",
			Metric:      int(metric64),
		})
	}
	return out, sc.Err()
}

// leHexIPv4 decodes a little-endian 8-hex-digit IPv4 word (as procfs stores
// it) into a net.IP.
func leHexIPv4(s string) (net.IP, bool) {
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, false
	}
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return net.IPv4(b[0], b[1], b[2], b[3]), true
}

// hexIPv6 decodes a 32-hex-digit (16-byte, big-endian) IPv6 address.
func hexIPv6(s string) (net.IP, bool) {
	if len(s) != 32 {
		return nil, false
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	ip := make(net.IP, net.IPv6len)
	copy(ip, b)
	return ip, true
}
