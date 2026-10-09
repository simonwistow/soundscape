package render

import (
	"context"
	"testing"
	"time"

	"github.com/simonwistow/soundscape/internal/audio"
	"github.com/simonwistow/soundscape/internal/clock"
	"github.com/simonwistow/soundscape/internal/source"
)

// ticks is a source of n one-second ticks starting at Unix time 1000, or
// endless with n < 0.
type ticks struct {
	n, i int
}

func (s *ticks) Run(context.Context, source.Emit) error { return nil }

func (s *ticks) Step() (int64, map[string]float64, bool) {
	if s.n >= 0 && s.i >= s.n {
		return 0, nil, false
	}
	s.i++
	return int64(999 + s.i), map[string]float64{"tick": float64(s.i)}, true
}

// countingSink counts the frames it's given.
type countingSink struct{ frames int }

func (c *countingSink) Write(f []float32) error { c.frames += len(f) / 2; return nil }
func (c *countingSink) Close() error            { return nil }

// clockSource records the clock's time each time it's rendered.
type clockSource struct {
	clk   clock.Clock
	times []time.Time
}

func (c *clockSource) Render([]float32) { c.times = append(c.times, c.clk.Now()) }

func TestRunRendersTheDurationAsFastAsItCan(t *testing.T) {
	start := time.Unix(5000, 0)
	clk := clock.NewVirtual(start)
	mixer := audio.NewMixer(44100)
	sink := &countingSink{}
	mixer.AddSink(sink)

	var at []time.Duration
	wall := time.Now()
	rendered := Run(context.Background(), Config{
		Source:     &ticks{n: -1},
		Process:    func(int64, map[string]float64) { at = append(at, clk.Now().Sub(start)) },
		Clock:      clk,
		Mixer:      mixer,
		SampleRate: 44100,
		Duration:   10 * time.Minute,
	})

	if took := time.Since(wall); took > 10*time.Second {
		t.Errorf("10 minutes took %v to render", took)
	}
	if rendered != 10*time.Minute {
		t.Errorf("rendered %v, want 10m", rendered)
	}
	// Every block that starts before the end: 10 minutes, rounded up to a
	// whole block.
	want := (600*44100 + audio.BlockFrames - 1) / audio.BlockFrames * audio.BlockFrames
	if sink.frames != want {
		t.Errorf("mixed %d frames, want %d", sink.frames, want)
	}
	if len(at) != 600 || at[0] != 0 || at[599] != 599*time.Second {
		t.Errorf("ticks processed at %d times, from %v to %v; want 600, from 0s to 9m59s", len(at), at[0], at[len(at)-1])
	}
}

func TestRunFiresTimersAsTheAudioReachesThem(t *testing.T) {
	start := time.Unix(0, 0)
	clk := clock.NewVirtual(start)
	mixer := audio.NewMixer(44100)
	src := &clockSource{clk: clk}
	mixer.AddSource(src)
	mixer.AddSink(&countingSink{})

	var firedAt time.Duration
	Run(context.Background(), Config{
		Source: &ticks{n: 1},
		Process: func(int64, map[string]float64) {
			clk.AfterFunc(300*time.Millisecond, func() { firedAt = clk.Now().Sub(start) })
		},
		Clock:      clk,
		Mixer:      mixer,
		SampleRate: 44100,
		Duration:   time.Minute,
	})

	if firedAt != 300*time.Millisecond {
		t.Errorf("timer fired at %v, want 300ms", firedAt)
	}
	// Each block is mixed with the clock at its own start, so a timer
	// lands within a block of when it was due.
	for i, at := range src.times {
		want := time.Duration(int64(i) * audio.BlockFrames * int64(time.Second) / 44100)
		if got := at.Sub(start); got != want {
			t.Fatalf("block %d mixed at %v, want %v", i, got, want)
		}
	}
}

func TestRunGivesAnEndingSourceOneMoreSecond(t *testing.T) {
	clk := clock.NewVirtual(time.Unix(0, 0))
	rendered := Run(context.Background(), Config{
		Source:     &ticks{n: 3},
		Process:    func(int64, map[string]float64) {},
		Clock:      clk,
		SampleRate: 44100,
		Duration:   time.Hour,
	})
	if rendered != 3*time.Second {
		t.Errorf("rendered %v of a 3-tick source, want 3s (ticks at 0, 1 and 2, plus a second)", rendered)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	clk := clock.NewVirtual(time.Unix(0, 0))
	n := 0
	rendered := Run(ctx, Config{
		Source: &ticks{n: -1},
		Process: func(int64, map[string]float64) {
			if n++; n == 5 {
				cancel()
			}
		},
		Clock:      clk,
		SampleRate: 44100,
		Duration:   time.Hour,
	})
	if rendered != 4*time.Second {
		t.Errorf("rendered %v, want 4s (cancelled during the fifth tick)", rendered)
	}
}
