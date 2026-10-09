package theme

import (
	"math"

	"github.com/simonwistow/soundscape/internal/metrics"
)

// Weather turns an input into a new one with a life of its own, for a
// theme's sounds to use like any other: a storm that builds up over a
// minute and takes longer to clear, and gusts around its level while it
// lasts, rather than following its input tick by tick.
//
//   - input is what drives it;
//   - build is the time constant, in seconds, with which it rises towards
//     its input's level, and clear the one with which it falls;
//   - gusts is how far it wanders around that level, as a fraction of it,
//     so clear weather is calm and a storm gusts the most.
//
// Weather starts clear (0) and builds up from there.
type Weather struct {
	Name  string  `yaml:"name"`
	Input string  `yaml:"input"`
	Build float64 `yaml:"build"`
	Clear float64 `yaml:"clear"`
	Gusts float64 `yaml:"gusts"`
}

// gustTime is the time constant, in seconds, of a gust: about how long the
// weather takes to wander from one side of its level to the other.
const gustTime = 6.0

type weatherState struct {
	level float64 // where it's got to, following its input
	gust  float64 // how far it's wandered from that, as a fraction of it
}

// applyWeather advances each weather by dt seconds and returns inputs with
// the weather's own values added, leaving inputs itself alone.
func (e *Engine) applyWeather(inputs map[string]float64, dt float64) map[string]float64 {
	if len(e.theme.Weather) == 0 {
		return inputs
	}
	all := make(map[string]float64, len(inputs)+len(e.theme.Weather))
	for k, v := range inputs {
		all[k] = v
	}

	for _, w := range e.theme.Weather {
		st := e.weather[w.Name]
		if st == nil {
			st = &weatherState{}
			e.weather[w.Name] = st
		}

		target := metrics.Clamp(inputs[w.Input], 0, 1)
		tau := w.Build
		if target < st.level {
			tau = w.Clear
		}
		st.level += (target - st.level) * (1 - math.Exp(-dt/tau))

		// An Ornstein-Uhlenbeck process: a random walk that's always drawn
		// back towards no gust at all, and wanders about gusts either way.
		st.gust += -st.gust*dt/gustTime + w.Gusts*math.Sqrt(2*dt/gustTime)*e.rng.NormFloat64()

		all[w.Name] = metrics.Clamp(st.level*(1+st.gust), 0, 1)
	}
	return all
}

// forgetWeather drops the weather a reload has taken away; weather that's
// still there carries on from where it was, with its new settings.
func (e *Engine) forgetWeather() {
	for name := range e.weather {
		gone := true
		for _, w := range e.theme.Weather {
			if w.Name == name {
				gone = false
				break
			}
		}
		if gone {
			delete(e.weather, name)
		}
	}
}
