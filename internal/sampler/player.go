// Package sampler is a real WAV-based sample player output backend: it
// loads sample groups from disk and plays them back with pitch shifting,
// velocity/pan control, looping, voice management, and crossfading, driven
// by abstract event.Sample events from the theme engine.
package sampler

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
)

// maxOneShotVoices bounds simultaneous one-shot playback; beyond this the
// oldest active voice is stolen to make room for a new one, since discrete
// hits (bird calls, splashes) are individually short-lived and this project
// favours "some natural variation" over exact fidelity to every trigger.
const maxOneShotVoices = 32

type Player struct {
	mu         sync.Mutex
	sampleRate int
	groups     map[string]*Group
	oneShots   []*voice
	loops      map[string]*voice
	rng        *rand.Rand
	nextID     uint64
}

// NewPlayer creates a sample player at the given sample rate (44100 is a
// reasonable default). It's an audio.Source: add it to a mixer to hear it.
func NewPlayer(sampleRate int) *Player {
	return &Player{
		sampleRate: sampleRate,
		groups:     make(map[string]*Group),
		loops:      make(map[string]*voice),
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// LoadGroup loads every .wav file in dir and registers it under name so
// event.Sample values can reference it.
func (p *Player) LoadGroup(name, dir string) error {
	g, err := LoadGroup(dir, p.sampleRate)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.groups[name] = g
	p.mu.Unlock()
	return nil
}

func (p *Player) Send(e event.Event) error {
	v, ok := e.(event.Sample)
	if !ok {
		return fmt.Errorf("sampler: unsupported event %T", e)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	group, ok := p.groups[v.Group]
	if !ok {
		return fmt.Errorf("sampler: unknown sample group %q", v.Group)
	}

	gain := clamp32(float32(v.Velocity), 0, 1)
	pan := clamp32(float32(v.Pan), -1, 1)

	if v.Loop {
		if existing, ok := p.loops[v.Actor]; ok {
			existing.targetGain = gain
			existing.pan = pan
			if v.Pitch > 0 {
				existing.pitch = v.Pitch
			}
			return nil
		}

		sample := group.Random(p.rng)
		if sample == nil {
			return fmt.Errorf("sampler: group %q has no samples", v.Group)
		}
		p.nextID++
		voice := newLoopVoice(p.nextID, v.Actor, sample, v.Pitch, gain, pan, p.sampleRate)
		p.loops[v.Actor] = voice
		return nil
	}

	sample := group.Random(p.rng)
	if sample == nil {
		return fmt.Errorf("sampler: group %q has no samples", v.Group)
	}
	p.nextID++
	voice := newOneShotVoice(p.nextID, sample, v.Pitch, gain, pan, v.DurationMs, p.sampleRate)
	p.addOneShot(voice)
	return nil
}

// addOneShot appends a new one-shot voice, stealing the oldest active one if
// the pool is already full.
func (p *Player) addOneShot(v *voice) {
	if len(p.oneShots) >= maxOneShotVoices {
		oldest := 0
		for i, existing := range p.oneShots {
			if existing.id < p.oneShots[oldest].id {
				oldest = i
			}
		}
		p.oneShots = append(p.oneShots[:oldest], p.oneShots[oldest+1:]...)
	}
	p.oneShots = append(p.oneShots, v)
}

// Render adds the next len(buf)/2 frames of every playing voice into buf,
// as interleaved stereo.
func (p *Player) Render(buf []float32) {
	frames := len(buf) / 2

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, v := range p.oneShots {
		v.render(buf, frames, p.sampleRate)
	}
	live := p.oneShots[:0]
	for _, v := range p.oneShots {
		if !v.finished {
			live = append(live, v)
		}
	}
	p.oneShots = live

	for _, v := range p.loops {
		v.render(buf, frames, p.sampleRate)
	}
}

func clamp32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
