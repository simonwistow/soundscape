package mapping

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	for in, want := range map[string]string{
		"wikipedia":         filepath.Join("mappings", "wikipedia.yaml"),
		"my.yaml":           "my.yaml",
		"./local":           "./local",
		"/abs/mapping.yaml": "/abs/mapping.yaml",
	} {
		if got := Resolve(in); got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    Mapping
		want string // substring of an expected error; "" means valid
	}{
		{"ok", Mapping{Source: "fastly", Inputs: map[string]Input{"activity": {Metric: "requests"}}}, ""},
		{"prometheus ok", Mapping{Source: "prometheus", Inputs: map[string]Input{"activity": {Query: "up"}}}, ""},
		{"source spec ok", Mapping{Source: "fastly:service=abc", Inputs: map[string]Input{"activity": {Metric: "requests"}}}, ""},
		{"prometheus spec ok", Mapping{Source: "prometheus:http://h:9090,interval=5s", Inputs: map[string]Input{"activity": {Query: "up"}}}, ""},
		{"prometheus spec needs queries", Mapping{Source: "prometheus:http://h:9090", Inputs: map[string]Input{"a": {Metric: "x"}}}, "no query"},
		{"bad source option", Mapping{Source: "fastly:region=eu", Inputs: map[string]Input{"a": {Metric: "x"}}}, `source: fastly:region=eu: unknown option "region"`},
		{"no source", Mapping{Inputs: map[string]Input{"a": {Metric: "x"}}}, "no source"},
		{"unknown source", Mapping{Source: "carrier-pigeon", Inputs: map[string]Input{"a": {Metric: "x"}}}, "unknown source"},
		{"no inputs", Mapping{Source: "simulate"}, "no inputs"},
		{"no metric", Mapping{Source: "simulate", Inputs: map[string]Input{"a": {}}}, "no metric"},
		{"query off prometheus", Mapping{Source: "fastly", Inputs: map[string]Input{"a": {Metric: "x", Query: "up"}}}, "only meaningful"},
		{"prometheus no query", Mapping{Source: "prometheus", Inputs: map[string]Input{"a": {Metric: "x"}}}, "no query"},
		{"inverted range", Mapping{Source: "simulate", Inputs: map[string]Input{"a": {Metric: "x", Normalise: &Range{Min: 5, Max: 1}}}}, "invalid normalise range"},
	} {
		errs := Validate(tc.m)
		if tc.want == "" {
			if len(errs) != 0 {
				t.Errorf("%s: unexpected errors %v", tc.name, errs)
			}
			continue
		}
		found := false
		for _, e := range errs {
			found = found || strings.Contains(e.Error(), tc.want)
		}
		if !found {
			t.Errorf("%s: expected an error containing %q, got %v", tc.name, tc.want, errs)
		}
	}
}

func TestUnbound(t *testing.T) {
	m := Mapping{Inputs: map[string]Input{"activity": {}, "flow": {}}}
	got := Unbound(m, []string{"activity", "trouble", "flow", "quirks"})
	if strings.Join(got, ",") != "trouble,quirks" {
		t.Fatalf("Unbound = %v, want [trouble quirks]", got)
	}
}

func TestConditionerNormalisesAndSmooths(t *testing.T) {
	c := NewConditioner(Mapping{Source: "simulate", Inputs: map[string]Input{
		"activity": {Metric: "requests", Normalise: &Range{Min: 0, Max: 1000}},
		"slow":     {Metric: "requests", Smoothing: 10},
	}})

	first := c.Apply(map[string]float64{"requests": 1000})
	if first["activity"] != 1 {
		t.Errorf("activity = %v, want 1 at the top of its range", first["activity"])
	}

	second := c.Apply(map[string]float64{"requests": 0})
	if second["activity"] != 0 {
		t.Errorf("activity = %v, want 0 with no smoothing", second["activity"])
	}
	if second["slow"] != 900 { // 1000 + (0-1000)/10, un-normalised
		t.Errorf("slow = %v, want 900", second["slow"])
	}
}

func TestConditionerPrometheusKeysByInputName(t *testing.T) {
	c := NewConditioner(Mapping{Source: "prometheus", Inputs: map[string]Input{
		"activity": {Query: "sum(rate(x[1m]))"},
	}})
	if got := c.Apply(map[string]float64{"activity": 42})["activity"]; got != 42 {
		t.Fatalf("activity = %v, want 42", got)
	}
}

func TestConditionerReloadPreservesSmoothing(t *testing.T) {
	m := Mapping{Source: "simulate", Inputs: map[string]Input{"slow": {Metric: "x", Smoothing: 10}}}
	c := NewConditioner(m)
	c.Apply(map[string]float64{"x": 1000})
	c.Apply(map[string]float64{"x": 0}) // 900

	c.Reload(m)
	if got := c.Apply(map[string]float64{"x": 0})["slow"]; got != 810 {
		t.Errorf("slow = %v after reload, want 810 (running value kept)", got)
	}

	// Changing the smoothing itself starts afresh with the new constant.
	m.Inputs = map[string]Input{"slow": {Metric: "x", Smoothing: 2}}
	c.Reload(m)
	if got := c.Apply(map[string]float64{"x": 0})["slow"]; got != 0 {
		t.Errorf("slow = %v after changing smoothing, want a fresh smoother (0)", got)
	}
}

// Every mapping shipped in mappings/ should load and validate.
func TestShippedMappingsValidate(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", Dir, "*.yaml"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no mappings found: %v", err)
	}
	for _, p := range paths {
		m, err := Load(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		for _, e := range Validate(m) {
			t.Errorf("%s: %v", p, e)
		}
	}
}
