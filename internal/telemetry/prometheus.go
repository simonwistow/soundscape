package telemetry

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// readPrometheus reads the Prometheus text exposition format, or
// OpenMetrics (the same, with timestamps in seconds and a closing # EOF):
//
//	# TYPE http_requests_total counter
//	http_requests_total{code="200"} 1027 1395066363000
//
// Every sample needs a timestamp. Counters, and histograms' and summaries'
// _sum, _count and _bucket series, are replayed as per-second rates; a
// counter's _created series, a time rather than a measure, is skipped.
func readPrometheus(r io.Reader) ([]sample, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)

	types := make(map[string]string)
	var samples []sample
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		if text[0] == '#' {
			fields := strings.Fields(text)
			if len(fields) >= 2 && fields[1] == "EOF" {
				break
			}
			if len(fields) >= 4 && fields[1] == "TYPE" {
				types[fields[2]] = fields[3]
			}
			continue
		}

		s, err := parsePrometheusLine(text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		kind := sampleKind(s.name, types)
		if kind == "skip" || math.IsNaN(s.value) {
			continue
		}
		s.counter = kind == "counter"
		samples = append(samples, s)
	}
	return samples, sc.Err()
}

// sampleKind says how to treat a sample named name, given the # TYPE lines
// so far: "counter", "gauge" or "skip".
func sampleKind(name string, types map[string]string) string {
	if t, ok := types[name]; ok {
		if t == "counter" {
			return "counter"
		}
		return "gauge" // a gauge, untyped, or a summary's quantiles
	}
	for _, suffix := range []string{"_total", "_created", "_sum", "_count", "_bucket"} {
		base, ok := strings.CutSuffix(name, suffix)
		if !ok {
			continue
		}
		switch types[base] {
		case "counter":
			if suffix == "_created" {
				return "skip"
			}
			return "counter"
		case "histogram", "summary":
			if suffix == "_created" {
				return "skip"
			}
			return "counter"
		}
	}
	return "gauge"
}

// parsePrometheusLine reads name{labels} value timestamp.
func parsePrometheusLine(text string) (sample, error) {
	// OpenMetrics exemplars follow " # ".
	if i := strings.Index(text, " # "); i >= 0 {
		text = text[:i]
	}

	end := strings.IndexAny(text, "{ \t")
	if end <= 0 {
		return sample{}, fmt.Errorf("want name{labels} value timestamp")
	}
	name := text[:end]
	rest := text[end:]

	labels := make(map[string]string)
	if rest[0] == '{' {
		var err error
		labels, rest, err = parseLabels(rest[1:])
		if err != nil {
			return sample{}, err
		}
	}

	fields := strings.Fields(rest)
	switch len(fields) {
	case 2:
	case 1:
		return sample{}, fmt.Errorf("no timestamp; replaying needs one on every sample")
	default:
		return sample{}, fmt.Errorf("want name{labels} value timestamp")
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return sample{}, fmt.Errorf("value %q isn't a number", fields[0])
	}
	t, err := parseTime(fields[1])
	if err != nil {
		return sample{}, err
	}
	return sample{t: t, series: seriesKey(name, labels), name: name, value: value}, nil
}

// parseLabels reads label="value",... up to the closing brace, and returns
// what follows it.
func parseLabels(s string) (map[string]string, string, error) {
	labels := make(map[string]string)
	for {
		s = strings.TrimLeft(s, " \t,")
		if s == "" {
			return nil, "", fmt.Errorf("unterminated labels")
		}
		if s[0] == '}' {
			return labels, s[1:], nil
		}
		eq := strings.IndexByte(s, '=')
		if eq <= 0 || eq+1 >= len(s) || s[eq+1] != '"' {
			return nil, "", fmt.Errorf("labels should be name=\"value\"")
		}
		key := strings.TrimSpace(s[:eq])
		s = s[eq+2:]

		var value strings.Builder
		closed := false
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					value.WriteByte('\n')
				default:
					value.WriteByte(s[i])
				}
				continue
			}
			if c == '"' {
				s = s[i+1:]
				closed = true
				break
			}
			value.WriteByte(c)
		}
		if !closed {
			return nil, "", fmt.Errorf("unterminated label value")
		}
		labels[key] = value.String()
	}
}
