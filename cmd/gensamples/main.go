// Command gensamples procedurally synthesizes small placeholder WAV sample
// sets (bird chirps, crowd murmur, etc.) so a theme can be heard end-to-end
// without needing real field recordings yet. They're deliberately simple
// synthesized tones/noise, not a substitute for real samples — see
// README.md.
package main

import (
	"flag"
	"log"
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
	themeName := flag.String("theme", "forest", "which placeholder sample set to generate: forest or market")
	out := flag.String("out", "", "directory to write sample groups into (default: themes/<theme>/samples)")
	seed := flag.Int64("seed", 42, "random seed, for reproducible placeholder assets")
	flag.Parse()

	if *out == "" {
		*out = filepath.Join("themes", *themeName, "samples")
	}
	rng := rand.New(rand.NewSource(*seed))

	switch *themeName {
	case "forest":
		generateForestSamples(rng, *out)
	case "market":
		generateMarketSamples(rng, *out)
	default:
		log.Fatalf("unknown --theme %q (expected forest or market)", *themeName)
	}

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
