// Package osc sends the theme engine's events as Open Sound Control
// messages over UDP, for SuperCollider, Max, Pure Data, TouchDesigner or
// anything else that speaks OSC to play, or react to, as it likes.
//
// Each event is one message, addressed by the sound that made it, so a
// receiver can route sounds with a single pattern match:
//
//	/soundscape/<sound>/note     i:pitch i:velocity i:duration_ms i:channel
//	/soundscape/<sound>/sample   s:group f:pitch f:gain f:pan i:duration_ms i:channel
//	/soundscape/<sound>/loop     s:group f:pitch f:gain f:pan i:channel
//	/soundscape/<sound>/control  f:value i:controller i:channel
//	/soundscape/program          i:channel i:bank i:program
//
// A sample's group is its sample_group directory's name (e.g. "birds"),
// and its pitch a playback ratio (1 = as recorded). A loop message is sent
// every tick while the loop plays, with its current gain, so it works as a
// continuous control as well. "/soundscape" can be changed with the
// prefix, and characters OSC doesn't allow in an address (space and
// #*,/?[]{}) become "_" in a sound's name.
package osc

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"path/filepath"
	"strings"

	"github.com/simonwistow/soundscape/internal/event"
)

// Output sends events to one OSC receiver.
type Output struct {
	conn   net.Conn
	prefix string
}

// Dial sets up sending to the OSC receiver at addr ("host:port"), with
// every address starting with prefix. UDP has no connection, so this
// succeeds whether or not anything is listening yet.
func Dial(addr, prefix string) (*Output, error) {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("osc: %w", err)
	}
	return &Output{conn: conn, prefix: strings.TrimRight(prefix, "/")}, nil
}

// String names the receiver, for logging.
func (o *Output) String() string {
	return o.conn.RemoteAddr().String()
}

func (o *Output) Send(e event.Event) error {
	msg, err := o.message(e)
	if err != nil {
		return err
	}
	// A receiver that isn't listening yet is no reason to stop: like a
	// synth that's switched off, it just misses what's sent meanwhile.
	_, _ = o.conn.Write(msg)
	return nil
}

func (o *Output) Close() error {
	return o.conn.Close()
}

func (o *Output) message(e event.Event) ([]byte, error) {
	switch ev := e.(type) {
	case event.Note:
		return encode(o.address(ev.Actor, "note"),
			int32(ev.Pitch), int32(ev.Velocity), int32(ev.DurationMs), int32(ev.Channel)), nil
	case event.Sample:
		group := filepath.Base(ev.Group)
		if ev.Loop {
			return encode(o.address(ev.Actor, "loop"),
				group, float32(ev.Pitch), float32(ev.Velocity), float32(ev.Pan), int32(ev.Channel)), nil
		}
		return encode(o.address(ev.Actor, "sample"),
			group, float32(ev.Pitch), float32(ev.Velocity), float32(ev.Pan), int32(ev.DurationMs), int32(ev.Channel)), nil
	case event.Control:
		return encode(o.address(ev.Actor, "control"),
			float32(ev.Value), int32(ev.Controller), int32(ev.Channel)), nil
	case event.Program:
		return encode(o.prefix+"/program", int32(ev.Channel), int32(ev.Bank), int32(ev.Program)), nil
	default:
		return nil, fmt.Errorf("osc: unsupported event %T", e)
	}
}

func (o *Output) address(actor, kind string) string {
	return o.prefix + "/" + addressSafe.Replace(actor) + "/" + kind
}

// addressSafe replaces the characters OSC reserves in an address part.
var addressSafe = strings.NewReplacer(
	" ", "_", "#", "_", "*", "_", ",", "_", "/", "_",
	"?", "_", "[", "_", "]", "_", "{", "_", "}", "_",
)

// encode builds an OSC 1.0 message from int32, float32 and string
// arguments.
func encode(address string, args ...any) []byte {
	tags := ","
	var data []byte
	for _, a := range args {
		switch v := a.(type) {
		case int32:
			tags += "i"
			data = binary.BigEndian.AppendUint32(data, uint32(v))
		case float32:
			tags += "f"
			data = binary.BigEndian.AppendUint32(data, math.Float32bits(v))
		case string:
			tags += "s"
			data = appendString(data, v)
		default:
			panic(fmt.Sprintf("osc: can't encode %T", a))
		}
	}
	msg := appendString(nil, address)
	msg = appendString(msg, tags)
	return append(msg, data...)
}

// appendString appends s as an OSC string: NUL-terminated, then padded
// with NULs to a multiple of four bytes.
func appendString(b []byte, s string) []byte {
	b = append(b, s...)
	return append(b, make([]byte, 4-len(s)%4)...)
}
