// Package render plays a soundscape faster than real time. It steps a
// source tick by tick, and between ticks mixes exactly as much audio as the
// time between them, running whatever comes due along the way on a virtual
// clock. Nothing waits, so a render runs as fast as the machine can mix.
package render

import (
	"context"
	"time"

	"github.com/simonwistow/soundscape/internal/audio"
	"github.com/simonwistow/soundscape/internal/clock"
	"github.com/simonwistow/soundscape/internal/source"
)

type Config struct {
	Source source.Stepper
	// Process takes each tick: conditioning, then the theme engine.
	Process func(timestamp int64, metrics map[string]float64)
	// Clock is the one the engine and outputs time things by.
	Clock *clock.Virtual
	// Mixer, if any, is stepped rather than started; nil when nothing
	// renders audio (a MIDI file on its own, say).
	Mixer      *audio.Mixer
	SampleRate int
	// Duration is how much soundscape to render.
	Duration time.Duration
	// Progress, if set, is told how much has been rendered after each tick.
	Progress func(done time.Duration)
}

// Run renders until Duration, the source runs out, or ctx is cancelled, and
// returns how much it rendered. A source that runs out gets one more
// second, for the last tick's sounds.
func Run(ctx context.Context, cfg Config) time.Duration {
	start := cfg.Clock.Now()
	end := start.Add(cfg.Duration)
	var frames int64

	// renderUntil mixes every block that starts before t, firing timers
	// due by each block's start first.
	renderUntil := func(t time.Time) {
		if cfg.Mixer == nil {
			cfg.Clock.Advance(t)
			return
		}
		for {
			at := start.Add(time.Duration(frames * int64(time.Second) / int64(cfg.SampleRate)))
			if !at.Before(t) {
				return
			}
			cfg.Clock.Advance(at)
			cfg.Mixer.Step()
			frames += audio.BlockFrames
		}
	}

	var first int64
	stepped := false
	last := start
	for ctx.Err() == nil {
		ts, metrics, ok := cfg.Source.Step()
		if !ok {
			end = minTime(end, last.Add(time.Second))
			break
		}
		if !stepped {
			first, stepped = ts, true
		}
		at := start.Add(time.Duration(ts-first) * time.Second)
		if !at.Before(end) {
			break
		}

		renderUntil(at)
		cfg.Clock.Advance(at)
		cfg.Process(ts, metrics)
		last = at

		if cfg.Progress != nil {
			cfg.Progress(at.Sub(start))
		}
	}

	if ctx.Err() == nil {
		renderUntil(end)
		cfg.Clock.Advance(end)
	}
	return cfg.Clock.Now().Sub(start)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
