// Package record writes the mix to audio files: WAV and MP3, picked by the
// file's extension. Each file is an audio.Sink, written as the mix plays.
package record

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/simonwistow/soundscape/internal/audio"
)

// Formats lists the file extensions Create understands.
var Formats = []string{".wav", ".mp3"}

// Check reports whether Create would accept path and options, without
// creating anything, so a mistake is caught before the soundscape starts.
// options are the --output file options: bitrate (kbps), for MP3 only.
func Check(path string, sampleRate int, options map[string]string) error {
	_, _, err := parse(path, sampleRate, options)
	return err
}

// Create starts a recording at path, in the format its extension names.
func Create(path string, sampleRate int, options map[string]string) (audio.Sink, error) {
	ext, bitrate, err := parse(path, sampleRate, options)
	if err != nil {
		return nil, err
	}
	// Return a nil interface, not a nil *WAV or *MP3, on failure.
	if ext == ".wav" {
		w, err := CreateWAV(path, sampleRate)
		if err != nil {
			return nil, err
		}
		return w, nil
	}
	m, err := CreateMP3(path, sampleRate, bitrate)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// parse works out the format and, for MP3, the bitrate.
func parse(path string, sampleRate int, options map[string]string) (ext string, bitrate int, err error) {
	ext = strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".wav":
		if _, ok := options["bitrate"]; ok {
			return "", 0, fmt.Errorf("%s: WAV is uncompressed, so it takes no bitrate", path)
		}
		return ext, 0, nil

	case ".mp3":
		bitrate = DefaultMP3Bitrate
		if b, ok := options["bitrate"]; ok {
			bitrate, err = strconv.Atoi(strings.TrimSuffix(strings.ToLower(b), "k"))
			if err != nil {
				return "", 0, fmt.Errorf("%s: bitrate %q isn't a number of kbps", path, b)
			}
		}
		if err := checkMP3(path, sampleRate, bitrate); err != nil {
			return "", 0, err
		}
		return ext, bitrate, nil

	default:
		return "", 0, fmt.Errorf("%s: can't tell the format from the extension (want %s)", path, strings.Join(Formats, " or "))
	}
}
