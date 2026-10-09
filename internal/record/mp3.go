package record

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"

	"github.com/braheezy/shine-mp3/pkg/mp3"
)

// DefaultMP3Bitrate is the MP3 bitrate, in kbps, unless an output asks for
// another. Shine is a simple encoder; at 256 kbps it was indistinguishable
// from LAME by ear on the forest storm, rain included.
const DefaultMP3Bitrate = 256

// mp3Bitrates are the bitrates MPEG-1 Layer III allows, in kbps, in the
// order of the frame header's bitrate index (which starts at 1).
var mp3Bitrates = []int{32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}

// mp3Gain undoes shine's +1 dB, so an MP3 plays back as loud as the mix
// (and a WAV of it).
var mp3Gain = math.Pow(10, -1.0/20)

// mp3FrameSamples is how many frames shine encodes at a time, at MPEG-1
// sample rates.
const mp3FrameSamples = 1152

// MP3 records constant-bitrate stereo MP3 with shine, a pure-Go encoder.
type MP3 struct {
	path    string
	f       *os.File
	w       *bufio.Writer
	enc     *mp3.Encoder
	pending []int16 // interleaved, up to one encoder frame
	err     error
}

// CreateMP3 starts an MP3 recording at path. sampleRate must be 32000,
// 44100 or 48000, and bitrate one of MPEG-1's, from 32 to 320 kbps.
func CreateMP3(path string, sampleRate, bitrate int) (*MP3, error) {
	if err := checkMP3(path, sampleRate, bitrate); err != nil {
		return nil, err
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &MP3{
		path:    path,
		f:       f,
		w:       bufio.NewWriterSize(f, 64*1024),
		enc:     newShine(sampleRate, bitrate, slices.Index(mp3Bitrates, bitrate)+1),
		pending: make([]int16, 0, mp3FrameSamples*2),
	}, nil
}

func checkMP3(path string, sampleRate, bitrate int) error {
	if sampleRate != 32000 && sampleRate != 44100 && sampleRate != 48000 {
		return fmt.Errorf("%s: MP3 can't be recorded at %d Hz", path, sampleRate)
	}
	if !slices.Contains(mp3Bitrates, bitrate) {
		return fmt.Errorf("%s: MP3 bitrate %d isn't one of %v kbps", path, bitrate, mp3Bitrates)
	}
	return nil
}

// newShine makes a stereo shine encoder at the given bitrate. Shine's
// NewEncoder always sets up 128 kbps, so the bitrate and the frame sizes
// that follow from it are set again here, as its NewEncoder works them out.
func newShine(sampleRate, bitrate, bitrateIndex int) *mp3.Encoder {
	enc := mp3.NewEncoder(sampleRate, 2)
	enc.Mpeg.Bitrate = int64(bitrate)
	enc.Mpeg.BitrateIndex = int64(bitrateIndex)
	slots := float64(enc.Mpeg.GranulesPerFrame) * 576 / float64(sampleRate) * float64(bitrate) * 1000 / float64(enc.Mpeg.BitsPerSlot)
	enc.Mpeg.WholeSlotsPerFrame = int64(slots)
	enc.Mpeg.FracSlotsPerFrame = slots - float64(enc.Mpeg.WholeSlotsPerFrame)
	enc.Mpeg.SlotLag = -enc.Mpeg.FracSlotsPerFrame
	return enc
}

// Write encodes interleaved stereo frames in [-1, 1].
func (m *MP3) Write(frames []float32) error {
	if m.err != nil {
		return m.err
	}
	for _, v := range frames {
		s := math.Round(float64(v) * mp3Gain * 32767)
		m.pending = append(m.pending, int16(min(max(s, -32768), 32767)))
		if len(m.pending) == cap(m.pending) {
			if err := m.encode(); err != nil {
				return err
			}
		}
	}
	return nil
}

// encode encodes one full frame from pending. (Shine's own Write takes
// half a frame at a time in stereo, so it isn't used.)
func (m *MP3) encode() error {
	data, n := m.enc.EncodeBufferInterleaved(m.pending)
	m.pending = m.pending[:0]
	if _, err := m.w.Write(data[:n]); err != nil {
		m.err = fmt.Errorf("%s: %w", m.path, err)
		return m.err
	}
	return nil
}

// Close encodes what's left, padded with silence, and finishes the file.
func (m *MP3) Close() error {
	if m.err == nil {
		// One frame for the remainder, then one of silence: shine holds
		// back the last few bytes of each frame until the next one, and the
		// end of the audio is delayed by the encoder's filters.
		for range 2 {
			for len(m.pending) < cap(m.pending) {
				m.pending = append(m.pending, 0)
			}
			if err := m.encode(); err != nil {
				break
			}
		}
	}
	err := errors.Join(m.w.Flush(), m.f.Close())
	if err != nil {
		return fmt.Errorf("%s: %w", m.path, err)
	}
	return nil
}
