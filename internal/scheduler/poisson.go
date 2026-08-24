// Package scheduler turns an expected event rate into a count of events for
// a given time window, so a configured rate means "expected events per
// second" rather than "probability of one event per tick".
package scheduler

import (
	"math"
	"math/rand"
)

// poissonApproxThreshold is where Knuth's direct algorithm starts losing
// precision (exp(-lambda) underflows towards zero for large lambda), so
// larger lambdas fall back to a normal approximation instead.
const poissonApproxThreshold = 30

// PoissonCount samples a Poisson-distributed number of events for a window
// where lambda events are expected on average (lambda = rate * elapsed
// seconds). It returns 0 for non-positive lambda.
func PoissonCount(rng *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}

	if lambda > poissonApproxThreshold {
		v := lambda + math.Sqrt(lambda)*rng.NormFloat64()
		if v < 0 {
			return 0
		}
		return int(math.Round(v))
	}

	// Knuth's algorithm: draw uniform randoms until their running product
	// drops below exp(-lambda); the number of draws minus one is Poisson
	// distributed with mean lambda.
	limit := math.Exp(-lambda)
	count := 0
	product := 1.0
	for {
		count++
		product *= rng.Float64()
		if product <= limit {
			return count - 1
		}
	}
}
