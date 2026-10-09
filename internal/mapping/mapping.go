// Package mapping binds a data source's raw metrics to a theme's abstract
// inputs. Themes only ever see named 0..1 values ("activity", "trouble",
// ...); a mapping file says which source to read, which of its metrics
// feeds each input, and how to condition it (smoothing, normalisation),
// since the right scale depends entirely on the source - Wikipedia's ~20
// edits/s and a CDN's 5000 req/s can't share a range.
//
//	source: fastly
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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/simonwistow/soundscape/internal/metrics"
	"github.com/simonwistow/soundscape/internal/source"
)

// Dir is where bare mapping names (--aliases wikipedia) are looked up.
const Dir = "mappings"

type Mapping struct {
	// Source is a source spec, as --source takes: wikipedia,
	// fastly:service=SID and so on.
	Source string           `yaml:"source"`
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
	var m Mapping
	if err := yaml.Unmarshal(b, &m); err != nil {
		return Mapping{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Validate reports problems with a mapping on its own; Unbound checks it
// against a theme.
func Validate(m Mapping) []error {
	var errs []error

	kind := ""
	if m.Source == "" {
		errs = append(errs, fmt.Errorf("mapping has no source (expected one of %s)", strings.Join(source.Kinds(), ", ")))
	} else if spec, err := source.ParseSpec(m.Source); err != nil {
		errs = append(errs, fmt.Errorf("source: %v", strings.TrimPrefix(err.Error(), "--source ")))
	} else {
		kind = spec.Kind
	}
	if len(m.Inputs) == 0 {
		errs = append(errs, fmt.Errorf("mapping defines no inputs"))
	}

	for _, name := range m.InputNames() {
		in := m.Inputs[name]
		switch kind {
		case "prometheus":
			if strings.TrimSpace(in.Query) == "" {
				errs = append(errs, fmt.Errorf("%s: prometheus input has no query", name))
			}
			if in.Metric != "" {
				errs = append(errs, fmt.Errorf("%s: prometheus inputs use query, not metric", name))
			}
		default:
			if in.Metric == "" {
				errs = append(errs, fmt.Errorf("%s: input has no metric", name))
			}
			if in.Query != "" {
				errs = append(errs, fmt.Errorf("%s: query is only meaningful for the prometheus source", name))
			}
		}
		if in.Normalise != nil && in.Normalise.Min >= in.Normalise.Max {
			errs = append(errs, fmt.Errorf("%s: invalid normalise range: min (%v) >= max (%v)",
				name, in.Normalise.Min, in.Normalise.Max))
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
		// Prometheus results come keyed by input name.
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
