package main

import (
	"math"
	"math/rand"
)

func generateForestSamples(rng *rand.Rand, out string) {
	generateGroup(rng, out+"/birds", 6, generateChirp)
	generateGroup(rng, out+"/woodpecker", 4, generateTaps)
	generateGroup(rng, out+"/splash", 4, generateSplash)

	writeOne(out+"/river-gentle", "loop.wav", generateNoiseLoop(rng, 3.0, 0.05, 0.5, false))
	writeOne(out+"/river-rushing", "loop.wav", generateNoiseLoop(rng, 3.0, 0.35, 0.75, true))
}

// generateChirp synthesizes a short frequency-sweep tone with a touch of
// noise breathiness, standing in for a bird call.
func generateChirp(rng *rand.Rand) []float32 {
	duration := 0.09 + rng.Float64()*0.08
	n := int(duration * sampleRate)

	startFreq := 1800 + rng.Float64()*1500
	endFreq := startFreq + (rng.Float64()*2-1)*1200

	tone := make([]float32, n)
	phase := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		freq := startFreq + (endFreq-startFreq)*t
		phase += 2 * math.Pi * freq / sampleRate
		tone[i] = float32(math.Sin(phase))
	}

	noise := whiteNoise(rng, n)
	mixed := make([]float32, n)
	for i := range mixed {
		mixed[i] = tone[i]*0.92 + noise[i]*0.06
	}

	return normalize(applyEnvelope(mixed, 0.7), 0.85)
}

// generateTaps synthesizes a short burst of 3-6 percussive knocks, standing
// in for a woodpecker (used as the "errors" disturbance sound).
func generateTaps(rng *rand.Rand) []float32 {
	numTaps := 3 + rng.Intn(4)
	totalDuration := 0.24 + rng.Float64()*0.14
	n := int(totalDuration * sampleRate)
	out := make([]float32, n)

	pos := 0.01 * sampleRate
	for t := 0; t < numTaps; t++ {
		knockFreq := 150 + rng.Float64()*90
		tapLen := samplesFor(0.025)

		for i := 0; i < tapLen; i++ {
			idx := int(pos) + i
			if idx >= n {
				break
			}
			tt := float64(i) / sampleRate
			decay := math.Exp(-tt * 90)
			knock := math.Sin(2*math.Pi*knockFreq*tt) * decay

			var click float64
			if i < samplesFor(0.003) {
				click = (rng.Float64()*2 - 1) * 0.5 * math.Exp(-tt*400)
			}
			out[idx] += float32(knock*0.85 + click)
		}
		pos += (0.045 + rng.Float64()*0.03) * sampleRate
	}

	return normalize(out, 0.85)
}

// generateSplash synthesizes a short bright noise burst.
func generateSplash(rng *rand.Rand) []float32 {
	n := int((0.07 + rng.Float64()*0.05) * sampleRate)
	noise := whiteNoise(rng, n)
	bright := lowpass(noise, 0.85) // alpha close to 1 = barely filtered, stays bright
	return normalize(applyEnvelope(bright, 0.5), 0.8)
}
