// Command gensamples procedurally synthesizes small placeholder WAV sample
// sets (bird chirps, woodpecker taps, river beds, splashes) so a theme can
// be heard end-to-end without needing real field recordings yet. They're
// deliberately simple synthesized tones/noise, not a substitute for real
// samples — see README.md.
package main

import (
	"flag"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"

	"example.com/fastly-soundscape/internal/sampler"
)

const sampleRate = 44100

// samplesFor converts a duration in seconds to a sample count. Routing
// through a function call (rather than inlining `int(seconds * sampleRate)`
// at call sites with literal seconds) avoids Go's constant-expression
// truncation rule, since the multiplication then happens at runtime.
func samplesFor(seconds float64) int {
	return int(seconds * sampleRate)
}

func main() {
	out := flag.String("out", "themes/forest/samples", "directory to write sample groups into")
	seed := flag.Int64("seed", 42, "random seed, for reproducible placeholder assets")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	generateGroup(rng, filepath.Join(*out, "birds"), 6, generateChirp)
	generateGroup(rng, filepath.Join(*out, "woodpecker"), 4, generateTaps)
	generateGroup(rng, filepath.Join(*out, "splash"), 4, generateSplash)

	writeOne(filepath.Join(*out, "river-gentle"), "loop.wav",
		generateRiverLayer(rng, 3.0, 0.05, 0.5, false))
	writeOne(filepath.Join(*out, "river-rushing"), "loop.wav",
		generateRiverLayer(rng, 3.0, 0.35, 0.75, true))

	log.Printf("wrote placeholder sample groups under %s", *out)
}

func generateGroup(rng *rand.Rand, dir string, count int, gen func(*rand.Rand) []float32) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", dir, err)
	}
	for i := 1; i <= count; i++ {
		path := filepath.Join(dir, "variant-"+strconv.Itoa(i)+".wav")
		if err := sampler.WriteWAV(path, gen(rng), sampleRate); err != nil {
			log.Fatalf("writing %s: %v", path, err)
		}
	}
}

func writeOne(dir, name string, data []float32) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := sampler.WriteWAV(path, data, sampleRate); err != nil {
		log.Fatalf("writing %s: %v", path, err)
	}
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

// generateRiverLayer synthesizes a seamlessly-loopable bed of filtered noise.
// Lower filterAlpha sounds darker/gentler; modulate adds slow amplitude
// movement for a more turbulent "rushing" character.
func generateRiverLayer(rng *rand.Rand, seconds float64, filterAlpha, amplitude float32, modulate bool) []float32 {
	length := int(seconds * sampleRate)
	overlap := samplesFor(0.3)

	gen := func(n int) []float32 {
		filtered := lowpass(whiteNoise(rng, n), filterAlpha)
		if modulate {
			for i := range filtered {
				t := float64(i) / sampleRate
				lfo := 1 + 0.15*math.Sin(2*math.Pi*0.3*t)
				filtered[i] *= float32(lfo)
			}
		}
		return filtered
	}

	return normalize(seamlessLoop(length, overlap, gen), amplitude)
}
