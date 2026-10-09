// Package mapping binds a data source's raw metrics to a theme's abstract
// inputs. Themes only ever see named 0..1 values ("activity", "trouble",
// ...); a mapping file says which of a source's metrics feeds each input,
// and how to condition it (smoothing, normalisation), since the right
// scale depends entirely on the source - Wikipedia's ~20 edits/s and a
// CDN's 5000 req/s can't share a range. The source itself is --source's;
// it picks the mapping named after it unless --mappings picks another.
//
//	inputs:
//	  activity:
//	    metric: requests
//	    smoothing: 8
//	    normalise: { min: 1, max: 5000 }
//
// For the prometheus source an input gives a PromQL `query` instead of a
// `metric`.
package mapping

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/simonwistow/soundscape/internal/metrics"
)

// Dir is where bare mapping names (--mappings wikipedia) are looked up.
const Dir = "mappings"

type Mapping struct {
	Inputs map[string]Input `yaml:"inputs"`
}

type Input struct {
	Metric    string  `yaml:"metric"`
	Query     string  `yaml:"query"`
	Smoothing float64 `yaml:"smoothing"`
	Normalise *Range  `yaml:"normalise"`
}

type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

// Resolve turns a mapping name into a file path: anything that looks like a
// path (contains a separator or ends in .yaml/.yml) is used as-is, and a
// bare name like "wikipedia" means mappings/wikipedia.yaml.
func Resolve(name string) string {
	if strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, '/') ||
		strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		return name
	}
	return filepath.Join(Dir, name+".yaml")
}

func Load(path string) (Mapping, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Mapping{}, err
	}
	// A mapping used to name its source; now --source does.
	var keys map[string]any
	if err := yaml.Unmarshal(b, &keys); err == nil {
		if _, ok := keys["source"]; ok {
			return Mapping{}, fmt.Errorf("%s: a mapping no longer names its source; remove source:, and pick the source with --source", path)
		}
	}

	// Strictly, so a misspelt key is an error rather than ignored.
	var m Mapping
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil && !errors.Is(err, io.EOF) {
		return Mapping{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Validate reports problems with a mapping on its own; Unbound checks it
// against a theme.
func Validate(m Mapping) []error {
	var errs []error
	if len(m.Inputs) == 0 {
		errs = append(errs, fmt.Errorf("mapping defines no inputs"))
	}
	for _, name := range m.InputNames() {
		in := m.Inputs[name]
		switch hasQuery := strings.TrimSpace(in.Query) != ""; {
		case in.Metric == "" && !hasQuery:
			errs = append(errs, fmt.Errorf("%s: input has no metric (or, for prometheus, query)", name))
		case in.Metric != "" && hasQuery:
			errs = append(errs, fmt.Errorf("%s: input has both a metric and a query; give one", name))
		}
		if in.Normalise != nil && in.Normalise.Min >= in.Normalise.Max {
			errs = append(errs, fmt.Errorf("%s: invalid normalise range: min (%v) >= max (%v)",
				name, in.Normalise.Min, in.Normalise.Max))
		}
	}
	return errs
}

// CheckSource reports what keeps a mapping from working with a kind of
// source: prometheus answers PromQL queries, the others have metrics, and a
// recording may hold either (a Prometheus run's results are recorded under
// their inputs' names).
func CheckSource(m Mapping, kind string) []error {
	var errs []error
	for _, name := range m.InputNames() {
		in := m.Inputs[name]
		switch {
		case kind == "prometheus" && in.Query == "":
			errs = append(errs, fmt.Errorf("%s: the prometheus source needs a query, not a metric", name))
		case kind != "prometheus" && kind != "file" && in.Query != "":
			errs = append(errs, fmt.Errorf("%s: a query is only for the prometheus source; %s needs a metric", name, kind))
		}
	}
	return errs
}

// Unbound returns the inputs a theme uses that the mapping doesn't provide.
// Those would read as a constant 0, which is almost always a typo or a
// mismatched theme/mapping pair rather than intent.
func Unbound(m Mapping, themeInputs []string) []string {
	var missing []string
	for _, name := range themeInputs {
		if _, ok := m.Inputs[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

// InputNames returns the mapping's input names in a stable order.
func (m Mapping) InputNames() []string {
	names := make([]string, 0, len(m.Inputs))
	for name := range m.Inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Queries returns input name -> PromQL for the prometheus source, whose
// results come back keyed by input name (see Conditioner.Apply).
func (m Mapping) Queries() map[string]string {
	q := make(map[string]string, len(m.Inputs))
	for name, in := range m.Inputs {
		q[name] = in.Query
	}
	return q
}

// Conditioner turns one tick of raw source metrics into theme inputs:
// each input's metric is smoothed and normalised into 0..1. It's safe for
// concurrent Apply and Reload (the latter from hot reload).
type Conditioner struct {
	mu        sync.Mutex
	mapping   Mapping
	smoothers map[string]*metrics.Smoother
}

func NewConditioner(m Mapping) *Conditioner {
	return &Conditioner{mapping: m, smoothers: buildSmoothers(m, nil)}
}

// Reload swaps in a new mapping, keeping the running smoothed value of any
// input that's still present under the same name and smoothing.
func (c *Conditioner) Reload(m Mapping) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smoothers = buildSmoothers(m, c.smoothers)
	c.mapping = m
}

func (c *Conditioner) Apply(raw map[string]float64) map[string]float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string]float64, len(c.mapping.Inputs))
	for name, in := range c.mapping.Inputs {
		// Prometheus results, live or recorded, come keyed by input name.
		key := in.Metric
		if key == "" {
			key = name
		}
		v := c.smoothers[name].Update(raw[key])
		if in.Normalise != nil {
			v = metrics.LogNormalise(v, in.Normalise.Min, in.Normalise.Max)
		}
		out[name] = v
	}
	return out
}

func buildSmoothers(m Mapping, previous map[string]*metrics.Smoother) map[string]*metrics.Smoother {
	s := make(map[string]*metrics.Smoother, len(m.Inputs))
	for name, in := range m.Inputs {
		if existing, ok := previous[name]; ok && existing.Seconds() == effectiveSmoothing(in) {
			s[name] = existing
			continue
		}
		s[name] = metrics.NewSmoother(effectiveSmoothing(in))
	}
	return s
}

func effectiveSmoothing(in Input) float64 {
	if in.Smoothing <= 1 {
		return 1
	}
	return in.Smoothing
}
