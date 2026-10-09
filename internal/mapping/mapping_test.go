package mapping

import (
	"os"
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
		{"metric", Mapping{Inputs: map[string]Input{"activity": {Metric: "requests"}}}, ""},
		{"query", Mapping{Inputs: map[string]Input{"activity": {Query: "up"}}}, ""},
		{"no inputs", Mapping{}, "no inputs"},
		{"neither", Mapping{Inputs: map[string]Input{"a": {}}}, "no metric"},
		{"both", Mapping{Inputs: map[string]Input{"a": {Metric: "x", Query: "up"}}}, "both a metric and a query"},
		{"inverted range", Mapping{Inputs: map[string]Input{"a": {Metric: "x", Normalise: &Range{Min: 5, Max: 1}}}}, "invalid normalise range"},
	} {
		expect(t, tc.name, Validate(tc.m), tc.want)
	}
}

func TestCheckSource(t *testing.T) {
	metrics := Mapping{Inputs: map[string]Input{"a": {Metric: "requests"}}}
	queries := Mapping{Inputs: map[string]Input{"a": {Query: "up"}}}
	for _, tc := range []struct {
		name string
		m    Mapping
		kind string
		want string
	}{
		{"fastly metrics", metrics, "fastly", ""},
		{"fastly queries", queries, "fastly", "only for the prometheus source"},
		{"prometheus queries", queries, "prometheus", ""},
		{"prometheus metrics", metrics, "prometheus", "needs a query"},
		{"file metrics", metrics, "file", ""},
		// A recording of a Prometheus run, under its inputs' names.
		{"file queries", queries, "file", ""},
	} {
		expect(t, tc.name, CheckSource(tc.m, tc.kind), tc.want)
	}
}

func expect(t *testing.T, name string, errs []error, want string) {
	t.Helper()
	if want == "" {
		if len(errs) != 0 {
			t.Errorf("%s: unexpected errors %v", name, errs)
		}
		return
	}
	for _, e := range errs {
		if strings.Contains(e.Error(), want) {
			return
		}
	}
	t.Errorf("%s: expected an error containing %q, got %v", name, want, errs)
}

func TestLoadRejectsASource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.yaml")
	os.WriteFile(path, []byte("source: fastly\ninputs:\n  a: { metric: x }\n"), 0o644)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "pick the source with --source") {
		t.Errorf("Load of a mapping with a source: %v", err)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "typo.yaml")
	os.WriteFile(path, []byte("inputs:\n  a: { metric: x, smoothnig: 3 }\n"), 0o644)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "smoothnig") {
		t.Errorf("Load of a misspelt key: %v", err)
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
	c := NewConditioner(Mapping{Inputs: map[string]Input{
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
	c := NewConditioner(Mapping{Inputs: map[string]Input{
		"activity": {Query: "sum(rate(x[1m]))"},
	}})
	if got := c.Apply(map[string]float64{"activity": 42})["activity"]; got != 42 {
		t.Fatalf("activity = %v, want 42", got)
	}
}

func TestConditionerReloadPreservesSmoothing(t *testing.T) {
	m := Mapping{Inputs: map[string]Input{"slow": {Metric: "x", Smoothing: 10}}}
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
