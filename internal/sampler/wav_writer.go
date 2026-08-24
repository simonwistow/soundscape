package sampler

import (
	"bytes"
	"encoding/binary"
	"os"
)

// WriteWAV writes mono float32 samples ([-1, 1], clamped) as a 16-bit PCM
// mono WAV file. Used by the placeholder sample generator (cmd/gensamples);
// loadWAV can read the result back (mono is duplicated to stereo on load).
func WriteWAV(path string, mono []float32, sampleRate int) error {
	data := make([]byte, len(mono)*2)
	for i, s := range mono {
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		binary.LittleEndian.PutUint16(data[i*2:], uint16(int16(s*32767)))
	}

	var buf bytes.Buffer
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(36+len(data)))
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1)) // mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // byte rate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))            // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))           // bits per sample

	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)

	return os.WriteFile(path, buf.Bytes(), 0o644)
}
