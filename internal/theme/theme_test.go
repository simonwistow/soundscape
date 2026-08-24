package theme

import (
	"testing"

	"example.com/fastly-soundscape/internal/output"
)

type recordingOutput struct {
	events []output.Event
}

func (r *recordingOutput) Send(e output.Event) error {
	r.events = append(r.events, e)
	return nil
}

func testTheme() Theme {
	return Theme{
		Name: "test",
		Sources: []Source{
			{Name: "requests", Metric: "requests", Smoothing: 1, Normalise: &Range{Min: 0, Max: 1000}},
		},
		Sounds: []Sound{
			{
				Name:       "birds",
				Type:       "probabilistic",
				Channel:    0,
				Source:     "requests",
				Rate:       &Rate{Min: 0.5, Max: 6},
				Velocity:   &Range{Min: 40, Max: 110},
				DurationMs: 200,
				Notes:      []int{60, 62, 64},
			},
			{
				Name:       "river",
				Type:       "continuous",
				Channel:    1,
				Source:     "requests",
				Controller: 74,
				MinValue:   0,
				MaxValue:   127,
			},
		},
	}
}

func TestEngineDeterministicWithSeed(t *testing.T) {
	run := func() []output.Event {
		out := &recordingOutput{}
		engine := NewEngineWithSeed(testTheme(), out, 12345)
		ts := int64(1000)
		for i := 0; i < 50; i++ {
			engine.Process(ts, map[string]float64{"requests": 800})
			ts++
		}
		return out.events
	}

	a := run()
	b := run()

	if len(a) != len(b) {
		t.Fatalf("event count differs between runs with the same seed: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Describe() != b[i].Describe() {
			t.Fatalf("event %d differs between runs: %q vs %q", i, a[i].Describe(), b[i].Describe())
		}
	}
}

func TestEngineProbabilisticProducesVariedCounts(t *testing.T) {
	th := testTheme()
	// Tune the rate so lambda ~= 1.6/tick: enough headroom for both
	// zero-event ticks and multi-event bursts to show up reliably.
	th.Sounds[0].Rate = &Rate{Min: 0.2, Max: 3}

	out := &recordingOutput{}
	engine := NewEngineWithSeed(th, out, 7)

	noteCounts := make([]int, 0, 200)
	ts := int64(2000)
	for i := 0; i < 200; i++ {
		before := len(out.events)
		engine.Process(ts, map[string]float64{"requests": 500})
		ts++
		noteEvents := 0
		for _, e := range out.events[before:] {
			if _, ok := e.(output.NoteOn); ok {
				noteEvents++
			}
		}
		noteCounts = append(noteCounts, noteEvents)
	}

	seenZero, seenTwoOrMore := false, false
	for _, c := range noteCounts {
		if c == 0 {
			seenZero = true
		}
		if c >= 2 {
			seenTwoOrMore = true
		}
	}

	if !seenZero {
		t.Fatalf("expected at least one tick with zero bird events, got %v", noteCounts)
	}
	if !seenTwoOrMore {
		t.Fatalf("expected at least one tick with 2+ bird events (Poisson bursts), got %v", noteCounts)
	}
}

func TestEngineSkipsLongGapEventFlood(t *testing.T) {
	out := &recordingOutput{}
	engine := NewEngineWithSeed(testTheme(), out, 3)

	engine.Process(0, map[string]float64{"requests": 900})
	// A huge gap (e.g. after a dropped connection) should be clamped rather
	// than interpreted as a single enormous tick.
	engine.Process(100000, map[string]float64{"requests": 900})

	noteEvents := 0
	for _, e := range out.events {
		if _, ok := e.(output.NoteOn); ok {
			noteEvents++
		}
	}

	if noteEvents > 200 {
		t.Fatalf("expected clamped tick to bound event flood, got %d note events", noteEvents)
	}
}
