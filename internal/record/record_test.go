package record

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tone is n frames of a 440 Hz tone on the left and 660 Hz on the right.
func tone(n int) []float32 {
	out := make([]float32, n*2)
	for i := range n {
		t := float64(i) / 44100
		out[2*i] = float32(0.5 * math.Sin(2*math.Pi*440*t))
		out[2*i+1] = float32(0.5 * math.Sin(2*math.Pi*660*t))
	}
	return out
}

func TestWAVRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.wav")
	w, err := CreateWAV(path, 44100)
	if err != nil {
		t.Fatal(err)
	}
	in := tone(44100 / 2)
	for i := 0; i < len(in); i += 1024 {
		if err := w.Write(in[i:min(i+1024, len(in))]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || string(b[36:40]) != "data" {
		t.Fatal("not a canonical WAV header")
	}
	if got := binary.LittleEndian.Uint16(b[22:]); got != 2 {
		t.Errorf("channels = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(b[24:]); got != 44100 {
		t.Errorf("sample rate = %d, want 44100", got)
	}
	dataSize := binary.LittleEndian.Uint32(b[40:])
	if int(dataSize) != len(in)*2 || len(b) != 44+len(in)*2 {
		t.Fatalf("data size %d, file %d bytes; want %d and %d", dataSize, len(b), len(in)*2, 44+len(in)*2)
	}
	if got := binary.LittleEndian.Uint32(b[4:]); got != 36+dataSize {
		t.Errorf("RIFF size = %d, want %d", got, 36+dataSize)
	}

	for i, want := range in {
		got := float64(int16(binary.LittleEndian.Uint16(b[44+i*2:]))) / 32767
		// Rounding plus triangular dither stays within 1.5 LSB.
		if math.Abs(got-float64(want)) > 1.5/32767 {
			t.Fatalf("sample %d = %v, want %v", i, got, want)
		}
	}
}

func TestWAVHeaderKeepsUpWhileRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.wav")
	w, err := CreateWAV(path, 44100)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Two seconds, so the header has been brought up to date at least once.
	in := tone(2 * 44100)
	if err := w.Write(in); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dataSize := binary.LittleEndian.Uint32(b[40:])
	if dataSize == 0 || int(dataSize) > len(b)-44 {
		t.Errorf("mid-recording header says %d bytes of data, file has %d", dataSize, len(b)-44)
	}
}

func TestWAVStopsAtItsSizeLimit(t *testing.T) {
	defer func(old int64) { maxWAVData = old }(maxWAVData)
	maxWAVData = 4 * 1000

	path := filepath.Join(t.TempDir(), "out.wav")
	w, err := CreateWAV(path, 44100)
	if err != nil {
		t.Fatal(err)
	}
	block := tone(512)
	if err := w.Write(block); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(block); err == nil || !strings.Contains(err.Error(), "record MP3 for longer") {
		t.Fatalf("Write past the limit = %v, want the size-limit error", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if got := binary.LittleEndian.Uint32(b[40:]); got != 512*4 {
		t.Errorf("data size = %d, want what was written before the limit (%d)", got, 512*4)
	}
}

func TestMP3(t *testing.T) {
	for _, bitrate := range []int{128, 192, 256, 320} {
		path := filepath.Join(t.TempDir(), "out.mp3")
		m, err := CreateMP3(path, 44100, bitrate)
		if err != nil {
			t.Fatal(err)
		}
		in := tone(2 * 44100)
		for i := 0; i < len(in); i += 1024 {
			if err := m.Write(in[i:min(i+1024, len(in))]); err != nil {
				t.Fatal(err)
			}
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}

		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// An MPEG-1 Layer III frame header: sync, then the bitrate index.
		if b[0] != 0xFF || b[1]&0xFE != 0xFA {
			t.Fatalf("%d kbps: doesn't start with an MPEG-1 Layer III frame: % x", bitrate, b[:4])
		}
		wantIndex := map[int]byte{128: 9, 192: 11, 256: 13, 320: 14}[bitrate]
		if got := b[2] >> 4; got != wantIndex {
			t.Errorf("%d kbps: bitrate index %d, want %d", bitrate, got, wantIndex)
		}

		// Two seconds, plus up to two frames of padding at the end.
		frames := math.Ceil(2*44100/1152.0) + 1
		want := frames * 1152 / 44100 * float64(bitrate) * 1000 / 8
		if math.Abs(float64(len(b))-want) > 4 {
			t.Errorf("%d kbps: %d bytes, want about %.0f", bitrate, len(b), want)
		}
	}
}

func TestCreate(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		options map[string]string
		wantErr string
	}{
		{"a.wav", nil, ""},
		{"a.WAV", nil, ""},
		{"a.mp3", nil, ""},
		{"a.mp3", map[string]string{"bitrate": "192"}, ""},
		{"a.mp3", map[string]string{"bitrate": "192k"}, ""},
		{"a.mp3", map[string]string{"bitrate": "200"}, "isn't one of"},
		{"a.mp3", map[string]string{"bitrate": "fast"}, "isn't a number"},
		{"a.wav", map[string]string{"bitrate": "192"}, "takes no bitrate"},
		{"a.flac", nil, "can't tell the format"},
		{"a", nil, "can't tell the format"},
	} {
		sink, err := Create(filepath.Join(dir, tc.name), 44100, tc.options)
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("Create(%s, %v): %v", tc.name, tc.options, err)
				continue
			}
			sink.Close()
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("Create(%s, %v) = %v, want an error mentioning %q", tc.name, tc.options, err, tc.wantErr)
		}
	}
}
