package sampler

import (
	"math"
	"path/filepath"
	"testing"
)

func TestWriteWAVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.wav")

	const sampleRate = 8000
	mono := make([]float32, 800)
	for i := range mono {
		mono[i] = float32(math.Sin(2 * math.Pi * 440 * float64(i) / sampleRate))
	}

	if err := WriteWAV(path, mono, sampleRate); err != nil {
		t.Fatalf("WriteWAV: %v", err)
	}

	frames, rate, err := loadWAV(path)
	if err != nil {
		t.Fatalf("loadWAV: %v", err)
	}
	if rate != sampleRate {
		t.Fatalf("sample rate = %d, want %d", rate, sampleRate)
	}
	if len(frames) != len(mono)*2 {
		t.Fatalf("frame count = %d, want %d", len(frames), len(mono)*2)
	}

	// 16-bit quantization introduces small error; just check it's close.
	for i, want := range mono {
		got := frames[i*2]
		if diff := float64(got - want); diff < -0.01 || diff > 0.01 {
			t.Fatalf("sample %d = %v, want ~%v", i, got, want)
		}
	}
}
