package theme

import (
	"math"
	"slices"
	"testing"

	"github.com/simonwistow/soundscape/internal/event"
)

// stormTheme has a storm following trouble, and a cc sound that reports
// the storm's value, so tests can watch it.
func stormTheme(gusts float64) Theme {
	return Theme{
		Name:    "test",
		Weather: []Weather{{Name: "storm", Input: "trouble", Build: 10, Clear: 40, Gusts: gusts}},
		Sounds: []Sound{{
			Name:     "gauge",
			Type:     "continuous",
			Input:    "storm",
			MinValue: 0,
			MaxValue: 1,
		}},
	}
}

// stormValues plays th one second at a time with trouble at each of
// troubles, and returns the storm's value after each.
func stormValues(engine *Engine, troubles []float64) []float64 {
	out := &recordingOutput{}
	engine.output = out
	var values []float64
	for i, trouble := range troubles {
		engine.Process(int64(1000+i), map[string]float64{"trouble": trouble})
		values = append(values, out.events[len(out.events)-1].(event.Control).Value)
	}
	return values
}

func repeat(v float64, n int) []float64 {
	vs := make([]float64, n)
	for i := range vs {
		vs[i] = v
	}
	return vs
}

func TestWeatherBuildsAndClears(t *testing.T) {
	engine := NewEngineWithSeed(stormTheme(0), &recordingOutput{}, 1)

	up := stormValues(engine, repeat(1, 60))
	if want := 1 - math.Exp(-1.0/10); math.Abs(up[0]-want) > 1e-9 {
		t.Fatalf("after a second, storm = %v, want %v", up[0], want)
	}
	if want := 1 - math.Exp(-10.0/10); math.Abs(up[9]-want) > 1e-9 {
		t.Fatalf("after one build time, storm = %v, want %v", up[9], want)
	}
	if up[59] < 0.99 {
		t.Fatalf("after six build times, storm = %v, want almost 1", up[59])
	}

	down := stormValues(engine, repeat(0, 10))
	if want := up[59] * math.Exp(-10.0/40); math.Abs(down[9]-want) > 1e-9 {
		t.Fatalf("ten seconds into clearing, storm = %v, want %v (clearing slower than it built)", down[9], want)
	}
}

func TestWeatherGustsAroundItsLevel(t *testing.T) {
	engine := NewEngineWithSeed(stormTheme(0.5), &recordingOutput{}, 2)

	calm := stormValues(engine, repeat(0, 30))
	for _, v := range calm {
		if v != 0 {
			t.Fatalf("clear weather gusted to %v", v)
		}
	}

	// Build to half strength, then watch it gust about there.
	stormValues(engine, repeat(0.5, 60))
	vs := stormValues(engine, repeat(0.5, 600))
	lo, hi, total := 1.0, 0.0, 0.0
	for _, v := range vs {
		lo, hi, total = min(lo, v), max(hi, v), total+v
	}
	if lo > 0.35 || hi < 0.65 {
		t.Fatalf("storm stayed within %.2f..%.2f, want gusts well either side of 0.5", lo, hi)
	}
	if mean := total / float64(len(vs)); math.Abs(mean-0.5) > 0.1 {
		t.Fatalf("storm averaged %.2f, want about 0.5", mean)
	}
}

func TestWeatherLeavesTheCallersInputsAlone(t *testing.T) {
	engine := NewEngineWithSeed(stormTheme(0), &recordingOutput{}, 1)
	inputs := map[string]float64{"trouble": 1}
	engine.Process(1000, inputs)
	if _, ok := inputs["storm"]; ok || len(inputs) != 1 {
		t.Fatalf("Process changed its inputs: %v", inputs)
	}
}

func TestWeatherCarriesOnOverReload(t *testing.T) {
	engine := NewEngineWithSeed(stormTheme(0), &recordingOutput{}, 1)
	before := stormValues(engine, repeat(1, 10))

	engine.Reload(stormTheme(0))
	after := stormValues(engine, repeat(1, 1))
	if after[0] <= before[9] {
		t.Fatalf("storm went from %v to %v over a reload, want it to carry on building", before[9], after[0])
	}

	th := stormTheme(0)
	th.Weather = nil
	engine.Reload(th)
	if len(engine.weather) != 0 {
		t.Fatal("the storm outlived its weather")
	}
}

func TestInputsOmitWeatherAndIncludeWhatItFollows(t *testing.T) {
	if got, want := Inputs(stormTheme(0)), []string{"trouble"}; !slices.Equal(got, want) {
		t.Fatalf("Inputs = %v, want %v", got, want)
	}
}

func TestValidateWeather(t *testing.T) {
	if errs := Validate(stormTheme(0.3)); len(errs) != 0 {
		t.Fatalf("expected valid weather, got: %v", errs)
	}

	th := stormTheme(1.5)
	th.Weather = append(th.Weather,
		Weather{Name: "storm", Input: "trouble", Build: 1, Clear: 1},
		Weather{Name: "squall", Input: "storm", Build: 0, Clear: -1},
	)
	errs := Validate(th)
	for _, want := range []string{
		"invalid gusts 1.5",
		"duplicate weather name",
		`input "storm" is weather itself`,
		"build must be more than 0",
		"clear must be more than 0",
	} {
		if !containsMsg(errs, want) {
			t.Errorf("expected %q, got: %v", want, errs)
		}
	}
}
