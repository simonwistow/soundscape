package sampler

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	formatPCM   = 1
	formatFloat = 3
	// formatExtensible wraps another format tag inside the fmt chunk's
	// extension; the real tag is compared separately.
	formatExtensible = 0xFFFE
)

type wavFormat struct {
	audioFormat   uint16
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
}

// loadWAV reads a RIFF/WAVE file and returns interleaved stereo float32
// samples ([-1, 1]) at the file's native sample rate. Supports PCM
// (8/16/24/32-bit integer) and IEEE float (32-bit) data, mono or stereo.
func loadWAV(path string) (frames []float32, sampleRate int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var riffHeader [12]byte
	if _, err := io.ReadFull(f, riffHeader[:]); err != nil {
		return nil, 0, fmt.Errorf("%s: reading RIFF header: %w", path, err)
	}
	if string(riffHeader[0:4]) != "RIFF" || string(riffHeader[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("%s: not a RIFF/WAVE file", path)
	}

	var format *wavFormat
	var raw []byte

	for {
		var chunkHeader [8]byte
		if _, err := io.ReadFull(f, chunkHeader[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, 0, fmt.Errorf("%s: reading chunk header: %w", path, err)
		}
		id := string(chunkHeader[0:4])
		size := binary.LittleEndian.Uint32(chunkHeader[4:8])

		switch id {
		case "fmt ":
			body := make([]byte, size)
			if _, err := io.ReadFull(f, body); err != nil {
				return nil, 0, fmt.Errorf("%s: reading fmt chunk: %w", path, err)
			}
			format = &wavFormat{
				audioFormat:   binary.LittleEndian.Uint16(body[0:2]),
				numChannels:   binary.LittleEndian.Uint16(body[2:4]),
				sampleRate:    binary.LittleEndian.Uint32(body[4:8]),
				bitsPerSample: binary.LittleEndian.Uint16(body[14:16]),
			}
			if format.audioFormat == formatExtensible && len(body) >= 40 {
				// The real format tag lives in the first two bytes of the
				// extension's sub-format GUID.
				format.audioFormat = binary.LittleEndian.Uint16(body[24:26])
			}
		case "data":
			raw = make([]byte, size)
			if _, err := io.ReadFull(f, raw); err != nil {
				return nil, 0, fmt.Errorf("%s: reading data chunk: %w", path, err)
			}
		default:
			if _, err := f.Seek(int64(size), io.SeekCurrent); err != nil {
				return nil, 0, fmt.Errorf("%s: skipping chunk %q: %w", path, id, err)
			}
		}
		if size%2 == 1 {
			// Chunks are word-aligned; skip the pad byte.
			if _, err := f.Seek(1, io.SeekCurrent); err != nil {
				break
			}
		}
	}

	if format == nil {
		return nil, 0, fmt.Errorf("%s: missing fmt chunk", path)
	}
	if raw == nil {
		return nil, 0, fmt.Errorf("%s: missing data chunk", path)
	}
	if format.numChannels == 0 {
		return nil, 0, fmt.Errorf("%s: invalid channel count", path)
	}

	mono, err := decodeSamples(raw, *format)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", path, err)
	}

	stereo := toStereo(mono, int(format.numChannels))
	return stereo, int(format.sampleRate), nil
}

// decodeSamples converts raw PCM/float bytes into per-channel-interleaved
// float32 samples in [-1, 1], still interleaved by the source channel count.
func decodeSamples(raw []byte, format wavFormat) ([]float32, error) {
	bytesPerSample := int(format.bitsPerSample) / 8
	if bytesPerSample == 0 {
		return nil, fmt.Errorf("unsupported bits-per-sample %d", format.bitsPerSample)
	}

	count := len(raw) / bytesPerSample
	out := make([]float32, count)

	switch {
	case format.audioFormat == formatPCM && format.bitsPerSample == 8:
		for i := 0; i < count; i++ {
			// 8-bit PCM is unsigned, centred at 128.
			out[i] = (float32(raw[i]) - 128) / 128
		}
	case format.audioFormat == formatPCM && format.bitsPerSample == 16:
		for i := 0; i < count; i++ {
			v := int16(binary.LittleEndian.Uint16(raw[i*2:]))
			out[i] = float32(v) / 32768
		}
	case format.audioFormat == formatPCM && format.bitsPerSample == 24:
		for i := 0; i < count; i++ {
			b := raw[i*3 : i*3+3]
			v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if v&0x800000 != 0 {
				v |= -1 << 24 // sign-extend
			}
			out[i] = float32(v) / 8388608
		}
	case format.audioFormat == formatPCM && format.bitsPerSample == 32:
		for i := 0; i < count; i++ {
			v := int32(binary.LittleEndian.Uint32(raw[i*4:]))
			out[i] = float32(v) / 2147483648
		}
	case format.audioFormat == formatFloat && format.bitsPerSample == 32:
		for i := 0; i < count; i++ {
			bits := binary.LittleEndian.Uint32(raw[i*4:])
			out[i] = math.Float32frombits(bits)
		}
	default:
		return nil, fmt.Errorf("unsupported format (tag=%d, bits=%d)", format.audioFormat, format.bitsPerSample)
	}

	return out, nil
}

// toStereo expands mono to a duplicated L/R stream, downmixes >2 channels by
// averaging, and passes stereo through unchanged.
func toStereo(interleaved []float32, channels int) []float32 {
	if channels == 2 {
		return interleaved
	}

	frameCount := len(interleaved) / channels
	out := make([]float32, frameCount*2)

	if channels == 1 {
		for i := 0; i < frameCount; i++ {
			out[i*2] = interleaved[i]
			out[i*2+1] = interleaved[i]
		}
		return out
	}

	for i := 0; i < frameCount; i++ {
		var sum float32
		for c := 0; c < channels; c++ {
			sum += interleaved[i*channels+c]
		}
		avg := sum / float32(channels)
		out[i*2] = avg
		out[i*2+1] = avg
	}
	return out
}
