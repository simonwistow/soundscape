package main

import (
	"math"
	"math/rand"
)

func generateMarketSamples(rng *rand.Rand, out string) {
	generateGroup(rng, out+"/chatter", 6, generateChatter)
	generateGroup(rng, out+"/bottles", 4, generateBottleClink)
	generateGroup(rng, out+"/accordion", 4, generateAccordionNote)

	writeOne(out+"/crowd-quiet", "loop.wav", generateNoiseLoop(rng, 3.0, 0.08, 0.45, false))
	writeOne(out+"/crowd-busy", "loop.wav", generateNoiseLoop(rng, 3.0, 0.4, 0.8, true))
}

// generateChatter synthesizes a short "babble" burst standing in for a
// snippet of market conversation: two close, noise-blended tones in a
// voice-like frequency range, evoking a formant without being a real word.
func generateChatter(rng *rand.Rand) []float32 {
	duration := 0.12 + rng.Float64()*0.1
	n := int(duration * sampleRate)

	f1 := 300 + rng.Float64()*250
	f2 := f1 * (1.6 + rng.Float64()*0.5) // a second formant-ish partial

	tone := make([]float32, n)
	phase1, phase2 := 0.0, 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		wobble := 1 + 0.08*math.Sin(2*math.Pi*7*t) // a little vocal wobble
		phase1 += 2 * math.Pi * f1 * wobble / sampleRate
		phase2 += 2 * math.Pi * f2 * wobble / sampleRate
		tone[i] = float32(math.Sin(phase1)*0.7 + math.Sin(phase2)*0.3)
	}

	noise := whiteNoise(rng, n)
	mixed := make([]float32, n)
	for i := range mixed {
		mixed[i] = tone[i]*0.75 + noise[i]*0.2
	}

	return normalize(applyEnvelope(mixed, 0.6), 0.8)
}

// generateBottleClink synthesizes a short bright metallic ring plus a sharp
// noise transient, standing in for a dropped bottle/glass (the "errors"
// disturbance sound, mirroring the forest's woodpecker).
func generateBottleClink(rng *rand.Rand) []float32 {
	duration := 0.18 + rng.Float64()*0.12
	n := int(duration * sampleRate)
	out := make([]float32, n)

	ringFreq := 2200 + rng.Float64()*1800
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		decay := math.Exp(-t * 14)
		ring := math.Sin(2*math.Pi*ringFreq*t) * decay
		out[i] = float32(ring * 0.8)
	}

	clickLen := samplesFor(0.006)
	for i := 0; i < clickLen && i < n; i++ {
		t := float64(i) / sampleRate
		out[i] += float32((rng.Float64()*2 - 1) * 0.7 * math.Exp(-t*250))
	}

	return normalize(out, 0.85)
}

// generateAccordionNote synthesizes a short sustained tone with a couple of
// harmonics and slow vibrato, standing in for a distant street-market
// accordion flourish. Uses an attack/sustain/release envelope rather than
// the chirps' symmetric raised-sine window, since a held note shouldn't
// fade immediately after its attack.
func generateAccordionNote(rng *rand.Rand) []float32 {
	notes := []float64{220, 246.94, 277.18, 329.63} // A3, B3, C#4, E4: a simple consonant set
	fundamental := notes[rng.Intn(len(notes))]

	duration := 0.5 + rng.Float64()*0.3
	n := int(duration * sampleRate)
	out := make([]float32, n)

	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		vibrato := 1 + 0.01*math.Sin(2*math.Pi*5*t)
		f := fundamental * vibrato
		sig := math.Sin(2*math.Pi*f*t)*0.6 +
			math.Sin(2*math.Pi*f*2*t)*0.25 +
			math.Sin(2*math.Pi*f*3*t)*0.1
		out[i] = float32(sig)
	}

	attack := samplesFor(0.03)
	release := samplesFor(0.15)
	for i := 0; i < n; i++ {
		env := float32(1)
		if i < attack {
			env = float32(i) / float32(attack)
		}
		if remaining := n - i; remaining < release {
			r := float32(remaining) / float32(release)
			if r < env {
				env = r
			}
		}
		out[i] *= env
	}

	return normalize(out, 0.75)
}
