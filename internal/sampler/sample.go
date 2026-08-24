package sampler

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Sample holds decoded, interleaved-stereo audio at the player's fixed
// sample rate, ready to be mixed by a voice without further conversion.
type Sample struct {
	Name   string
	Frames []float32 // interleaved L/R
	Length int       // frame count (Frames has Length*2 elements)
}

// loadSample loads a WAV file and resamples it to targetRate if needed.
func loadSample(path string, targetRate int) (*Sample, error) {
	frames, sourceRate, err := loadWAV(path)
	if err != nil {
		return nil, err
	}
	if sourceRate <= 0 {
		return nil, fmt.Errorf("%s: invalid sample rate %d", path, sourceRate)
	}
	if sourceRate != targetRate {
		frames = resampleStereo(frames, sourceRate, targetRate)
	}

	return &Sample{
		Name:   strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Frames: frames,
		Length: len(frames) / 2,
	}, nil
}

// resampleStereo linearly resamples interleaved stereo audio from
// sourceRate to targetRate. Adequate for the naturalistic ambience this
// project targets; not a substitute for a proper band-limited resampler.
func resampleStereo(in []float32, sourceRate, targetRate int) []float32 {
	srcFrames := len(in) / 2
	if srcFrames == 0 {
		return in
	}

	ratio := float64(sourceRate) / float64(targetRate)
	dstFrames := int(float64(srcFrames) / ratio)
	out := make([]float32, dstFrames*2)

	for i := 0; i < dstFrames; i++ {
		pos := float64(i) * ratio
		i0 := int(pos)
		if i0 >= srcFrames-1 {
			out[i*2] = in[(srcFrames-1)*2]
			out[i*2+1] = in[(srcFrames-1)*2+1]
			continue
		}
		frac := float32(pos - float64(i0))
		out[i*2] = in[i0*2] + (in[(i0+1)*2]-in[i0*2])*frac
		out[i*2+1] = in[i0*2+1] + (in[(i0+1)*2+1]-in[i0*2+1])*frac
	}
	return out
}

// Group is a named collection of samples (e.g. "bird calls") that a theme
// can trigger, letting the player pick a random variation each time.
type Group struct {
	Name    string
	Samples []*Sample
}

// LoadGroup loads every .wav file in dir (non-recursive) as one Group.
func LoadGroup(dir string, targetRate int) (*Group, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".wav") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names) // deterministic load order regardless of filesystem

	if len(names) == 0 {
		return nil, fmt.Errorf("%s: no .wav files found", dir)
	}

	g := &Group{Name: filepath.Base(dir)}
	for _, name := range names {
		s, err := loadSample(filepath.Join(dir, name), targetRate)
		if err != nil {
			return nil, err
		}
		g.Samples = append(g.Samples, s)
	}
	return g, nil
}

// Random returns a random sample from the group.
func (g *Group) Random(rng *rand.Rand) *Sample {
	if len(g.Samples) == 0 {
		return nil
	}
	return g.Samples[rng.Intn(len(g.Samples))]
}
