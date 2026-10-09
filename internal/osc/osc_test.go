package osc

import (
	"bytes"
	"encoding/binary"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
)

// The examples from the OSC 1.0 specification.
func TestEncodeMatchesTheSpec(t *testing.T) {
	got := encode("/oscillator/4/frequency", float32(440))
	want := []byte("/oscillator/4/frequency\x00,f\x00\x00\x43\xdc\x00\x00")
	if !bytes.Equal(got, want) {
		t.Fatalf("encode = %q, want %q", got, want)
	}

	got = encode("/foo", int32(1000), int32(-1), "hello", float32(1.234), float32(5.678))
	want = []byte("/foo\x00\x00\x00\x00,iisff\x00\x00" +
		"\x00\x00\x03\xe8\xff\xff\xff\xffhello\x00\x00\x00\x3f\x9d\xf3\xb6\x40\xb5\xb2\x2d")
	if !bytes.Equal(got, want) {
		t.Fatalf("encode = %q, want %q", got, want)
	}
}

// decode reads back a message made by encode, for the tests.
func decode(t *testing.T, msg []byte) (string, []any) {
	t.Helper()
	str := func() string {
		end := bytes.IndexByte(msg, 0)
		s := string(msg[:end])
		msg = msg[(end/4+1)*4:]
		return s
	}
	address, tags := str(), str()
	var args []any
	for _, tag := range tags[1:] {
		switch tag {
		case 'i':
			args = append(args, int32(binary.BigEndian.Uint32(msg)))
			msg = msg[4:]
		case 'f':
			args = append(args, math.Float32frombits(binary.BigEndian.Uint32(msg)))
			msg = msg[4:]
		case 's':
			args = append(args, str())
		default:
			t.Fatalf("unexpected type tag %q", tag)
		}
	}
	if len(msg) != 0 {
		t.Fatalf("%d bytes left over", len(msg))
	}
	return address, args
}

func TestEventsOverUDP(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	out, err := Dial(pc.LocalAddr().String(), "/soundscape/")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	tests := []struct {
		ev      event.Event
		address string
		args    []any
	}{
		{
			event.Note{Actor: "melody", Channel: 2, Pitch: 64, Velocity: 90, DurationMs: 700},
			"/soundscape/melody/note", []any{int32(64), int32(90), int32(700), int32(2)},
		},
		{
			event.Sample{Actor: "birds", Group: "/themes/forest/samples/birds", Pitch: 1.25, Velocity: 0.5, Pan: -0.75, DurationMs: 180},
			"/soundscape/birds/sample", []any{"birds", float32(1.25), float32(0.5), float32(-0.75), int32(180), int32(0)},
		},
		{
			event.Sample{Actor: "river gentle", Group: "samples/river-gentle", Loop: true, Pitch: 1, Velocity: 0.25},
			"/soundscape/river_gentle/loop", []any{"river-gentle", float32(1), float32(0.25), float32(0), int32(0)},
		},
		{
			event.Control{Actor: "wind", Channel: 1, Controller: 74, Value: 63.5},
			"/soundscape/wind/control", []any{float32(63.5), int32(74), int32(1)},
		},
		{
			event.Program{Channel: 3, Bank: 1, Program: 11},
			"/soundscape/program", []any{int32(3), int32(1), int32(11)},
		},
	}

	buf := make([]byte, 1024)
	for _, tt := range tests {
		if err := out.Send(tt.ev); err != nil {
			t.Fatalf("Send(%v): %v", tt.ev, err)
		}
		_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			t.Fatalf("receiving %v: %v", tt.ev, err)
		}
		if n%4 != 0 {
			t.Fatalf("message is %d bytes, not a multiple of 4", n)
		}
		address, args := decode(t, buf[:n])
		if address != tt.address {
			t.Errorf("address = %q, want %q", address, tt.address)
		}
		if len(args) != len(tt.args) {
			t.Fatalf("%s args = %v, want %v", address, args, tt.args)
		}
		for i := range args {
			if args[i] != tt.args[i] {
				t.Errorf("%s args = %v, want %v", address, args, tt.args)
				break
			}
		}
	}
}

func TestSendingToNobodyIsntAnError(t *testing.T) {
	// Find a port nothing's listening on.
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	pc.Close()

	out, err := Dial(addr, "/soundscape")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	for i := 0; i < 3; i++ {
		if err := out.Send(event.Note{Actor: "x", Pitch: 60}); err != nil {
			t.Fatalf("Send to a closed port: %v", err)
		}
	}
}

func TestAddressSafe(t *testing.T) {
	if got := addressSafe.Replace("a b#c*d,e/f?g[h]i{j}k"); strings.ContainsAny(got, " #*,/?[]{}") {
		t.Fatalf("addressSafe left reserved characters in %q", got)
	}
}
