package record

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
)

const wavHeaderSize = 44

// maxWAVData is the most audio a WAV can hold: its sizes are 32-bit. At
// 44.1 kHz 16-bit stereo that's about 6.7 hours.
var maxWAVData int64 = math.MaxUint32 - wavHeaderSize // a var so tests can lower it

// WAV records 16-bit stereo PCM. Its header is brought up to date every
// second or so, so a recording cut short by a crash still opens, and once
// more on Close.
type WAV struct {
	path       string
	f          *os.File
	w          *bufio.Writer
	sampleRate int
	dataBytes  int64
	sinceSync  int64
	rng        *rand.Rand
	buf        []byte
	err        error
}

// CreateWAV starts a WAV recording at path.
func CreateWAV(path string, sampleRate int) (*WAV, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := &WAV{
		path:       path,
		f:          f,
		w:          bufio.NewWriterSize(f, 64*1024),
		sampleRate: sampleRate,
		rng:        rand.New(rand.NewPCG(1, 2)),
	}
	if _, err := w.w.Write(wavHeader(sampleRate, 0)); err != nil {
		f.Close()
		return nil, err
	}
	return w, nil
}

// Write appends interleaved stereo frames in [-1, 1].
func (w *WAV) Write(frames []float32) error {
	if w.err != nil {
		return w.err
	}
	n := int64(len(frames) * 2)
	if w.dataBytes+n > maxWAVData {
		w.err = fmt.Errorf("%s: a WAV can't hold more than about %.1f hours; record MP3 for longer", w.path,
			float64(maxWAVData)/float64(w.sampleRate*4)/3600)
		return w.err
	}

	if cap(w.buf) < len(frames)*2 {
		w.buf = make([]byte, len(frames)*2)
	}
	b := w.buf[:len(frames)*2]
	for i, v := range frames {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(w.toInt16(v)))
	}
	if _, err := w.w.Write(b); err != nil {
		w.err = fmt.Errorf("%s: %w", w.path, err)
		return w.err
	}
	w.dataBytes += n

	w.sinceSync += n
	if w.sinceSync >= int64(w.sampleRate*4) {
		w.sinceSync = 0
		if err := w.syncHeader(); err != nil {
			w.err = fmt.Errorf("%s: %w", w.path, err)
			return w.err
		}
	}
	return nil
}

// toInt16 quantises with triangular dither, so quiet fades end in a gentle
// hiss rather than gritty truncation distortion.
func (w *WAV) toInt16(v float32) int16 {
	dither := w.rng.Float64() - w.rng.Float64()
	s := math.Round(float64(v)*32767 + dither)
	return int16(min(max(s, -32768), 32767))
}

// syncHeader writes out what's buffered and rewrites the header's sizes to
// match it.
func (w *WAV) syncHeader() error {
	if err := w.w.Flush(); err != nil {
		return err
	}
	_, err := w.f.WriteAt(wavHeader(w.sampleRate, uint32(w.dataBytes)), 0)
	return err
}

// Close finishes the file. It's still finished after a failed Write, with
// what was recorded until then; that Write reported its own error.
func (w *WAV) Close() error {
	err := errors.Join(w.syncHeader(), w.f.Close())
	if err != nil {
		return fmt.Errorf("%s: %w", w.path, err)
	}
	return nil
}

func wavHeader(sampleRate int, dataBytes uint32) []byte {
	const channels, bits = 2, 16
	h := make([]byte, wavHeaderSize)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+dataBytes)
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], channels)
	binary.LittleEndian.PutUint32(h[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(h[28:], uint32(sampleRate*channels*bits/8))
	binary.LittleEndian.PutUint16(h[32:], channels*bits/8)
	binary.LittleEndian.PutUint16(h[34:], bits)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], dataBytes)
	return h
}
