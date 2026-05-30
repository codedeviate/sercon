//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/bsdbpf"
	"github.com/gopacket/gopacket/layers"
)

// darwinLiveSource adapts a *bsdbpf.BPFSniffer (BPF /dev/bpfX) to liveSource.
// The bsdbpf sniffer does not expose its datalink type, so we default to
// Ethernet. This is correct for typical en* interfaces; loopback (lo0, which
// is DLT_NULL/LOOP) is mis-labelled as Ethernet here — acceptable for v1.
type darwinLiveSource struct {
	s *bsdbpf.BPFSniffer
}

func (s *darwinLiveSource) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	return s.s.ReadPacketData()
}

func (s *darwinLiveSource) LinkType() layers.LinkType { return layers.LinkTypeEthernet }

func (s *darwinLiveSource) Close() error { return s.s.Close() }

// openLiveCapture opens a live BPF capture on iface via bsdbpf.NewBPFSniffer.
// snaplen maps to the sniffer's ReadBufLen; promisc maps to Options.Promisc.
// Permission errors (no /dev/bpf access / not root) are wrapped with a
// privilege hint. NewBPFSniffer panics (rather than returning an error) when
// it cannot acquire any /dev/bpfX device — the common no-privilege case — so
// we recover and convert that into a clean rejection.
func openLiveCapture(iface string, promisc bool, snaplen int) (src liveSource, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("net.capture.open: %v (live capture needs root or /dev/bpf access on macOS)", r)
		}
	}()
	sniffer, err := bsdbpf.NewBPFSniffer(iface, &bsdbpf.Options{
		ReadBufLen: snaplen,
		Promisc:    promisc,
		Immediate:  true,
	})
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return nil, fmt.Errorf("net.capture.open: %w (live capture needs root or /dev/bpf access on macOS)", err)
		}
		return nil, fmt.Errorf("net.capture.open: %w", err)
	}
	return &darwinLiveSource{s: sniffer}, nil
}
