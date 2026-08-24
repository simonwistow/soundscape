package theme

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"example.com/fastly-soundscape/internal/metrics"
	"example.com/fastly-soundscape/internal/output"
	"gopkg.in/yaml.v3"
)

type Theme struct {
	Name   string   `yaml:"name"`
	Sources []Source `yaml:"sources"`
	Sounds []Sound   `yaml:"sounds"`
}

type Source struct {
	Name        string  `yaml:"name"`
	Metric      string  `yaml:"metric"`
	Smoothing  float64 `yaml:"smoothing"`
	Normalise   *Range  `yaml:"normalise"`
}

type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

type Sound struct {
	Name          string         `yaml:"name"`
	Type          string         `yaml:"type"`
	Channel       int            `yaml:"channel"`
	Source        string         `yaml:"source"`
	Rate          *Rate          `yaml:"rate"`
	Velocity      *Range         `yaml:"velocity"`
	Notes         []int          `yaml:"notes"`
	DurationMs    int            `yaml:"duration_ms"`
	Controller    int            `yaml:"controller"`
	MinValue      int            `yaml:"min_value"`
	MaxValue      int            `yaml:"max_value"`
}

type Rate struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

type Engine struct {
	theme    Theme
	output   output.Output
	smoothers map[string]*metrics.Smoother
	last     map[string]float64
	rng      *rand.Rand
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

func NewEngine(t Theme, out output.Output) *Engine {
	s := make(map[string]*metrics.Smoother)
	for _, source := range t.Sources {
		seconds := source.Smoothing
		if seconds <= 0 {
			seconds = 1
		}
		s[source.Name] = metrics.NewSmoother(seconds)
	}

	return &Engine{
		theme:    t,
		output:   out,
		smoothers: s,
		last:     make(map[string]float64),
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (e *Engine) Process(_ int64, raw map[string]float64) {
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
			e.processProbabilistic(sound, value)
		case "continuous":
			e.processContinuous(sound, value)
		default:
			fmt.Printf("warning: unknown sound type %q\n", sound.Type)
		}
	}
}

func (e *Engine) processProbabilistic(sound Sound, value float64) {
	if sound.Rate == nil || len(sound.Notes) == 0 {
		return
	}

	rate := metrics.Lerp(sound.Rate.Min, sound.Rate.Max, value)
	if e.rng.Float64() >= rate {
		return
	}

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
		Channel: sound.Channel,
		Note: note,
		Velocity: velocity,
		DurationMs: duration,
	})
}

func (e *Engine) processContinuous(sound Sound, value float64) {
	v := int(metrics.Lerp(float64(sound.MinValue), float64(sound.MaxValue), value))
	_ = e.output.Send(output.CC{
		Channel: sound.Channel,
		Controller: sound.Controller,
		Value: v,
	})
}
