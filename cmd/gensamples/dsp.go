package main

import (
	"math"
	"math/rand"
)

// whiteNoise returns n uniform random samples in [-1, 1].
func whiteNoise(rng *rand.Rand, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(rng.Float64()*2 - 1)
	}
	return out
}

// lowpass applies a simple one-pole low-pass filter in place, smoothing
// white noise towards a warmer, more watery texture. Smaller alpha means a
// darker (more filtered) sound.
func lowpass(in []float32, alpha float32) []float32 {
	out := make([]float32, len(in))
	var prev float32
	for i, v := range in {
		prev += alpha * (v - prev)
		out[i] = prev
	}
	return out
}

// normalize scales a signal so its peak absolute value equals target.
func normalize(in []float32, target float32) []float32 {
	var peak float32
	for _, v := range in {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak == 0 {
		return in
	}
	scale := target / peak
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = v * scale
	}
	return out
}

// seamlessLoop generates length+overlap samples via gen, then crossfades the
// extra tail into the head so the first `length` samples loop without a
// seam click.
func seamlessLoop(length, overlap int, gen func(n int) []float32) []float32 {
	raw := gen(length + overlap)
	out := make([]float32, length)
	copy(out, raw[:length])
	for i := 0; i < overlap; i++ {
		blend := float32(i) / float32(overlap)
		out[i] = out[i]*(1-blend) + raw[length+i]*blend
	}
	return out
}

// softClip applies a gentle tanh-style saturation for a harsher, more
// "disturbed" texture without harsh digital clipping.
func softClip(in []float32, drive float32) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		x := v * drive
		out[i] = x / (1 + abs32(x))
	}
	return out
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// applyEnvelope multiplies a signal by a raised-sine window, giving a smooth
// attack and decay so one-shot samples never click even before the sample
// player's own fade is applied.
func applyEnvelope(in []float32, power float64) []float32 {
	n := len(in)
	out := make([]float32, n)
	for i, v := range in {
		t := float64(i) / float64(n-1)
		env := math.Sin(math.Pi * t)
		if power != 1 {
			env = math.Pow(env, power)
		}
		out[i] = v * float32(env)
	}
	return out
}
