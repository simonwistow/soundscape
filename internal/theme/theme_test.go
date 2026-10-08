package theme

import (
	"testing"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
)

type recordingOutput struct {
	events []event.Event
}

func (r *recordingOutput) Send(e event.Event) error {
	r.events = append(r.events, e)
	return nil
}

func testTheme() Theme {
	return Theme{
		Name: "test",
		Sounds: []Sound{
			{
				Name:       "birds",
				Type:       "probabilistic",
				Channel:    0,
				Input:      "activity",
				Rate:       &Rate{Min: 0.5, Max: 6},
				Velocity:   &Range{Min: 40, Max: 110},
				DurationMs: 200,
				Notes:      []int{60, 62, 64},
			},
			{
				Name:       "river",
				Type:       "continuous",
				Channel:    1,
				Input:      "activity",
				Controller: 74,
				MinValue:   0,
				MaxValue:   127,
			},
		},
	}
}

func TestEngineDeterministicWithSeed(t *testing.T) {
	run := func() []event.Event {
		out := &recordingOutput{}
		engine := NewEngineWithSeed(testTheme(), out, 12345)
		ts := int64(1000)
		for i := 0; i < 50; i++ {
			engine.Process(ts, map[string]float64{"activity": 0.97})
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
		engine.Process(ts, map[string]float64{"activity": 0.9})
		ts++
		noteEvents := 0
		for _, e := range out.events[before:] {
			if _, ok := e.(event.Note); ok {
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

	engine.Process(0, map[string]float64{"activity": 0.985})
	// A huge gap (e.g. after a dropped connection) should be clamped rather
	// than interpreted as a single enormous tick.
	engine.Process(100000, map[string]float64{"activity": 0.985})

	noteEvents := 0
	for _, e := range out.events {
		if _, ok := e.(event.Note); ok {
			noteEvents++
		}
	}

	if noteEvents > 200 {
		t.Fatalf("expected clamped tick to bound event flood, got %d note events", noteEvents)
	}
}

func TestEngineReloadAppliesNewTheme(t *testing.T) {
	out := &recordingOutput{}
	engine := NewEngineWithSeed(testTheme(), out, 1)
	engine.Process(0, map[string]float64{"activity": 0.9})

	reloaded := testTheme()
	reloaded.Sounds[1].Controller = 99 // change the river's CC controller
	engine.Reload(reloaded)

	out.events = nil
	engine.Process(1, map[string]float64{"activity": 0.9})

	found := false
	for _, e := range out.events {
		if c, ok := e.(event.Control); ok && c.Actor == "river" {
			found = true
			if c.Controller != 99 {
				t.Fatalf("Controller = %d, want 99 after reload", c.Controller)
			}
		}
	}
	if !found {
		t.Fatalf("expected a river control event after reload")
	}
}

func TestEngineMissingInputReadsAsZero(t *testing.T) {
	out := &recordingOutput{}
	engine := NewEngineWithSeed(testTheme(), out, 1)
	engine.Process(0, map[string]float64{})

	for _, e := range out.events {
		if c, ok := e.(event.Control); ok && c.Actor == "river" && c.Value != 0 {
			t.Fatalf("river CC = %v with no input, want 0 (min_value)", c.Value)
		}
	}
}

func TestInputs(t *testing.T) {
	th := testTheme()
	th.Sounds = append(th.Sounds, Sound{Name: "alarm", Input: "trouble"})
	got := Inputs(th)
	if len(got) != 2 || got[0] != "activity" || got[1] != "trouble" {
		t.Fatalf("Inputs = %v, want [activity trouble]", got)
	}
}

func TestEngineSampleOutputEmitsSampleEvents(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{
				Name:        "birds",
				Type:        "probabilistic",
				Output:      "sample",
				Input:       "activity",
				Rate:        &Rate{Min: 3, Max: 3},
				Velocity:    &Range{Min: 0.6, Max: 0.9},
				DurationMs:  150,
				SampleGroup: "samples/birds",
				PitchJitter: &Range{Min: 0.9, Max: 1.1},
			},
		},
	}

	out := &recordingOutput{}
	engine := NewEngineWithSeed(th, out, 1)
	engine.Process(0, map[string]float64{"activity": 0.9})
	engine.Process(1, map[string]float64{"activity": 0.9})

	if len(out.events) == 0 {
		t.Fatalf("expected at least one sample event")
	}
	for _, e := range out.events {
		s, ok := e.(event.Sample)
		if !ok {
			t.Fatalf("expected event.Sample, got %T", e)
		}
		if s.Loop {
			t.Fatalf("probabilistic sample sound should not set Loop")
		}
		if s.Group != th.Sounds[0].SampleGroup {
			t.Fatalf("Group = %q, want %q", s.Group, th.Sounds[0].SampleGroup)
		}
		if s.Pitch < 0.9 || s.Pitch > 1.1 {
			t.Fatalf("Pitch = %v, want within pitch_jitter range", s.Pitch)
		}
	}
}

func TestEngineSampleLoopEmitsLoopEvents(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{
				Name:        "river-gentle",
				Type:        "continuous",
				Output:      "sample_loop",
				Input:       "flow",
				SampleGroup: "samples/river-gentle",
				MinValue:    1,
				MaxValue:    0,
			},
			{
				Name:        "river-rushing",
				Type:        "continuous",
				Output:      "sample_loop",
				Input:       "flow",
				SampleGroup: "samples/river-rushing",
				MinValue:    0,
				MaxValue:    1,
			},
		},
	}

	out := &recordingOutput{}
	engine := NewEngineWithSeed(th, out, 1)
	engine.Process(0, map[string]float64{"flow": 0.9})

	if len(out.events) != 2 {
		t.Fatalf("expected 2 loop events, got %d", len(out.events))
	}

	gentle := out.events[0].(event.Sample)
	rushing := out.events[1].(event.Sample)

	if !gentle.Loop || !rushing.Loop {
		t.Fatalf("expected both sample_loop sounds to set Loop=true")
	}
	if gentle.Velocity > 0.2 {
		t.Fatalf("gentle layer gain should fall as flow rises, got %v", gentle.Velocity)
	}
	if rushing.Velocity < 0.8 {
		t.Fatalf("rushing layer gain should rise with flow, got %v", rushing.Velocity)
	}
}

func TestEngineSpreadDelaysEventsWithinTheTick(t *testing.T) {
	th := testTheme()
	th.Sounds = th.Sounds[:1]
	th.Sounds[0].Spread = true

	out := &recordingOutput{}
	engine := NewEngineWithSeed(th, out, 7)
	var delays []time.Duration
	engine.after = func(d time.Duration, f func()) {
		delays = append(delays, d)
		f()
	}

	// Ticks two seconds apart, so the spread should reach past one second.
	ts := int64(1000)
	for i := 0; i < 50; i++ {
		engine.Process(ts, map[string]float64{"activity": 1})
		ts += 2
	}

	if len(delays) == 0 {
		t.Fatal("expected spread events to be scheduled")
	}
	if len(out.events) != len(delays) {
		t.Fatalf("sent %d events, scheduled %d", len(out.events), len(delays))
	}
	var max time.Duration
	for _, d := range delays {
		if d < 0 || d >= 2*time.Second {
			t.Fatalf("delay %v outside the 2s tick", d)
		}
		if d > max {
			max = d
		}
	}
	if max <= time.Second {
		t.Fatalf("longest delay %v; expected some beyond 1s with 2s ticks", max)
	}
}

func TestEngineWithoutSpreadSendsImmediately(t *testing.T) {
	out := &recordingOutput{}
	engine := NewEngineWithSeed(testTheme(), out, 7)
	engine.after = func(time.Duration, func()) {
		t.Fatal("events without spread should not be delayed")
	}
	ts := int64(1000)
	for i := 0; i < 20; i++ {
		engine.Process(ts, map[string]float64{"activity": 1})
		ts++
	}
	if len(out.events) == 0 {
		t.Fatal("expected events")
	}
}

func intp(v int) *int { return &v }

func TestEngineSelectsProgramsBeforeNotesAndAfterReload(t *testing.T) {
	th := testTheme()
	th.Sounds[0].Program = intp(11)
	th.Sounds[1].Program = intp(32)
	th.Sounds[1].Bank = 8

	out := &recordingOutput{}
	engine := NewEngineWithSeed(th, out, 1)
	engine.Process(1000, map[string]float64{"activity": 1})
	engine.Process(1001, map[string]float64{"activity": 1})

	var programs []event.Program
	for i, e := range out.events {
		if p, ok := e.(event.Program); ok {
			if i >= 2 {
				t.Fatalf("program change at event %d, after other events", i)
			}
			programs = append(programs, p)
		}
	}
	want := []event.Program{{Channel: 0, Program: 11}, {Channel: 1, Bank: 8, Program: 32}}
	if len(programs) != 2 || programs[0] != want[0] || programs[1] != want[1] {
		t.Fatalf("programs = %v, want %v once each", programs, want)
	}

	th.Sounds[0].Program = intp(12)
	engine.Reload(th)
	out.events = nil
	engine.Process(1002, map[string]float64{"activity": 1})
	if p, ok := out.events[0].(event.Program); !ok || p.Program != 12 {
		t.Fatalf("first event after reload = %v, want the new program", out.events[0])
	}
}
