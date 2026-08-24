package theme

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"example.com/fastly-soundscape/internal/metrics"
	"example.com/fastly-soundscape/internal/output"
	"example.com/fastly-soundscape/internal/scheduler"
	"gopkg.in/yaml.v3"
)

// maxTickSeconds bounds how large a single Process() step's elapsed time can
// be treated as. Without this, a long pause between records (e.g. a dropped
// connection) would otherwise be interpreted as one huge tick and unleash a
// flood of catch-up events instead of naturally resuming.
const maxTickSeconds = 10.0

type Theme struct {
	Name    string   `yaml:"name"`
	Sources []Source `yaml:"sources"`
	Sounds  []Sound  `yaml:"sounds"`
}

type Source struct {
	Name      string  `yaml:"name"`
	Metric    string  `yaml:"metric"`
	Smoothing float64 `yaml:"smoothing"`
	Normalise *Range  `yaml:"normalise"`
}

type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

type Sound struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Channel    int    `yaml:"channel"`
	Source     string `yaml:"source"`
	Rate       *Rate  `yaml:"rate"`
	Velocity   *Range `yaml:"velocity"`
	Notes      []int  `yaml:"notes"`
	DurationMs int    `yaml:"duration_ms"`
	Controller int    `yaml:"controller"`
	MinValue   int    `yaml:"min_value"`
	MaxValue   int    `yaml:"max_value"`
}

type Rate struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

type Engine struct {
	theme     Theme
	output    output.Output
	smoothers map[string]*metrics.Smoother
	last      map[string]float64
	rng       *rand.Rand
	lastTick  int64
}

func Load(path string) (Theme, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	var t Theme
	if err := yaml.Unmarshal(b, &t); err != nil {
		return Theme{}, err
	}
	if t.Name == "" {
		return Theme{}, fmt.Errorf("theme name is required")
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
	s := make(map[string]*metrics.Smoother)
	for _, source := range t.Sources {
		seconds := source.Smoothing
		if seconds <= 0 {
			seconds = 1
		}
		s[source.Name] = metrics.NewSmoother(seconds)
	}

	return &Engine{
		theme:     t,
		output:    out,
		smoothers: s,
		last:      make(map[string]float64),
		rng:       rand.New(rand.NewSource(seed)),
	}
}

func (e *Engine) Process(timestamp int64, raw map[string]float64) {
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

	values := make(map[string]float64)

	for _, source := range e.theme.Sources {
		v := raw[source.Metric]
		if s := e.smoothers[source.Name]; s != nil {
			v = s.Update(v)
		}

		if source.Normalise != nil {
			v = metrics.LogNormalise(v, source.Normalise.Min, source.Normalise.Max)
		}

		values[source.Name] = v
		e.last[source.Name] = v
	}

	for _, sound := range e.theme.Sounds {
		value := values[sound.Source]

		switch sound.Type {
		case "probabilistic":
			e.processProbabilistic(sound, value, dt)
		case "continuous":
			e.processContinuous(sound, value)
		default:
			fmt.Printf("warning: unknown sound type %q\n", sound.Type)
		}
	}
}

// processProbabilistic treats sound.Rate as an expected number of events per
// second (not a per-tick probability) and draws an event count from a
// Poisson process, so a tick can naturally produce zero, one, or several
// events instead of at most one.
func (e *Engine) processProbabilistic(sound Sound, value float64, dt float64) {
	if sound.Rate == nil || len(sound.Notes) == 0 {
		return
	}

	ratePerSecond := metrics.Lerp(sound.Rate.Min, sound.Rate.Max, value)
	lambda := ratePerSecond * dt
	count := scheduler.PoissonCount(e.rng, lambda)

	for i := 0; i < count; i++ {
		note := sound.Notes[e.rng.Intn(len(sound.Notes))]
		velocity := 80
		if sound.Velocity != nil {
			velocity = int(metrics.Lerp(sound.Velocity.Min, sound.Velocity.Max, value))
		}

		duration := sound.DurationMs
		if duration <= 0 {
			duration = 300
		}

		_ = e.output.Send(output.NoteOn{
			Channel:    sound.Channel,
			Note:       note,
			Velocity:   velocity,
			DurationMs: duration,
		})
	}
}

func (e *Engine) processContinuous(sound Sound, value float64) {
	v := int(metrics.Lerp(float64(sound.MinValue), float64(sound.MaxValue), value))
	_ = e.output.Send(output.CC{
		Channel:    sound.Channel,
		Controller: sound.Controller,
		Value:      v,
	})
}
