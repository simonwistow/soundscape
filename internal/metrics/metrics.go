package metrics

import "math"

type Smoother struct {
	seconds float64
	alpha   float64
	value   float64
	set     bool
}

func NewSmoother(seconds float64) *Smoother {
	if seconds <= 1 {
		seconds = 1
	}
	return &Smoother{seconds: seconds, alpha: 1 / seconds}
}

// Seconds is the smoothing time constant the Smoother was built with.
func (s *Smoother) Seconds() float64 { return s.seconds }

func (s *Smoother) Update(v float64) float64 {
	if !s.set {
		s.value = v
		s.set = true
		return v
	}
	s.value += s.alpha * (v - s.value)
	return s.value
}

func Normalise(value, min, max float64) float64 {
	if max <= min {
		return 0
	}
	v := (value - min) / (max - min)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func LogNormalise(value, min, max float64) float64 {
	if value <= 0 {
		return 0
	}
	return Normalise(math.Log1p(value), math.Log1p(min), math.Log1p(max))
}

func Lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}

func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
