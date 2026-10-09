package synth

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/sinshu/go-meltysynth/meltysynth"

	"github.com/simonwistow/soundscape/internal/event"
)

// SoundFontOutput plays note, cc and program events on a SoundFont. It's an
// audio.Source: add it to a mixer to hear it.
type SoundFontOutput struct {
	mu          sync.Mutex
	synth       *meltysynth.Synthesizer
	left, right []float32
}

// NewSoundFontOutput loads the SoundFont at path into a synth running at
// sampleRate.
func NewSoundFontOutput(path string, sampleRate int) (*SoundFontOutput, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sf, err := meltysynth.NewSoundFont(f)
	if err != nil {
		return nil, err
	}

	settings := meltysynth.NewSynthesizerSettings(int32(sampleRate))
	s, err := meltysynth.NewSynthesizer(sf, settings)
	if err != nil {
		return nil, err
	}

	return &SoundFontOutput{synth: s}, nil
}

// Render adds the synth's next len(buf)/2 frames into buf, as interleaved
// stereo.
func (s *SoundFontOutput) Render(buf []float32) {
	frames := len(buf) / 2
	if len(s.left) < frames {
		s.left = make([]float32, frames)
		s.right = make([]float32, frames)
	}
	left, right := s.left[:frames], s.right[:frames]

	s.mu.Lock()
	s.synth.Render(left, right)
	s.mu.Unlock()

	for i := range frames {
		buf[2*i] += left[i]
		buf[2*i+1] += right[i]
	}
}

func (s *SoundFontOutput) Send(e event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch v := e.(type) {
	case event.Note:
		s.synth.NoteOn(int32(v.Channel), int32(v.Pitch), int32(v.Velocity))

		go func(channel, pitch int, duration time.Duration) {
			time.Sleep(duration)
			s.mu.Lock()
			s.synth.NoteOff(int32(channel), int32(pitch))
			s.mu.Unlock()
		}(v.Channel, v.Pitch, time.Duration(v.DurationMs)*time.Millisecond)

	case event.Control:
		// SoundFont synths expose MIDI controllers. This lets a theme use
		// the same event model for an external MIDI device and the internal
		// synth. Exact controller semantics depend on the SoundFont.
		s.synth.ProcessMidiMessage(
			int32(v.Channel),
			0xB0,
			int32(v.Controller),
			int32(v.Value),
		)

	case event.Program:
		s.synth.ProcessMidiMessage(int32(v.Channel), 0xB0, 0x00, int32(v.Bank)) // bank select
		s.synth.ProcessMidiMessage(int32(v.Channel), 0xC0, int32(v.Program), 0)

	default:
		return fmt.Errorf("unsupported event %T", e)
	}

	return nil
}

// Preset is one instrument in a SoundFont, selected by its bank and
// program number.
type Preset struct {
	Bank, Program int
	Name          string
}

// Presets lists the instruments in the SoundFont at path, by bank and then
// program.
func Presets(path string) ([]Preset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sf, err := meltysynth.NewSoundFont(f)
	if err != nil {
		return nil, err
	}
	presets := make([]Preset, len(sf.Presets))
	for i, p := range sf.Presets {
		presets[i] = Preset{Bank: int(p.BankNumber), Program: int(p.PatchNumber), Name: p.Name}
	}
	sort.Slice(presets, func(i, j int) bool {
		if presets[i].Bank != presets[j].Bank {
			return presets[i].Bank < presets[j].Bank
		}
		return presets[i].Program < presets[j].Program
	})
	return presets, nil
}
