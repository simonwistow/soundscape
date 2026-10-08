package theme

import (
	"math"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
	"github.com/simonwistow/soundscape/internal/metrics"
	"github.com/simonwistow/soundscape/internal/scheduler"
)

// A flock sound is a group of callers that arrive together, pass over and
// leave, rather than a stream of unrelated calls:
//
//   - rate is how many flocks arrive per second, like a probabilistic
//     sound's rate;
//   - size is how many members a flock has, from size.min when its input
//     is 0 to size.max when it's 1 (give or take a quarter, flock to
//     flock);
//   - pass is how many seconds a flock takes to go by, picked at random
//     from the range for each flock;
//   - call_rate is how many times a second each member calls.
//
// While a flock passes it swells in, peaks overhead and fades away, and
// sweeps across the stereo field from one side to the other. Each member
// keeps its own voice for the whole pass - its own pitch (from
// pitch_jitter) for samples, or its own note for notes - and its own place
// in the flock, a little to one side of the middle.
//
// MIDI has no per-note pan, so note flocks only swell and fade.

const (
	// flockSweep is how far towards the edges a flock comes from and goes
	// to; members sit up to flockWidth either side of its middle.
	flockSweep = 0.8
	flockWidth = 0.2

	// callWobble varies each call's pitch by up to this fraction either way
	// around its member's own, so a member doesn't repeat itself exactly.
	callWobble = 0.02
)

type flock struct {
	sound         string
	start, length float64 // in Engine.clock seconds
	from, to      float64 // pan at the start and end of the pass
	members       []flockMember
}

type flockMember struct {
	pitch float64 // samples: playback ratio
	note  int     // notes: the one it calls on
	pan   float64 // offset from the flock's middle
}

// launchFlocks starts the flocks that arrive during this tick. They're
// heard from advanceFlocks, which runs once every sound has had its turn.
func (e *Engine) launchFlocks(sound Sound, value, dt float64) {
	if sound.Rate == nil || sound.Size == nil || sound.Pass == nil {
		return
	}
	if sound.Output != "sample" && len(sound.Notes) == 0 {
		return
	}

	rate := metrics.Lerp(sound.Rate.Min, sound.Rate.Max, value)
	for n := scheduler.PoissonCount(e.rng, rate*dt); n > 0; n-- {
		f := &flock{
			sound:  sound.Name,
			start:  e.clock + e.rng.Float64()*dt,
			length: metrics.Lerp(sound.Pass.Min, sound.Pass.Max, e.rng.Float64()),
			from:   -flockSweep,
			to:     flockSweep,
		}
		if e.rng.Intn(2) == 0 {
			f.from, f.to = f.to, f.from
		}

		size := metrics.Lerp(sound.Size.Min, sound.Size.Max, value) * (0.75 + 0.5*e.rng.Float64())
		for i := max(1, int(math.Round(size))); i > 0; i-- {
			m := flockMember{pitch: 1, pan: (e.rng.Float64()*2 - 1) * flockWidth}
			if sound.PitchJitter != nil {
				m.pitch = metrics.Lerp(sound.PitchJitter.Min, sound.PitchJitter.Max, e.rng.Float64())
			}
			if len(sound.Notes) > 0 {
				m.note = sound.Notes[e.rng.Intn(len(sound.Notes))]
			}
			f.members = append(f.members, m)
		}
		e.flocks = append(e.flocks, f)
	}
}

// advanceFlocks schedules the calls each flock makes between now and the
// next tick, and forgets flocks that have gone by. A flock whose sound has
// been reloaded away goes with it; one whose sound is still there carries
// on with its new settings.
func (e *Engine) advanceFlocks(inputs map[string]float64, dt float64) {
	sounds := make(map[string]Sound)
	for _, s := range e.theme.Sounds {
		if s.Type == "flock" {
			sounds[s.Name] = s
		}
	}

	live := e.flocks[:0]
	for _, f := range e.flocks {
		sound, ok := sounds[f.sound]
		if !ok || f.start+f.length <= e.clock {
			continue
		}
		live = append(live, f)

		value := metrics.Clamp(inputs[sound.Input], 0, 1)
		for _, m := range f.members {
			for n := scheduler.PoissonCount(e.rng, sound.CallRate*dt); n > 0; n-- {
				at := e.clock + e.rng.Float64()*dt
				if at < f.start || at >= f.start+f.length {
					continue
				}
				ev := e.flockCall(sound, f, m, value, (at-f.start)/f.length)
				e.after(time.Duration((at-e.clock)*float64(time.Second)), func() { _ = e.output.Send(ev) })
			}
		}
	}
	clear(e.flocks[len(live):])
	e.flocks = live
}

// flockCall is one member's call, progress (0..1) of the way through its
// flock's pass.
func (e *Engine) flockCall(sound Sound, f *flock, m flockMember, value, progress float64) event.Event {
	// Quiet at either end, loudest overhead.
	swell := math.Sin(math.Pi * progress)

	duration := sound.DurationMs
	if duration <= 0 {
		duration = 300
	}

	if sound.Output == "sample" {
		velocity := 0.8
		if sound.Velocity != nil {
			velocity = metrics.Lerp(sound.Velocity.Min, sound.Velocity.Max, value)
		}
		return event.Sample{
			Actor:      sound.Name,
			Channel:    sound.Channel,
			Group:      sound.SampleGroup,
			Pitch:      m.pitch * (1 + (e.rng.Float64()*2-1)*callWobble),
			Velocity:   velocity * swell,
			Pan:        metrics.Clamp(f.from+(f.to-f.from)*progress+m.pan, -1, 1),
			DurationMs: duration,
		}
	}

	velocity := 80.0
	if sound.Velocity != nil {
		velocity = metrics.Lerp(sound.Velocity.Min, sound.Velocity.Max, value)
	}
	return event.Note{
		Actor:      sound.Name,
		Channel:    sound.Channel,
		Pitch:      m.note,
		Velocity:   int(math.Round(velocity * swell)),
		DurationMs: duration,
	}
}
