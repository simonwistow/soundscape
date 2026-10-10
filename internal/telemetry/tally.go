package telemetry

import "math"

// tally counts events - requests in a log, packets in a capture - into
// per-second metrics as a file is read, so memory goes with the seconds
// covered rather than the events.
type tally struct {
	sums     map[int64]map[string]float64
	means    map[int64]map[string]*mean
	distinct map[int64]map[string]map[string]bool
}

type mean struct {
	sum, max float64
	n        int
}

func newTally() *tally {
	return &tally{
		sums:     make(map[int64]map[string]float64),
		means:    make(map[int64]map[string]*mean),
		distinct: make(map[int64]map[string]map[string]bool),
	}
}

// add adds v to name's total for the second t falls in.
func (t *tally) add(at float64, name string, v float64) {
	sec := int64(math.Floor(at))
	m := t.sums[sec]
	if m == nil {
		m = make(map[string]float64)
		t.sums[sec] = m
	}
	m[name] += v
}

// observe records a measurement, such as a response time: the second gets
// name, the mean, and name_max.
func (t *tally) observe(at float64, name string, v float64) {
	sec := int64(math.Floor(at))
	m := t.means[sec]
	if m == nil {
		m = make(map[string]*mean)
		t.means[sec] = m
	}
	mn := m[name]
	if mn == nil {
		mn = &mean{max: v}
		m[name] = mn
	}
	mn.sum += v
	mn.max = max(mn.max, v)
	mn.n++
}

// count counts key once a second under name: distinct clients, say.
func (t *tally) count(at float64, name, key string) {
	sec := int64(math.Floor(at))
	m := t.distinct[sec]
	if m == nil {
		m = make(map[string]map[string]bool)
		t.distinct[sec] = m
	}
	if m[name] == nil {
		m[name] = make(map[string]bool)
	}
	m[name][key] = true
}

// samples returns every second's metrics.
func (t *tally) samples() []sample {
	var out []sample
	emit := func(sec int64, name string, v float64) {
		out = append(out, sample{t: float64(sec), series: name, name: name, value: v})
	}
	for sec, m := range t.sums {
		for name, v := range m {
			emit(sec, name, v)
		}
	}
	for sec, m := range t.means {
		for name, mn := range m {
			emit(sec, name, mn.sum/float64(mn.n))
			emit(sec, name+"_max", mn.max)
		}
	}
	for sec, m := range t.distinct {
		for name, keys := range m {
			emit(sec, name, float64(len(keys)))
		}
	}
	return out
}
