package theme

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
	"github.com/simonwistow/soundscape/internal/metrics"
	"github.com/simonwistow/soundscape/internal/output"
	"github.com/simonwistow/soundscape/internal/scheduler"
	"gopkg.in/yaml.v3"
)

// maxTickSeconds bounds how large a single Process() step's elapsed time can
// be treated as. Without this, a long pause between records (e.g. a dropped
// connection) would otherwise be interpreted as one huge tick and unleash a
// flood of catch-up events instead of naturally resuming.
const maxTickSeconds = 10.0

// Theme is a set of sounds driven by abstract, source-independent inputs
// (named 0..1 values such as "activity" or "trouble"). Which data source
// feeds each input, and how it's scaled into 0..1, is a mapping's job (see
// internal/mapping), so one theme can be played from any source.
type Theme struct {
	Name   string  `yaml:"name"`
	Sounds []Sound `yaml:"sounds"`
}

type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

// Sound describes one behaviour. Type selects the scheduling primitive
// ("probabilistic", "continuous" or "flock", see flock.go); Output selects
// how that primitive is realised: "note" (default for probabilistic and
// flock) and "cc" (default for
// continuous) emit MIDI-style events for the SoundFont backend, while
// "sample" and "sample_loop" emit event.Sample values for the WAV sample
// player. A "sample_loop" sound's ramped value (min_value..max_value) is
// used directly as loop gain, so two sample_loop sounds sharing an input
// with opposite min/max ramps crossfade against each other.
//
// A probabilistic sound's events otherwise all happen as the tick that
// produced them is processed, so several in one tick sound together, on a
// beat set by how often the source reports. Spread scatters them at random
// across the time until the next tick (about a second, for most sources)
// instead.
//
// Program (and optionally Bank) picks the instrument a note or cc sound's
// channel plays with, on a SoundFont or General MIDI synth: 0 is a piano,
// 32 an acoustic bass, and so on (see `soundscape soundfont-presets`).
// It's a property of the channel, so sounds sharing a channel must agree.
type Sound struct {
	Name        string  `yaml:"name"`
	Type        string  `yaml:"type"`
	Output      string  `yaml:"output"`
	Channel     int     `yaml:"channel"`
	Input       string  `yaml:"input"`
	Rate        *Rate   `yaml:"rate"`
	Velocity    *Range  `yaml:"velocity"`
	Notes       []int   `yaml:"notes"`
	DurationMs  int     `yaml:"duration_ms"`
	Controller  int     `yaml:"controller"`
	MinValue    float64 `yaml:"min_value"`
	MaxValue    float64 `yaml:"max_value"`
	SampleGroup string  `yaml:"sample_group"`
	Pitch       float64 `yaml:"pitch"`
	PitchJitter *Range  `yaml:"pitch_jitter"`
	Spread      bool    `yaml:"spread"`
	Program     *int    `yaml:"program"`
	Bank        int     `yaml:"bank"`

	// Flock settings; see flock.go.
	Size     *Range  `yaml:"size"`
	Pass     *Range  `yaml:"pass"`
	CallRate float64 `yaml:"call_rate"`
}

type Rate struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

type Engine struct {
	mu       sync.Mutex
	theme    Theme
	output   output.Output
	rng      *rand.Rand
	lastTick int64

	// clock is seconds of soundscape time: the sum of every tick's dt,
	// which is what flocks are timed against.
	clock  float64
	flocks []*flock

	// programsSent is whether the theme's instruments have been selected
	// since it was loaded.
	programsSent bool

	// after runs f once d has passed; time.AfterFunc, except in tests.
	after func(d time.Duration, f func())
}

func Load(path string) (Theme, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	// Strict decoding, so a misspelt key - or a theme still in the old
	// format with sources/metric, which now belong in a mapping - is an
	// error instead of a silently ignored field.
	var t Theme
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return Theme{}, fmt.Errorf("%s: %w", path, err)
	}
	if t.Name == "" {
		return Theme{}, fmt.Errorf("theme name is required")
	}

	// sample_group paths are conventionally relative to the theme file
	// itself, so a theme directory stays self-contained wherever it's run
	// from.
	baseDir := filepath.Dir(path)
	for i, sound := range t.Sounds {
		if sound.SampleGroup != "" && !filepath.IsAbs(sound.SampleGroup) {
			t.Sounds[i].SampleGroup = filepath.Join(baseDir, sound.SampleGroup)
		}
	}

	return t, nil
}

// NewEngine builds an Engine whose randomness is seeded from the current
// time, suitable for live operation where every run should sound different.
func NewEngine(t Theme, out output.Output) *Engine {
	return NewEngineWithSeed(t, out, time.Now().UnixNano())
}

// NewEngineWithSeed builds an Engine with a fixed random seed, so that
// probabilistic behaviours (and tests that depend on them) are reproducible.
func NewEngineWithSeed(t Theme, out output.Output, seed int64) *Engine {
	return &Engine{
		theme:  t,
		output: out,
		rng:    rand.New(rand.NewSource(seed)),
		after:  func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

// Reload swaps in a new theme definition (e.g. for hot-reloading an edited
// theme file). It's the caller's responsibility to
// validate the new theme first (see Validate) — Reload doesn't check.
func (e *Engine) Reload(t Theme) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.theme = t
	e.programsSent = false
}

// Inputs returns the distinct input names a theme's sounds use, in order of
// first use.
func Inputs(t Theme) []string {
	seen := make(map[string]bool)
	var names []string
	for _, sound := range t.Sounds {
		if sound.Input == "" || seen[sound.Input] {
			continue
		}
		seen[sound.Input] = true
		names = append(names, sound.Input)
	}
	return names
}

// Process advances the theme by one tick. inputs holds the theme's input
// values, each already conditioned into 0..1 (see mapping.Conditioner); a
// missing input reads as 0.
func (e *Engine) Process(timestamp int64, inputs map[string]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	dt := 1.0
	if e.lastTick != 0 {
		if d := float64(timestamp - e.lastTick); d > 0 {
			dt = d
		}
	}
	if dt > maxTickSeconds {
		dt = maxTickSeconds
	}
	e.lastTick = timestamp
	e.clock += dt

	if !e.programsSent {
		e.sendPrograms()
		e.programsSent = true
	}

	for _, sound := range e.theme.Sounds {
		value := metrics.Clamp(inputs[sound.Input], 0, 1)

		switch sound.Type {
		case "probabilistic":
			e.processProbabilistic(sound, value, dt)
		case "continuous":
			e.processContinuous(sound, value)
		case "flock":
			e.launchFlocks(sound, value, dt)
		default:
			fmt.Printf("warning: unknown sound type %q\n", sound.Type)
		}
	}
	e.advanceFlocks(inputs, dt)
}

// sendPrograms selects each channel's instrument, for the channels the
// theme sets one on. Validate makes sure sounds sharing a channel agree, so
// the first one to set it is enough.
func (e *Engine) sendPrograms() {
	done := make(map[int]bool)
	for _, sound := range e.theme.Sounds {
		if sound.Program == nil || done[sound.Channel] {
			continue
		}
		done[sound.Channel] = true
		_ = e.output.Send(event.Program{Channel: sound.Channel, Bank: sound.Bank, Program: *sound.Program})
	}
}

// processProbabilistic treats sound.Rate as an expected number of events per
// second (not a per-tick probability) and draws an event count from a
// Poisson process, so a tick can naturally produce zero, one, or several
// events instead of at most one.
func (e *Engine) processProbabilistic(sound Sound, value float64, dt float64) {
	if sound.Rate == nil {
		return
	}
	if sound.Output == "sample" && sound.SampleGroup == "" {
		return
	}
	if sound.Output != "sample" && len(sound.Notes) == 0 {
		return
	}

	ratePerSecond := metrics.Lerp(sound.Rate.Min, sound.Rate.Max, value)
	lambda := ratePerSecond * dt
	count := scheduler.PoissonCount(e.rng, lambda)

	for i := 0; i < count; i++ {
		var ev event.Event
		var delay time.Duration
		if sound.Spread {
			delay = time.Duration(e.rng.Float64() * dt * float64(time.Second))
		}

		velocity := 80.0
		if sound.Velocity != nil {
			velocity = metrics.Lerp(sound.Velocity.Min, sound.Velocity.Max, value)
		}

		duration := sound.DurationMs
		if duration <= 0 {
			duration = 300
		}

		if sound.Output == "sample" {
			pitch := 1.0
			if sound.PitchJitter != nil {
				pitch = metrics.Lerp(sound.PitchJitter.Min, sound.PitchJitter.Max, e.rng.Float64())
			}
			// A little random pan spread keeps repeated triggers (many
			// birds in a flock) from all sounding like the same point.
			pan := (e.rng.Float64()*2 - 1) * 0.4

			ev = event.Sample{
				Actor:      sound.Name,
				Channel:    sound.Channel,
				Group:      sound.SampleGroup,
				Pitch:      pitch,
				Velocity:   velocity,
				Pan:        pan,
				DurationMs: duration,
			}
		} else {
			ev = event.Note{
				Actor:      sound.Name,
				Channel:    sound.Channel,
				Pitch:      sound.Notes[e.rng.Intn(len(sound.Notes))],
				Velocity:   int(velocity),
				DurationMs: duration,
			}
		}

		if delay == 0 {
			_ = e.output.Send(ev)
			continue
		}
		// Everything random is decided above, under e.mu; only the send
		// waits, and the backends are safe to call from another goroutine.
		e.after(delay, func() { _ = e.output.Send(ev) })
	}
}

func (e *Engine) processContinuous(sound Sound, value float64) {
	v := metrics.Lerp(sound.MinValue, sound.MaxValue, value)

	if sound.Output == "sample_loop" {
		if sound.SampleGroup == "" {
			return
		}
		pitch := sound.Pitch
		if pitch <= 0 {
			pitch = 1
		}
		_ = e.output.Send(event.Sample{
			Actor:    sound.Name,
			Channel:  sound.Channel,
			Group:    sound.SampleGroup,
			Loop:     true,
			Pitch:    pitch,
			Velocity: v,
		})
		return
	}

	_ = e.output.Send(event.Control{
		Actor:      sound.Name,
		Channel:    sound.Channel,
		Controller: sound.Controller,
		Value:      v,
	})
}
