package synth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/sinshu/go-meltysynth/meltysynth"

	"github.com/simonwistow/soundscape/internal/audio"
	"github.com/simonwistow/soundscape/internal/event"
)

type SoundFontOutput struct {
	mu     sync.Mutex
	synth  *meltysynth.Synthesizer
	player *oto.Player
	pipe   *audio.Pipe
}

func NewSoundFontOutput(path string) (*SoundFontOutput, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sf, err := meltysynth.NewSoundFont(f)
	if err != nil {
		return nil, err
	}

	settings := meltysynth.NewSynthesizerSettings(44100)
	s, err := meltysynth.NewSynthesizer(sf, settings)
	if err != nil {
		return nil, err
	}

	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   44100,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
	})
	if err != nil {
		return nil, err
	}
	<-ready

	pipe := audio.NewPipe()
	player := ctx.NewPlayer(pipe)
	player.Play()

	out := &SoundFontOutput{
		synth:  s,
		player: player,
		pipe:   pipe,
	}

	go out.renderLoop(settings.SampleRate)
	return out, nil
}

func (s *SoundFontOutput) renderLoop(sampleRate int32) {
	const frames = 512
	left := make([]float32, frames)
	right := make([]float32, frames)

	for {
		s.mu.Lock()
		s.synth.Render(left, right)
		s.mu.Unlock()

		buf := bytes.NewBuffer(make([]byte, 0, frames*8))
		for i := 0; i < frames; i++ {
			_ = binary.Write(buf, binary.LittleEndian, left[i])
			_ = binary.Write(buf, binary.LittleEndian, right[i])
		}

		if _, err := s.pipe.Write(buf.Bytes()); err != nil {
			return
		}

		// Keep a modest amount of audio buffered without allowing the
		// renderer to run arbitrarily far ahead of the audio device.
		time.Sleep(time.Duration(float64(frames) / float64(sampleRate) * float64(time.Second) / 2))
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

func (s *SoundFontOutput) Close() {
	if s.pipe != nil {
		s.pipe.Close()
	}
	if s.player != nil {
		_ = s.player.Close()
	}
}
