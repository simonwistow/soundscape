package midi

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonwistow/soundscape/internal/clock"
	"github.com/simonwistow/soundscape/internal/event"
)

func TestEncodeVLQ(t *testing.T) {
	cases := []struct {
		value uint32
		want  []byte
	}{
		{0, []byte{0x00}},
		{64, []byte{0x40}},
		{127, []byte{0x7F}},
		{128, []byte{0x81, 0x00}},
		{8192, []byte{0xC0, 0x00}},
		{16383, []byte{0xFF, 0x7F}},
		{16384, []byte{0x81, 0x80, 0x00}},
	}
	for _, c := range cases {
		got := encodeVLQ(c.value)
		if !bytes.Equal(got, c.want) {
			t.Errorf("encodeVLQ(%d) = % X, want % X", c.value, got, c.want)
		}
	}
}

func TestWriterProducesValidSMFHeader(t *testing.T) {
	w := NewWriter(clock.Real{})
	w.NoteOn(0, 60, 100)
	w.NoteOff(0, 60)
	w.ControlChange(1, 74, 64)

	path := filepath.Join(t.TempDir(), "out.mid")
	if err := w.WriteFile(path); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data := readFile(t, path)

	if !bytes.HasPrefix(data, []byte("MThd")) {
		t.Fatalf("file does not start with MThd header")
	}
	headerLen := binary.BigEndian.Uint32(data[4:8])
	if headerLen != 6 {
		t.Fatalf("header length = %d, want 6", headerLen)
	}
	format := binary.BigEndian.Uint16(data[8:10])
	ntrks := binary.BigEndian.Uint16(data[10:12])
	division := binary.BigEndian.Uint16(data[12:14])
	if format != 0 {
		t.Fatalf("format = %d, want 0", format)
	}
	if ntrks != 1 {
		t.Fatalf("ntrks = %d, want 1", ntrks)
	}
	if division != ticksPerQuarter {
		t.Fatalf("division = %d, want %d", division, ticksPerQuarter)
	}

	trackChunk := data[14:]
	if !bytes.HasPrefix(trackChunk, []byte("MTrk")) {
		t.Fatalf("expected MTrk chunk after header, got %q", trackChunk[:4])
	}
	trackLen := binary.BigEndian.Uint32(trackChunk[4:8])
	trackBody := trackChunk[8:]
	if uint32(len(trackBody)) != trackLen {
		t.Fatalf("track body length = %d, want declared length %d", len(trackBody), trackLen)
	}

	// The track must end with an end-of-track meta event.
	if !bytes.HasSuffix(trackBody, []byte{0xFF, 0x2F, 0x00}) {
		t.Fatalf("track does not end with an end-of-track meta event")
	}
}

func TestNoteOnOffProducesCorrectStatusBytes(t *testing.T) {
	w := NewWriter(clock.Real{})
	w.NoteOn(2, 64, 100)
	w.NoteOff(2, 64)

	// Skip the tempo meta event written by NewWriter, then look for our
	// note-on/note-off status bytes further in the track.
	if !bytes.Contains(w.track, []byte{0x92, 64, 100}) {
		t.Fatalf("expected a note-on status byte 0x92 (channel 2) in track data")
	}
	if !bytes.Contains(w.track, []byte{0x82, 64, 0}) {
		t.Fatalf("expected a note-off status byte 0x82 (channel 2) in track data")
	}
}

func TestControlChangeClampsOutOfRangeValues(t *testing.T) {
	w := NewWriter(clock.Real{})
	w.ControlChange(99, 200, -5) // channel/controller/value all out of range

	if !bytes.Contains(w.track, []byte{0xBF, 127, 0}) {
		t.Fatalf("expected clamped CC bytes (channel 15, controller 127, value 0), got % X", w.track)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}

func TestProgramChangeSelectsBankThenProgram(t *testing.T) {
	w := NewWriter(clock.Real{})
	w.ProgramChange(3, 8, 32)

	// Bank select (CC 0), then, with no delay, the program change.
	if !bytes.Contains(w.track, []byte{0xB3, 0x00, 8, 0x00, 0xC3, 32}) {
		t.Fatalf("expected bank select then program change on channel 3, got % X", w.track)
	}
}

// trackDeltas returns the delta time of each event in a file's track,
// after the tempo event, by walking VLQ deltas and the fixed-size channel
// messages the writer emits.
func trackDeltas(t *testing.T, data []byte) []uint32 {
	t.Helper()
	track := data[14+8:]
	track = track[1+6:] // the tempo event: delta 0, FF 51 03 tt tt tt
	var deltas []uint32
	for len(track) > 0 {
		var d uint32
		for {
			b := track[0]
			track = track[1:]
			d = d<<7 | uint32(b&0x7F)
			if b&0x80 == 0 {
				break
			}
		}
		switch status := track[0]; {
		case status == 0xFF: // end of track
			return deltas
		case status&0xF0 == 0xC0:
			track = track[2:]
		default:
			track = track[3:]
		}
		deltas = append(deltas, d)
	}
	return deltas
}

func TestVirtualOutputTimesNotesByItsClock(t *testing.T) {
	start := time.Unix(0, 0)
	clk := clock.NewVirtual(start)
	path := filepath.Join(t.TempDir(), "out.mid")
	out := NewVirtualOutput(path, clk)

	clk.Advance(start.Add(2 * time.Second))
	out.Send(event.Note{Channel: 0, Pitch: 60, Velocity: 100, DurationMs: 500})
	clk.Advance(start.Add(10 * time.Second))
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	// 960 ticks a second: on at 2 s, off half a second later.
	got := trackDeltas(t, readFile(t, path))
	if want := []uint32{1920, 480}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("deltas = %v, want %v", got, want)
	}
}

func TestWriterDoesNotDrift(t *testing.T) {
	start := time.Unix(0, 0)
	clk := clock.NewVirtual(start)
	w := NewWriter(clk)
	// A third of a second is 320 ticks exactly, but its float steps don't
	// land on whole ticks; a thousand of them should still total 320000.
	for i := 1; i <= 1000; i++ {
		clk.Advance(start.Add(time.Duration(i) * time.Second / 3))
		w.ControlChange(0, 1, 1)
	}
	path := filepath.Join(t.TempDir(), "out.mid")
	if err := w.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	var total uint32
	for _, d := range trackDeltas(t, readFile(t, path)) {
		total += d
	}
	if total < 319999 || total > 320000 {
		t.Errorf("total ticks = %d, want 320000", total)
	}
}
