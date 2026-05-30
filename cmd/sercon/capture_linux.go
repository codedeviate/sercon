//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// linuxLiveSource adapts a *pcapgo.EthernetHandle (AF_PACKET) to liveSource.
// AF_PACKET frames are always Ethernet-framed, so LinkType is fixed.
type linuxLiveSource struct {
	h *pcapgo.EthernetHandle
}

func (s *linuxLiveSource) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	return s.h.ReadPacketData()
}

func (s *linuxLiveSource) LinkType() layers.LinkType { return layers.LinkTypeEthernet }

func (s *linuxLiveSource) Close() error { return s.h.Close() }

// openLiveCapture opens a live AF_PACKET capture on iface via
// pcapgo.NewEthernetHandle. Promiscuous mode and snaplen are applied to the
// handle. EPERM/EACCES (no CAP_NET_RAW / not root) are wrapped with a
// privilege hint.
func openLiveCapture(iface string, promisc bool, snaplen int) (liveSource, error) {
	h, err := pcapgo.NewEthernetHandle(iface)
	if err != nil {
		return nil, wrapLinuxCaptureErr(err)
	}
	if err := h.SetPromiscuous(promisc); err != nil {
		_ = h.Close()
		return nil, wrapLinuxCaptureErr(err)
	}
	if err := h.SetCaptureLength(snaplen); err != nil {
		_ = h.Close()
		return nil, wrapLinuxCaptureErr(err)
	}
	return &linuxLiveSource{h: h}, nil
}

// wrapLinuxCaptureErr adds a root / CAP_NET_RAW hint for permission errors and
// otherwise prefixes the binding name.
func wrapLinuxCaptureErr(err error) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("net.capture.open: %w (live capture needs root / CAP_NET_RAW on Linux)", err)
	}
	return fmt.Errorf("net.capture.open: %w", err)
}
