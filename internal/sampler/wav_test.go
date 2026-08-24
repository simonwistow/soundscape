package sampler

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeTestWAV writes a minimal PCM16 WAV file with a synthetic sine wave,
// for use as a test fixture without depending on any external asset.
func writeTestWAV(t *testing.T, path string, sampleRate, channels, frames int, freqHz float64) {
	t.Helper()

	data := make([]byte, frames*channels*2)
	for i := 0; i < frames; i++ {
		v := int16(math.Sin(2*math.Pi*freqHz*float64(i)/float64(sampleRate)) * 20000)
		for c := 0; c < channels; c++ {
			binary.LittleEndian.PutUint16(data[(i*channels+c)*2:], uint16(v))
		}
	}

	var buf []byte
	buf = append(buf, "RIFF"...)
	buf = append(buf, make([]byte, 4)...) // total size, filled below
	buf = append(buf, "WAVE"...)

	buf = append(buf, "fmt "...)
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1) // PCM
	binary.LittleEndian.PutUint16(fmtChunk[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(fmtChunk[4:8], uint32(sampleRate))
	byteRate := sampleRate * channels * 2
	binary.LittleEndian.PutUint32(fmtChunk[8:12], uint32(byteRate))
	binary.LittleEndian.PutUint16(fmtChunk[12:14], uint16(channels*2))
	binary.LittleEndian.PutUint16(fmtChunk[14:16], 16)
	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(fmtChunk)))
	buf = append(buf, sizeBuf...)
	buf = append(buf, fmtChunk...)

	buf = append(buf, "data"...)
	sizeBuf = make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(data)))
	buf = append(buf, sizeBuf...)
	buf = append(buf, data...)

	binary.LittleEndian.PutUint32(buf[4:8], uint32(len(buf)-8))

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("writing test WAV: %v", err)
	}
}

func TestLoadWAVMonoToStereo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	writeTestWAV(t, path, 44100, 1, 1000, 440)

	frames, rate, err := loadWAV(path)
	if err != nil {
		t.Fatalf("loadWAV: %v", err)
	}
	if rate != 44100 {
		t.Fatalf("sample rate = %d, want 44100", rate)
	}
	if len(frames) != 1000*2 {
		t.Fatalf("frame count = %d, want %d", len(frames), 1000*2)
	}
	for i := 0; i < 1000; i++ {
		if frames[i*2] != frames[i*2+1] {
			t.Fatalf("mono source should duplicate to identical L/R at frame %d", i)
		}
	}
}

func TestLoadWAVStereoPassthrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stereo.wav")
	writeTestWAV(t, path, 22050, 2, 500, 220)

	frames, rate, err := loadWAV(path)
	if err != nil {
		t.Fatalf("loadWAV: %v", err)
	}
	if rate != 22050 {
		t.Fatalf("sample rate = %d, want 22050", rate)
	}
	if len(frames) != 500*2 {
		t.Fatalf("frame count = %d, want %d", len(frames), 500*2)
	}
}

func TestLoadSampleResamplesToTargetRate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "low-rate.wav")
	writeTestWAV(t, path, 22050, 1, 2205, 440) // 0.1s of audio

	s, err := loadSample(path, 44100)
	if err != nil {
		t.Fatalf("loadSample: %v", err)
	}

	wantFrames := 4410 // 0.1s at 44100
	if diff := s.Length - wantFrames; diff < -5 || diff > 5 {
		t.Fatalf("resampled length = %d, want ~%d", s.Length, wantFrames)
	}
}

func TestLoadGroupPicksAmongSamples(t *testing.T) {
	dir := t.TempDir()
	writeTestWAV(t, filepath.Join(dir, "a.wav"), 44100, 1, 100, 300)
	writeTestWAV(t, filepath.Join(dir, "b.wav"), 44100, 1, 100, 600)

	g, err := LoadGroup(dir, 44100)
	if err != nil {
		t.Fatalf("LoadGroup: %v", err)
	}
	if len(g.Samples) != 2 {
		t.Fatalf("loaded %d samples, want 2", len(g.Samples))
	}
}
