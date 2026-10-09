// Package midi turns note and control events into MIDI, two ways:
// VirtualOutput records them as a Standard MIDI File (.mid) that any DAW or
// player can open, and LiveOutput sends them to a MIDI port in real time.
//
// LiveOutput uses RtMidi (through gomidi's rtmididrv), which talks to the
// system's MIDI layer: CoreMIDI on macOS, ALSA on Linux. There's no pure-Go
// way to do that, so building this package needs cgo and a C++ compiler.
// Linux already needed cgo for ALSA audio, so the only new requirement is
// on macOS, where the Xcode command line tools provide it.
package midi

import (
	"bytes"
	"encoding/binary"
	"os"
	"sync"
	"time"

	"github.com/simonwistow/soundscape/internal/clock"
)

const (
	ticksPerQuarter     = 480
	tempoUsecPerQuarter = 500000 // 120 BPM
)

// ticksPerSecond converts wall-clock elapsed time into MIDI ticks, given the
// fixed tempo/division above.
func ticksPerSecond() float64 {
	return float64(ticksPerQuarter) / (float64(tempoUsecPerQuarter) / 1e6)
}

// Writer accumulates MIDI channel events (with delta-times derived from
// the time on its clock as they're appended) into a single-track Standard
// MIDI File.
type Writer struct {
	mu        sync.Mutex
	track     []byte
	clock     clock.Clock
	start     time.Time
	lastTicks uint32
}

func NewWriter(clk clock.Clock) *Writer {
	w := &Writer{clock: clk, start: clk.Now()}
	// A tempo meta event up front so a DAW's transport matches our
	// wall-clock-derived tick math.
	w.appendEvent(0, []byte{
		0xFF, 0x51, 0x03,
		byte((tempoUsecPerQuarter >> 16) & 0xFF),
		byte((tempoUsecPerQuarter >> 8) & 0xFF),
		byte(tempoUsecPerQuarter & 0xFF),
	})
	return w
}

func (w *Writer) NoteOn(channel, note, velocity int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendEvent(w.delta(), []byte{0x90 | clampNibble(channel), clampByte(note), clampByte(velocity)})
}

func (w *Writer) NoteOff(channel, note int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendEvent(w.delta(), []byte{0x80 | clampNibble(channel), clampByte(note), 0})
}

func (w *Writer) ControlChange(channel, controller, value int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendEvent(w.delta(), []byte{0xB0 | clampNibble(channel), clampByte(controller), clampByte(value)})
}

// ProgramChange selects bank and program on channel: a bank select
// controller change followed by a program change.
func (w *Writer) ProgramChange(channel, bank, program int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendEvent(w.delta(), []byte{0xB0 | clampNibble(channel), 0x00, clampByte(bank)})
	w.appendEvent(0, []byte{0xC0 | clampNibble(channel), clampByte(program)})
}

// delta returns the time since the last event, in MIDI ticks. It counts
// from the start, so rounding to whole ticks doesn't add up over a long
// session. Must be called with mu held.
func (w *Writer) delta() uint32 {
	ticks := uint32(max(w.clock.Now().Sub(w.start).Seconds(), 0) * ticksPerSecond())
	if ticks < w.lastTicks {
		return 0
	}
	d := ticks - w.lastTicks
	w.lastTicks = ticks
	return d
}

// appendEvent must be called with mu held.
func (w *Writer) appendEvent(delta uint32, event []byte) {
	w.track = append(w.track, encodeVLQ(delta)...)
	w.track = append(w.track, event...)
}

// WriteFile finalizes the accumulated events (adding an end-of-track meta
// event) and writes a complete Standard MIDI File to path.
func (w *Writer) WriteFile(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	track := append([]byte{}, w.track...)
	track = append(track, encodeVLQ(0)...)
	track = append(track, 0xFF, 0x2F, 0x00) // end of track

	var buf bytes.Buffer
	buf.WriteString("MThd")
	_ = binary.Write(&buf, binary.BigEndian, uint32(6))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // format 0: single track
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // ntrks
	_ = binary.Write(&buf, binary.BigEndian, uint16(ticksPerQuarter))

	buf.WriteString("MTrk")
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(track)))
	buf.Write(track)

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// encodeVLQ encodes a value as a MIDI variable-length quantity: 7 bits per
// byte, most-significant byte first, every byte but the last with its top
// bit set.
func encodeVLQ(value uint32) []byte {
	buf := []byte{byte(value & 0x7F)}
	value >>= 7
	for value > 0 {
		buf = append([]byte{byte(value&0x7F) | 0x80}, buf...)
		value >>= 7
	}
	return buf
}

func clampByte(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 127 {
		return 127
	}
	return byte(v)
}

func clampNibble(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 15 {
		return 15
	}
	return byte(v)
}
