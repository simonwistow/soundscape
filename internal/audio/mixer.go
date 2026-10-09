package audio

import (
	"errors"
	"log"
	"sync"
	"time"
)

// BlockFrames is how many frames the mixer renders at a time: about 11.6 ms
// at 44.1 kHz.
const BlockFrames = 512

// Source is something that makes sound: the sample player, the SoundFont
// synth. Render adds its next len(buf)/2 frames of interleaved stereo
// float32 audio into buf, which already holds the other sources' audio.
// It's called from the mixer's goroutine, so sources lock their own state.
type Source interface {
	Render(buf []float32)
}

// Sink is somewhere the mix goes: the speakers, a file. Write takes a block
// of interleaved stereo float32 frames in [-1, 1]; it may keep the slice
// only until it returns.
type Sink interface {
	Write(frames []float32) error
	Close() error
}

// pacer is a Sink whose Write blocks until the audio device is ready for
// more, so it, rather than the clock, sets the pace of the mix.
type pacer interface {
	pacesMix()
}

// Mixer renders its sources into one stereo stream and hands every block to
// all of its sinks. With speakers among the sinks the audio device paces it;
// without, it keeps pace with the clock, so the mix still runs in real time
// alongside the telemetry.
type Mixer struct {
	sampleRate int
	sources    []Source
	sinks      []Sink

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	err       error
}

func NewMixer(sampleRate int) *Mixer {
	return &Mixer{
		sampleRate: sampleRate,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// AddSource and AddSink must be called before Start.
func (m *Mixer) AddSource(s Source) { m.sources = append(m.sources, s) }
func (m *Mixer) AddSink(s Sink)     { m.sinks = append(m.sinks, s) }

func (m *Mixer) Start() {
	go m.run()
}

func (m *Mixer) run() {
	defer close(m.done)

	active := append([]Sink(nil), m.sinks...)
	buf := make([]float32, BlockFrames*2)
	start := time.Now()
	var rendered int64

	for {
		select {
		case <-m.stop:
			return
		default:
		}

		for i := range buf {
			buf[i] = 0
		}
		for _, s := range m.sources {
			s.Render(buf)
		}
		for i, v := range buf {
			buf[i] = min(max(v, -1), 1)
		}

		// A sink that fails (a full disk, say) is dropped, and the rest
		// carry on: a broken recording shouldn't silence the speakers.
		live := active[:0]
		for _, s := range active {
			if err := s.Write(buf); err != nil {
				log.Printf("audio: %v (stopped writing to it)", err)
				m.err = errors.Join(m.err, err)
				continue
			}
			live = append(live, s)
		}
		active = live

		rendered += BlockFrames
		if !paced(active) {
			due := start.Add(time.Duration(rendered) * time.Second / time.Duration(m.sampleRate))
			select {
			case <-m.stop:
				return
			case <-time.After(time.Until(due)):
			}
		}
	}
}

func paced(sinks []Sink) bool {
	for _, s := range sinks {
		if _, ok := s.(pacer); ok {
			return true
		}
	}
	return false
}

// Close stops the mix and then closes every sink, so a file sink gets to
// finish its file. It returns any errors from the sinks, including one that
// failed mid-mix. It's safe to call more than once.
func (m *Mixer) Close() error {
	m.closeOnce.Do(func() {
		close(m.stop)

		// The loop notices stop within a block, unless a speakers sink is
		// stuck waiting on a stalled device; closing that sink releases
		// it. Other sinks are only closed once the loop has finished, so a
		// file is never closed mid-write.
		closed := make(map[Sink]bool)
		var errs []error
		select {
		case <-m.done:
		case <-time.After(time.Second):
			for _, s := range m.sinks {
				if _, ok := s.(pacer); ok {
					errs = append(errs, s.Close())
					closed[s] = true
				}
			}
			<-m.done
		}

		errs = append(errs, m.err)
		for _, s := range m.sinks {
			if !closed[s] {
				errs = append(errs, s.Close())
			}
		}
		m.err = errors.Join(errs...)
	})
	return m.err
}
