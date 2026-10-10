package telemetry

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// pcapngMagic starts a pcapng file; anything else is read as classic pcap.
var pcapngMagic = []byte{0x0A, 0x0D, 0x0D, 0x0A}

// readPcap counts a packet capture, pcap or pcapng, into per-second
// metrics:
//
//	packets, bytes                    everything captured (bytes as sent,
//	                                  even if the capture cut packets short)
//	tcp_packets, tcp_bytes, udp_...,  by protocol; icmp counts ICMP and
//	icmp_..., other_...               ICMPv6, other anything else
//	tcp_syn, tcp_rst, tcp_fin         TCP connections opened (a SYN without
//	                                  an ACK), reset and closed
//	hosts                             distinct source addresses
//	flows                             distinct conversations: protocol and
//	                                  both ends' addresses and ports
//	dns_queries, dns_failures         DNS queries, and answers saying the
//	                                  name doesn't exist or the server
//	                                  failed (NXDOMAIN, SERVFAIL)
func readPcap(r io.Reader) ([]sample, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	magic, _ := br.Peek(4)

	type packetReader interface {
		ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	}
	var pr packetReader
	linkType := func(gopacket.CaptureInfo) layers.LinkType { return 0 }

	if bytes.Equal(magic, pcapngMagic) {
		ng, err := pcapgo.NewNgReader(br, pcapgo.DefaultNgReaderOptions)
		if err != nil {
			return nil, fmt.Errorf("pcapng: %w", err)
		}
		pr = ng
		// Each interface in a pcapng file has its own link type.
		linkType = func(ci gopacket.CaptureInfo) layers.LinkType {
			if iface, err := ng.Interface(ci.InterfaceIndex); err == nil {
				return iface.LinkType
			}
			return ng.LinkType()
		}
	} else {
		pc, err := pcapgo.NewReader(br)
		if err != nil {
			return nil, fmt.Errorf("pcap: %w", err)
		}
		pr = pc
		linkType = func(gopacket.CaptureInfo) layers.LinkType { return pc.LinkType() }
	}

	t := newTally()
	for n := 1; ; n++ {
		data, ci, err := pr.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				break // a capture cut short: keep what's there
			}
			return nil, fmt.Errorf("packet %d: %w", n, err)
		}
		countPacket(t, data, ci, linkType(ci))
	}
	return t.samples(), nil
}

func countPacket(t *tally, data []byte, ci gopacket.CaptureInfo, link layers.LinkType) {
	at := float64(ci.Timestamp.UnixNano()) / 1e9
	size := float64(ci.Length)
	t.add(at, "packets", 1)
	t.add(at, "bytes", size)

	packet := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	var src, dst string
	if net := packet.NetworkLayer(); net != nil {
		flow := net.NetworkFlow()
		src, dst = flow.Src().String(), flow.Dst().String()
		t.count(at, "hosts", src)
	}

	proto := "other"
	var sport, dport string
	switch l := packet.TransportLayer().(type) {
	case *layers.TCP:
		proto = "tcp"
		sport, dport = l.SrcPort.String(), l.DstPort.String()
		if l.SYN && !l.ACK {
			t.add(at, "tcp_syn", 1)
		}
		if l.RST {
			t.add(at, "tcp_rst", 1)
		}
		if l.FIN {
			t.add(at, "tcp_fin", 1)
		}
	case *layers.UDP:
		proto = "udp"
		sport, dport = l.SrcPort.String(), l.DstPort.String()
	default:
		if packet.Layer(layers.LayerTypeICMPv4) != nil || packet.Layer(layers.LayerTypeICMPv6) != nil {
			proto = "icmp"
		}
	}
	t.add(at, proto+"_packets", 1)
	t.add(at, proto+"_bytes", size)

	if src != "" {
		// The same conversation either way round.
		a, b := src+" "+sport, dst+" "+dport
		if b < a {
			a, b = b, a
		}
		t.count(at, "flows", proto+" "+a+" "+b)
	}

	if l := packet.Layer(layers.LayerTypeDNS); l != nil {
		dns := l.(*layers.DNS)
		switch {
		case !dns.QR:
			t.add(at, "dns_queries", 1)
		case dns.ResponseCode == layers.DNSResponseCodeNXDomain || dns.ResponseCode == layers.DNSResponseCodeServFail:
			t.add(at, "dns_failures", 1)
		}
	}
}
