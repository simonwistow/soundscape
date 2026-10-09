package audio

import (
	"encoding/binary"
	"math"

	"github.com/ebitengine/oto/v3"
)

// pipeBlocks is how many mixed blocks may queue for the audio device:
// enough to ride out scheduling hiccups (about 46 ms), little enough that a
// new event is heard promptly.
const pipeBlocks = 4

// Speakers is a Sink that plays the mix through the default audio device.
// Its Write blocks while the device has enough queued, so the device paces
// the mix.
type Speakers struct {
	pipe   *Pipe
	player *oto.Player
	bytes  []byte
}

// NewSpeakers opens the audio device. A process can only do this once.
func NewSpeakers(sampleRate int) (*Speakers, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
	})
	if err != nil {
		return nil, err
	}
	<-ready

	pipe := NewPipe(pipeBlocks * BlockFrames * 2 * 4)
	player := ctx.NewPlayer(pipe)
	// Oto reads half a second ahead by default, which is half a second
	// between an event and hearing it; a tenth is plenty.
	player.SetBufferSize(sampleRate / 10 * 2 * 4)
	player.Play()
	return &Speakers{pipe: pipe, player: player}, nil
}

func (s *Speakers) Write(frames []float32) error {
	if cap(s.bytes) < len(frames)*4 {
		s.bytes = make([]byte, len(frames)*4)
	}
	b := s.bytes[:len(frames)*4]
	for i, v := range frames {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	_, err := s.pipe.Write(b)
	return err
}

func (s *Speakers) Close() error {
	s.pipe.Close()
	return s.player.Close()
}

func (s *Speakers) pacesMix() {}
