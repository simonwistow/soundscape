package telemetry

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// packetAt is a packet to capture, at a whole number of seconds from T.
type packetAt struct {
	sec    int64
	layers []gopacket.SerializableLayer
}

var (
	macA, macB = net.HardwareAddr{2, 0, 0, 0, 0, 1}, net.HardwareAddr{2, 0, 0, 0, 0, 2}
	client     = net.IP{10, 0, 0, 1}
	server     = net.IP{10, 0, 0, 2}
	resolver   = net.IP{8, 8, 8, 8}
)

func ipv4(src, dst net.IP, proto layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{Version: 4, TTL: 64, Protocol: proto, SrcIP: src, DstIP: dst}
}

func eth(t layers.EthernetType) *layers.Ethernet {
	return &layers.Ethernet{SrcMAC: macA, DstMAC: macB, EthernetType: t}
}

func tcp(src, dst net.IP, sport, dport layers.TCPPort, flags func(*layers.TCP)) []gopacket.SerializableLayer {
	ip := ipv4(src, dst, layers.IPProtocolTCP)
	t := &layers.TCP{SrcPort: sport, DstPort: dport, Window: 1024}
	flags(t)
	t.SetNetworkLayerForChecksum(ip)
	return []gopacket.SerializableLayer{eth(layers.EthernetTypeIPv4), ip, t}
}

func dns(src, dst net.IP, sport, dport layers.UDPPort, msg *layers.DNS) []gopacket.SerializableLayer {
	ip := ipv4(src, dst, layers.IPProtocolUDP)
	u := &layers.UDP{SrcPort: sport, DstPort: dport}
	u.SetNetworkLayerForChecksum(ip)
	return []gopacket.SerializableLayer{eth(layers.EthernetTypeIPv4), ip, u, msg}
}

// capture is a little traffic: a TCP connection opened, answered, reset and
// closed, a DNS lookup that fails, a ping, and an IPv6 datagram.
func capture() []packetAt {
	question := []layers.DNSQuestion{{Name: []byte("nowhere.example"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}
	ip6 := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolUDP,
		SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
	u6 := &layers.UDP{SrcPort: 4000, DstPort: 4001}
	u6.SetNetworkLayerForChecksum(ip6)

	return []packetAt{
		{0, tcp(client, server, 50000, 80, func(t *layers.TCP) { t.SYN = true })},
		{0, tcp(server, client, 80, 50000, func(t *layers.TCP) { t.SYN, t.ACK = true, true })},
		{0, dns(client, resolver, 5353, 53, &layers.DNS{ID: 1, RD: true, Questions: question})},
		{1, dns(resolver, client, 53, 5353, &layers.DNS{ID: 1, QR: true, ResponseCode: layers.DNSResponseCodeNXDomain, Questions: question})},
		{1, []gopacket.SerializableLayer{eth(layers.EthernetTypeIPv4), ipv4(client, server, layers.IPProtocolICMPv4),
			&layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0)}}},
		{2, tcp(server, client, 80, 50000, func(t *layers.TCP) { t.RST = true })},
		{2, tcp(client, server, 50001, 80, func(t *layers.TCP) { t.FIN, t.ACK = true, true })},
		{2, []gopacket.SerializableLayer{eth(layers.EthernetTypeIPv6), ip6, u6, gopacket.Payload("hello")}},
	}
}

// writeCapture writes packets as pcap or, with ng, pcapng, and returns
// each packet's length.
func writeCapture(t *testing.T, path string, ng bool, packets []packetAt) []int {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	write := func(ci gopacket.CaptureInfo, data []byte) error { return nil }
	var flush func() error
	if ng {
		w, err := pcapgo.NewNgWriter(f, layers.LinkTypeEthernet)
		if err != nil {
			t.Fatal(err)
		}
		write, flush = w.WritePacket, w.Flush
	} else {
		w := pcapgo.NewWriter(f)
		if err := w.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
			t.Fatal(err)
		}
		write, flush = w.WritePacket, func() error { return nil }
	}

	var sizes []int
	for _, p := range packets {
		buf := gopacket.NewSerializeBuffer()
		if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, p.layers...); err != nil {
			t.Fatal(err)
		}
		data := buf.Bytes()
		ci := gopacket.CaptureInfo{Timestamp: time.Unix(1696000000+p.sec, 250e6), CaptureLength: len(data), Length: len(data)}
		if err := write(ci, data); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(data))
	}
	if err := flush(); err != nil {
		t.Fatal(err)
	}
	return sizes
}

func TestPcap(t *testing.T) {
	for _, ng := range []bool{false, true} {
		name := "capture.pcap"
		if ng {
			name = "capture.pcapng"
		}
		path := filepath.Join(t.TempDir(), name)
		s := writeCapture(t, path, ng, capture())
		got := ticks(t, path, "")

		sum := func(i ...int) float64 {
			n := 0
			for _, j := range i {
				n += s[j]
			}
			return float64(n)
		}
		const T = 1696000000
		want := map[int64]map[string]float64{
			T: {"packets": 3, "bytes": sum(0, 1, 2), "tcp_packets": 2, "tcp_bytes": sum(0, 1), "udp_packets": 1, "udp_bytes": sum(2),
				"tcp_syn": 1, "dns_queries": 1, "hosts": 2, "flows": 2},
			T + 1: {"packets": 2, "bytes": sum(3, 4), "udp_packets": 1, "udp_bytes": sum(3), "icmp_packets": 1, "icmp_bytes": sum(4),
				"dns_failures": 1, "hosts": 2, "flows": 2},
			T + 2: {"packets": 3, "bytes": sum(5, 6, 7), "tcp_packets": 2, "tcp_bytes": sum(5, 6), "udp_packets": 1, "udp_bytes": sum(7),
				"tcp_rst": 1, "tcp_fin": 1, "hosts": 3, "flows": 3},
		}
		// A count seen before but missing from a second is 0 there.
		seen := map[string]bool{}
		for sec := int64(T); sec <= T+2; sec++ {
			for k := range seen {
				if _, ok := want[sec][k]; !ok {
					want[sec][k] = 0
				}
			}
			for k := range want[sec] {
				seen[k] = true
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\ngot  %v\nwant %v", name, got, want)
		}
	}
}

func TestPcapCutShort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.pcap")
	writeCapture(t, path, false, capture())
	b, _ := os.ReadFile(path)
	os.WriteFile(path, b[:len(b)-10], 0o644)

	got := ticks(t, path, "")
	if got[1696000000]["packets"] != 3 {
		t.Errorf("a capture cut short lost its start: %v", got)
	}
}

func TestPcapNotACapture(t *testing.T) {
	if _, err := Read(write(t, "a.pcap", "not a capture at all"), ""); err == nil {
		t.Error("no error for a file that isn't a capture")
	}
}
