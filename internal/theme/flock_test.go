package theme

import (
	"math"
	"sort"
	"testing"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
)

func flockTheme() Theme {
	return Theme{
		Name: "test",
		Sounds: []Sound{{
			Name:        "starlings",
			Type:        "flock",
			Output:      "sample",
			Input:       "activity",
			Rate:        &Rate{Min: 0, Max: 0.2},
			Size:        &Range{Min: 2, Max: 8},
			Pass:        &Range{Min: 4, Max: 6},
			CallRate:    3,
			Velocity:    &Range{Min: 0.5, Max: 1},
			SampleGroup: "birds",
			PitchJitter: &Range{Min: 1, Max: 2},
		}},
	}
}

// timedCall is an event and when it was sent, in Engine.clock seconds.
type timedCall struct {
	at float64
	ev event.Event
}

// runFlocks plays engine for ticks one-second ticks with every input at
// value, and returns the calls sent, in the order they'd be heard.
func runFlocks(engine *Engine, value float64, ticks int) []timedCall {
	var calls []timedCall
	engine.after = func(d time.Duration, f func()) {
		out := &recordingOutput{}
		engine.output = out
		f()
		for _, ev := range out.events {
			calls = append(calls, timedCall{engine.clock + d.Seconds(), ev})
		}
	}
	for i := 0; i < ticks; i++ {
		engine.Process(int64(1000+i), map[string]float64{"activity": value})
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].at < calls[j].at })
	return calls
}

func TestFlockSweepsAcrossAndSwells(t *testing.T) {
	th := flockTheme()
	th.Sounds[0].Rate = &Rate{}
	engine := NewEngineWithSeed(th, &recordingOutput{}, 3)
	// One flock, all its members in the middle of it, passing left to
	// right from 2s to 7s.
	engine.flocks = []*flock{{
		sound: "starlings", start: 2, length: 5, from: -0.8, to: 0.8,
		members: []flockMember{{pitch: 1}, {pitch: 1.5}, {pitch: 2}},
	}}

	calls := runFlocks(engine, 1, 10)
	if len(calls) < 20 {
		t.Fatalf("got %d calls, want plenty from 3 members calling 3 times a second for 5s", len(calls))
	}
	for _, c := range calls {
		s, ok := c.ev.(event.Sample)
		if !ok {
			t.Fatalf("flock sent %T, want event.Sample", c.ev)
		}
		if c.at < 2 || c.at >= 7 {
			t.Fatalf("call at %.2fs, outside the pass", c.at)
		}
		progress := (c.at - 2) / 5
		if want := -0.8 + 1.6*progress; math.Abs(s.Pan-want) > 1e-9 {
			t.Errorf("at %.2fs pan = %.3f, want %.3f", c.at, s.Pan, want)
		}
		if want := math.Sin(math.Pi * progress); math.Abs(s.Velocity-want) > 1e-9 {
			t.Errorf("at %.2fs gain = %.3f, want %.3f", c.at, s.Velocity, want)
		}
	}
	if len(engine.flocks) != 0 {
		t.Fatal("the flock should be forgotten once it has gone by")
	}
}

func TestFlockSizeFollowsInput(t *testing.T) {
	th := flockTheme()
	th.Sounds[0].Rate = &Rate{Min: 0.2, Max: 0.2}

	engine := NewEngineWithSeed(th, &recordingOutput{}, 5)
	engine.after = func(time.Duration, func()) {}
	sizes := func(value float64) (total int) {
		engine.flocks = nil
		for i := 0; i < 100; i++ {
			engine.Process(int64(2000+i), map[string]float64{"activity": value})
			for _, f := range engine.flocks {
				if f.start > engine.clock-1 {
					total += len(f.members)
				}
			}
		}
		return total
	}

	quiet, busy := sizes(0), sizes(1)
	if busy < 2*quiet {
		t.Fatalf("members in quiet flocks %d, busy flocks %d; want busy flocks much bigger", quiet, busy)
	}
}

func TestNoteFlockMembersKeepTheirNotes(t *testing.T) {
	th := flockTheme()
	th.Sounds[0].Output = "note"
	th.Sounds[0].SampleGroup = ""
	th.Sounds[0].Rate = &Rate{}
	th.Sounds[0].Notes = []int{60, 62, 64, 67, 69}
	th.Sounds[0].Velocity = &Range{Min: 60, Max: 100}
	engine := NewEngineWithSeed(th, &recordingOutput{}, 3)
	engine.flocks = []*flock{{
		sound: "starlings", start: 2, length: 5,
		members: []flockMember{{note: 62}, {note: 69}},
	}}

	calls := runFlocks(engine, 1, 10)
	if len(calls) == 0 {
		t.Fatal("expected the flock to call")
	}
	for _, c := range calls {
		n, ok := c.ev.(event.Note)
		if !ok {
			t.Fatalf("flock sent %T, want event.Note", c.ev)
		}
		if n.Pitch != 62 && n.Pitch != 69 {
			t.Fatalf("a member called on %d, not its own note", n.Pitch)
		}
		if want := int(math.Round(100 * math.Sin(math.Pi*(c.at-2)/5))); n.Velocity != want {
			t.Errorf("at %.2fs velocity = %d, want %d", c.at, n.Velocity, want)
		}
	}
}

func TestFlocksGoWhenTheirSoundIsReloadedAway(t *testing.T) {
	th := flockTheme()
	th.Sounds[0].Rate = &Rate{Min: 1, Max: 1}
	engine := NewEngineWithSeed(th, &recordingOutput{}, 1)
	engine.after = func(time.Duration, func()) {}

	engine.Process(1000, map[string]float64{"activity": 1})
	engine.Process(1001, map[string]float64{"activity": 1})
	if len(engine.flocks) == 0 {
		t.Fatal("expected flocks to be passing")
	}

	engine.Reload(testTheme())
	engine.Process(1002, map[string]float64{"activity": 1})
	if len(engine.flocks) != 0 {
		t.Fatalf("%d flocks outlived their sound", len(engine.flocks))
	}
}

func TestValidateFlock(t *testing.T) {
	if errs := Validate(flockThemeWithSamples(t)); len(errs) != 0 {
		t.Fatalf("expected a valid flock, got: %v", errs)
	}

	th := flockThemeWithSamples(t)
	th.Sounds[0].Size = &Range{Min: 0, Max: 4}
	th.Sounds[0].Pass = nil
	th.Sounds[0].CallRate = 0
	th.Sounds[0].Spread = true
	errs := Validate(th)
	for _, want := range []string{"invalid size range", "flock has no pass", "flock has no call_rate", "spread doesn't apply to flocks"} {
		if !containsMsg(errs, want) {
			t.Errorf("expected %q, got: %v", want, errs)
		}
	}

	th = testTheme()
	th.Sounds[0].CallRate = 2
	if errs := Validate(th); !containsMsg(errs, "only apply to flocks") {
		t.Fatalf("expected call_rate on a probabilistic sound to be rejected, got: %v", errs)
	}
}

func flockThemeWithSamples(t *testing.T) Theme {
	th := flockTheme()
	th.Sounds[0].SampleGroup = "../../themes/forest/samples/birds"
	return th
}
