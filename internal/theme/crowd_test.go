package theme

import (
	"testing"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
)

func crowdTheme() Theme {
	return Theme{
		Name: "test",
		Sounds: []Sound{{
			Name:        "shoppers",
			Type:        "crowd",
			Output:      "sample",
			Input:       "activity",
			Size:        &Range{Min: 2, Max: 20},
			CallRate:    0.5,
			Velocity:    &Range{Min: 0.4, Max: 0.8},
			SampleGroup: "chatter",
			PitchJitter: &Range{Min: 0.8, Max: 1.2},
		}},
	}
}

func TestCrowdDriftsTowardsItsSize(t *testing.T) {
	engine := NewEngineWithSeed(crowdTheme(), &recordingOutput{}, 1)
	engine.after = func(time.Duration, func()) {}
	people := func() int { return len(engine.crowds["shoppers"].people) }

	ts := int64(1000)
	tick := func(value float64) {
		engine.Process(ts, map[string]float64{"activity": value})
		ts++
	}

	tick(1)
	if n := people(); n == 0 || n >= 15 {
		t.Fatalf("%d people after the first second; want some, but not most, of 20 to have arrived", n)
	}
	for i := 0; i < 60; i++ {
		tick(1)
	}
	if n := people(); n != 20 {
		t.Fatalf("%d people after a minute busy, want 20", n)
	}
	for i := 0; i < 60; i++ {
		tick(0)
	}
	if n := people(); n != 2 {
		t.Fatalf("%d people after a minute quiet, want 2", n)
	}
}

func TestCrowdPeopleKeepTheirVoicesAndPlaces(t *testing.T) {
	engine := NewEngineWithSeed(crowdTheme(), &recordingOutput{}, 2)
	var calls []event.Sample
	engine.after = func(d time.Duration, f func()) {
		if d < 0 || d >= time.Second {
			t.Fatalf("call delayed %v, beyond the one-second tick", d)
		}
		out := &recordingOutput{}
		engine.output = out
		f()
		calls = append(calls, out.events[0].(event.Sample))
	}
	// Two people, who are there from the start.
	engine.crowds["shoppers"] = &crowd{people: []person{
		{pitch: 0.9, pan: -0.5, gain: 1},
		{pitch: 1.1, pan: 0.5, gain: 0.5},
	}}
	th := crowdTheme()
	th.Sounds[0].Size = &Range{Min: 2, Max: 2}
	engine.Reload(th)

	for i := 0; i < 60; i++ {
		engine.Process(int64(1000+i), map[string]float64{"activity": 1})
	}
	if len(calls) < 30 {
		t.Fatalf("got %d calls, want about 60 from 2 people calling every other second for a minute", len(calls))
	}
	for _, c := range calls {
		switch c.Pan {
		case -0.5:
			if c.Velocity != 0.8 || c.Pitch < 0.9*(1-callWobble) || c.Pitch > 0.9*(1+callWobble) {
				t.Fatalf("the near person called as %+v", c)
			}
		case 0.5:
			if c.Velocity != 0.4 || c.Pitch < 1.1*(1-callWobble) || c.Pitch > 1.1*(1+callWobble) {
				t.Fatalf("the far person called as %+v", c)
			}
		default:
			t.Fatalf("a call from pan %v, where nobody is", c.Pan)
		}
	}
}

func TestCrowdOutburstsBringEveryoneIn(t *testing.T) {
	th := crowdTheme()
	th.Sounds[0].Size = &Range{Min: 10, Max: 10}
	th.Sounds[0].CallRate = 1e-9 // all but silent, apart from outbursts
	th.Sounds[0].Rate = &Rate{Min: 0.05, Max: 0.05}
	engine := NewEngineWithSeed(th, &recordingOutput{}, 3)
	engine.crowds["shoppers"] = &crowd{}
	for i := 0; i < 10; i++ {
		engine.crowds["shoppers"].people = append(engine.crowds["shoppers"].people, engine.newPerson(th.Sounds[0]))
	}

	// With people all but silent otherwise, a tick's calls are an outburst
	// (at this rate, one at a time).
	var bursts [][]time.Duration
	var tick []time.Duration
	engine.after = func(d time.Duration, f func()) { tick = append(tick, d) }
	for i := 0; i < 200; i++ {
		tick = nil
		engine.Process(int64(1000+i), map[string]float64{"activity": 1})
		if len(tick) > 0 {
			bursts = append(bursts, tick)
		}
	}

	if len(bursts) == 0 {
		t.Fatal("expected outbursts")
	}
	for _, b := range bursts {
		if len(b) < 10 || len(b) > 10*crowdBurstCalls {
			t.Fatalf("an outburst had %d calls, want each of 10 people to call 1 to %d times", len(b), crowdBurstCalls)
		}
		lo, hi := b[0], b[0]
		for _, d := range b {
			lo, hi = min(lo, d), max(hi, d)
		}
		if hi-lo > crowdBurst*time.Second {
			t.Fatalf("an outburst lasted %v, want at most %vs", hi-lo, crowdBurst)
		}
	}
}

func TestCrowdsGoWhenTheirSoundIsReloadedAway(t *testing.T) {
	engine := NewEngineWithSeed(crowdTheme(), &recordingOutput{}, 1)
	engine.after = func(time.Duration, func()) {}
	engine.Process(1000, map[string]float64{"activity": 1})
	if len(engine.crowds) == 0 {
		t.Fatal("expected a crowd")
	}
	engine.Reload(testTheme())
	if len(engine.crowds) != 0 {
		t.Fatal("the crowd outlived its sound")
	}
}

func TestValidateCrowd(t *testing.T) {
	th := crowdTheme()
	th.Sounds[0].SampleGroup = "../../themes/market/samples/chatter"
	if errs := Validate(th); len(errs) != 0 {
		t.Fatalf("expected a valid crowd, got: %v", errs)
	}

	th.Sounds[0].Size = &Range{Min: 0, Max: 0}
	th.Sounds[0].CallRate = 0
	th.Sounds[0].Pass = &Range{Min: 1, Max: 2}
	errs := Validate(th)
	for _, want := range []string{"invalid size range", "crowd has no call_rate", "pass only applies to flock sounds"} {
		if !containsMsg(errs, want) {
			t.Errorf("expected %q, got: %v", want, errs)
		}
	}
}
