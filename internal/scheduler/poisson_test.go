package scheduler

import (
	"math/rand"
	"testing"
)

func TestPoissonCountZeroLambda(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100; i++ {
		if got := PoissonCount(rng, 0); got != 0 {
			t.Fatalf("PoissonCount(rng, 0) = %d, want 0", got)
		}
	}
}

func TestPoissonCountDeterministicWithSeed(t *testing.T) {
	const lambda = 0.6
	seq := func() []int {
		rng := rand.New(rand.NewSource(42))
		out := make([]int, 200)
		for i := range out {
			out[i] = PoissonCount(rng, lambda)
		}
		return out
	}

	a := seq()
	b := seq()

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sequence diverged at index %d: %d != %d", i, a[i], b[i])
		}
	}
}

func TestPoissonCountProducesVariation(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	counts := map[int]int{}
	for i := 0; i < 1000; i++ {
		counts[PoissonCount(rng, 1.5)]++
	}

	if len(counts) < 3 {
		t.Fatalf("expected several distinct event counts, got %v", counts)
	}
	if counts[0] == 0 {
		t.Fatalf("expected some zero-event ticks, got %v", counts)
	}
}

func TestPoissonCountMeanApproximatesLambda(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	const lambda = 4.2
	const trials = 20000

	total := 0
	for i := 0; i < trials; i++ {
		total += PoissonCount(rng, lambda)
	}
	mean := float64(total) / float64(trials)

	if diff := mean - lambda; diff < -0.15 || diff > 0.15 {
		t.Fatalf("mean %.3f too far from lambda %.3f", mean, lambda)
	}
}

func TestPoissonCountLargeLambdaApproximation(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	const lambda = 50.0
	const trials = 5000

	total := 0
	for i := 0; i < trials; i++ {
		total += PoissonCount(rng, lambda)
	}
	mean := float64(total) / float64(trials)

	if diff := mean - lambda; diff < -1.5 || diff > 1.5 {
		t.Fatalf("mean %.3f too far from lambda %.3f", mean, lambda)
	}
}
