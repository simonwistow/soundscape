// Package telemetry reads and writes recorded telemetry: a source's raw
// metrics over time, so a period can be replayed through a mapping, in real
// time or rendered faster than that.
//
// It reads four formats: CSV, JSON Lines, InfluxDB line protocol and
// Prometheus/OpenMetrics text. Whatever the format, a file becomes a list
// of samples - a time, a series and a value - which are replayed a second
// at a time. A series keeps its last value for up to five minutes, as in
// Prometheus, so data scraped every 15 seconds still plays a tick a second,
// while a longer gap is skipped rather than filled.
//
// Series are named:
//   - in CSV and JSON Lines, as the column or key says (nested JSON objects
//     joined with dots: {"cpu": {"user": 1}} is cpu.user);
//   - in line protocol, measurement.field;
//   - in Prometheus text, by the metric name.
//
// A labelled series (Prometheus labels, line protocol tags) is available
// both exactly, as name{a="1",b="2"} with the labels sorted, and as plain
// name, summed across all of that name's series. Prometheus counters, and
// histograms' and summaries' _sum, _count and _bucket, become per-second
// rates, as PromQL's rate() would make them.
package telemetry

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Formats are the names format= takes.
var Formats = []string{"csv", "jsonl", "influx", "prometheus"}

// staleness is how long a series keeps its last value without a new one.
const staleness = 5 * 60

// extensions maps file extensions to formats.
var extensions = map[string]string{
	".csv":     "csv",
	".jsonl":   "jsonl",
	".ndjson":  "jsonl",
	".json":    "jsonl",
	".lp":      "influx",
	".influx":  "influx",
	".line":    "influx",
	".prom":    "prometheus",
	".om":      "prometheus",
	".metrics": "prometheus",
}

// FormatOf returns the format path's extension implies, or "" if it
// doesn't imply one.
func FormatOf(path string) string {
	return extensions[strings.ToLower(filepath.Ext(path))]
}

// sample is one value of one series at one time.
type sample struct {
	t      float64 // Unix seconds
	series string  // name, or name{labels}
	name   string  // the series' name without labels
	value  float64
	// counter values only go up (apart from resets) and are replayed as
	// their rate of change.
	counter bool
}

// Read parses the file at path in format ("" to go by its extension).
func Read(path, format string) (*Recording, error) {
	if format == "" {
		format = FormatOf(path)
		if format == "" {
			return nil, fmt.Errorf("%s: can't tell the format from the extension; add format=%s", path, strings.Join(Formats, "|"))
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var samples []sample
	switch format {
	case "csv":
		samples, err = readCSV(f)
	case "jsonl":
		samples, err = readJSONL(f)
	case "influx":
		samples, err = readInflux(f)
	case "prometheus":
		samples, err = readPrometheus(f)
	default:
		return nil, fmt.Errorf("%s: unknown format %q (want %s)", path, format, strings.Join(Formats, ", "))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r := newRecording(samples)
	if len(r.samples) == 0 {
		// (A counter needs two samples to make a rate.)
		return nil, fmt.Errorf("%s: no samples to play", path)
	}
	return r, nil
}

// newRecording turns counters into rates and sorts everything by time.
func newRecording(samples []sample) *Recording {
	slices.SortStableFunc(samples, func(a, b sample) int { return cmp.Compare(a.t, b.t) })

	type last struct{ t, v float64 }
	previous := make(map[string]last)
	out := samples[:0]
	for _, s := range samples {
		if !s.counter {
			out = append(out, s)
			continue
		}
		p, seen := previous[s.series]
		previous[s.series] = last{s.t, s.value}
		if !seen || s.t <= p.t {
			continue
		}
		increase := s.value - p.v
		if increase < 0 { // a reset: it counted up from zero since
			increase = s.value
		}
		s.value = increase / (s.t - p.t)
		out = append(out, s)
	}

	series := make(map[string]bool)
	for _, s := range out {
		series[s.series] = true
	}
	return &Recording{samples: out, series: len(series)}
}

// Recording is a file's samples, replayed a second at a time by Step.
type Recording struct {
	samples []sample
	series  int
}

// Start and End are the first and last seconds with samples.
func (r *Recording) Start() int64 { return int64(math.Floor(r.samples[0].t)) }
func (r *Recording) End() int64   { return int64(math.Floor(r.samples[len(r.samples)-1].t)) }

// Series is how many distinct series the recording holds.
func (r *Recording) Series() int { return r.series }

// Describe says what's in the recording, for the log.
func (r *Recording) Describe() string {
	start, end := time.Unix(r.Start(), 0).UTC(), time.Unix(r.End(), 0).UTC()
	return fmt.Sprintf("%d series, %v from %s to %s", r.series,
		end.Sub(start)+time.Second, start.Format(time.DateTime), end.Format(time.DateTime)+" UTC")
}

// player steps through a recording, holding each series' latest value.
type player struct {
	r       *Recording
	next    int   // the next sample to apply
	now     int64 // the next second to emit
	current map[string]held
	names   map[string]string // series -> name, for labelled series
}

type held struct {
	value float64
	at    int64
}

func newPlayer(r *Recording) *player {
	return &player{r: r, now: r.Start(), current: make(map[string]held), names: make(map[string]string)}
}

// step returns the next second that has any live series, or false at the
// end.
func (p *player) step() (int64, map[string]float64, bool) {
	for p.now <= p.r.End() {
		sec := p.now
		p.now++
		for p.next < len(p.r.samples) && int64(math.Floor(p.r.samples[p.next].t)) <= sec {
			s := p.r.samples[p.next]
			p.current[s.series] = held{s.value, sec}
			if s.series != s.name {
				p.names[s.series] = s.name
			}
			p.next++
		}

		metrics := make(map[string]float64, len(p.current))
		sums := make(map[string]float64)
		for series, h := range p.current {
			if sec-h.at > staleness {
				delete(p.current, series)
				continue
			}
			metrics[series] = h.value
			if name, ok := p.names[series]; ok {
				sums[name] += h.value
			}
		}
		if len(metrics) == 0 {
			// A gap: jump to the next sample rather than step through it.
			if p.next < len(p.r.samples) {
				p.now = int64(math.Floor(p.r.samples[p.next].t))
			}
			continue
		}
		for name, v := range sums {
			metrics[name] = v
		}
		return sec, metrics, true
	}
	return 0, nil, false
}

// parseTime reads a timestamp: a number of seconds, milliseconds,
// microseconds or nanoseconds since the Unix epoch, told apart by size (any
// time from 1973 to 5138 is unambiguous), or an RFC 3339 date.
func parseTime(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		switch a := math.Abs(n); {
		case a < 1e11:
			return n, nil
		case a < 1e14:
			return n / 1e3, nil
		case a < 1e17:
			return n / 1e6, nil
		default:
			return n / 1e9, nil
		}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, fmt.Errorf("timestamp %q is neither a Unix time nor an RFC 3339 date", s)
	}
	return float64(t.UnixNano()) / 1e9, nil
}

// seriesKey names a labelled series: name{a="1",b="2"}, labels sorted and
// values escaped as in Prometheus.
func seriesKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(labels[k]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}
