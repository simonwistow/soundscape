package synth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/sinshu/go-meltysynth/meltysynth"

	"example.com/fastly-soundscape/internal/output"
)

type SoundFontOutput struct {
	mu     sync.Mutex
	synth  *meltysynth.Synthesizer
	player *oto.Player
	pipe   *audioPipe
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

	pipe := newAudioPipe()
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

func (s *SoundFontOutput) Send(e output.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch v := e.(type) {
	case output.NoteOn:
		s.synth.NoteOn(int32(v.Channel), int32(v.Note), int32(v.Velocity))

		go func(channel, note int, duration time.Duration) {
			time.Sleep(duration)
			s.mu.Lock()
			s.synth.NoteOff(int32(channel), int32(note))
			s.mu.Unlock()
		}(v.Channel, v.Note, time.Duration(v.DurationMs)*time.Millisecond)

	case output.CC:
		// SoundFont synths expose MIDI controllers. This lets a theme use
		// the same event model for an external MIDI device and the internal
		// synth. Exact controller semantics depend on the SoundFont.
		s.synth.ProcessMidiMessage(
			int32(v.Channel),
			0xB0,
			int32(v.Controller),
			int32(v.Value),
		)

	default:
		return fmt.Errorf("unsupported event %T", e)
	}

	return nil
}

func (s *SoundFontOutput) Close() {
	if s.pipe != nil {
		s.pipe.Close()
	}
	if s.player != nil {
		_ = s.player.Close()
	}
}

type audioPipe struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    []byte
	closed bool
}

func newAudioPipe() *audioPipe {
	p := &audioPipe{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *audioPipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, fmt.Errorf("audio pipe closed")
	}

	p.buf = append(p.buf, b...)
	p.cond.Signal()
	return len(b), nil
}

func (p *audioPipe) Read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for len(p.buf) == 0 && !p.closed {
		p.cond.Wait()
	}

	if len(p.buf) == 0 && p.closed {
		return 0, fmt.Errorf("audio pipe closed")
	}

	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

func (p *audioPipe) Close() {
	p.mu.Lock()
	p.closed = true
	p.cond.Broadcast()
	p.mu.Unlock()
}
