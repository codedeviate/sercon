package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// TestUDP_ConnectedSendOnMessage opens a connected UDP socket to an in-process
// echo server (ReadFromUDP then WriteToUDP back to the sender), sends a
// datagram, and asserts onMessage fires with the echoed payload.
func TestUDP_ConnectedSendOnMessage(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	go func() {
		buf := make([]byte, 64)
		n, addr, err := pc.ReadFromUDP(buf)
		if err != nil {
			return
		}
		_, _ = pc.WriteToUDP(buf[:n], addr)
	}()

	host, port, _ := net.SplitHostPort(pc.LocalAddr().String())
	got := runSocketScript(t, fmt.Sprintf(`
		const u = await net.udp.open({ host: %q, port: %s });
		const out = await new Promise(res => { u.onMessage(ev => res(ev.text)); u.send("pong"); });
		await u.close();
		__capture(out);
	`, host, port))
	if got != "pong" {
		t.Errorf("udp echo: got %q want %q", got, "pong")
	}
}

// TestUDP_BoundReceivesFromSender opens a bound UDP socket on an ephemeral
// port, reads back its `local` address to learn the port, then an external Go
// UDP sender sends a datagram. Asserts onMessage fires with the payload and a
// non-empty sender address in the event meta.
func TestUDP_BoundReceivesFromSender(t *testing.T) {
	// Discover a free UDP port in Go, then hand it to the script so the
	// external sender knows where to aim before the script has resolved.
	probe, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, _ := net.SplitHostPort(probe.LocalAddr().String())
	_ = probe.Close()

	// External sender: retry until the bound socket is up.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort("127.0.0.1", portStr))
		if err != nil {
			return
		}
		conn, err := net.DialUDP("udp", nil, raddr)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = conn.Write([]byte("hello-udp"))
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()

	got := runSocketScript(t, fmt.Sprintf(`
		const u = await net.udp.open({ bind: "127.0.0.1:%s" });
		const out = await new Promise(res => {
			u.onMessage(ev => res({ text: ev.text, address: ev.address, port: ev.port }));
		});
		await u.close();
		__capture(JSON.stringify(out));
	`, portStr))

	gotStr, ok := got.(string)
	if !ok {
		t.Fatalf("expected JSON string, got %T (%v)", got, got)
	}
	if !strings.Contains(gotStr, `"text":"hello-udp"`) {
		t.Errorf("bound recv: payload missing in %q", gotStr)
	}
	if !strings.Contains(gotStr, `"address":"127.0.0.1"`) {
		t.Errorf("bound recv: sender address missing/wrong in %q", gotStr)
	}
}
