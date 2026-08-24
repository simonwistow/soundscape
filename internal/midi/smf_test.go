package midi

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
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
	w := NewWriter()
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
	w := NewWriter()
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
	w := NewWriter()
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
