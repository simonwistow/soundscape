package telemetry

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// readInflux reads InfluxDB line protocol:
//
//	measurement[,tag=value...] field=value[,field=value...] timestamp
//
// Each numeric or boolean field is a series named measurement.field, with
// the tags as its labels; string fields are ignored. Timestamps are
// required, in any precision (see parseTime).
func readInflux(r io.Reader) ([]sample, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var samples []sample
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || text[0] == '#' {
			continue
		}
		parsed, err := parseInfluxLine(text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		samples = append(samples, parsed...)
	}
	return samples, sc.Err()
}

func parseInfluxLine(text string) ([]sample, error) {
	sections := splitUnescaped(text, ' ', true)
	if len(sections) != 3 {
		if len(sections) == 2 {
			return nil, fmt.Errorf("no timestamp; replaying needs one on every line")
		}
		return nil, fmt.Errorf("want measurement[,tags] fields timestamp")
	}

	key := splitUnescaped(sections[0], ',', false)
	measurement := unescape(key[0])
	if measurement == "" {
		return nil, fmt.Errorf("no measurement")
	}
	tags := make(map[string]string)
	for _, tag := range key[1:] {
		kv := splitUnescaped(tag, '=', false)
		if len(kv) != 2 {
			return nil, fmt.Errorf("tag %q isn't key=value", tag)
		}
		tags[unescape(kv[0])] = unescape(kv[1])
	}

	t, err := parseTime(sections[2])
	if err != nil {
		return nil, err
	}

	var samples []sample
	for _, field := range splitUnescaped(sections[1], ',', true) {
		k, v, ok := cutUnescaped(field, '=')
		if !ok {
			return nil, fmt.Errorf("field %q isn't key=value", field)
		}
		value, ok, err := influxValue(v)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", unescape(k), err)
		}
		if !ok {
			continue // a string
		}
		name := measurement + "." + unescape(k)
		samples = append(samples, sample{t: t, series: seriesKey(name, tags), name: name, value: value})
	}
	if len(samples) == 0 && !strings.Contains(sections[1], "=") {
		return nil, fmt.Errorf("no fields")
	}
	return samples, nil
}

// influxValue reads a field value: a float, an integer (12i), an unsigned
// integer (12u) or a boolean. ok is false for a string, which isn't a
// metric.
func influxValue(v string) (value float64, ok bool, err error) {
	switch {
	case v == "":
		return 0, false, fmt.Errorf("no value")
	case v[0] == '"':
		return 0, false, nil
	}
	switch v {
	case "t", "T", "true", "True", "TRUE":
		return 1, true, nil
	case "f", "F", "false", "False", "FALSE":
		return 0, true, nil
	}
	if last := v[len(v)-1]; last == 'i' || last == 'u' {
		n, err := strconv.ParseInt(v[:len(v)-1], 10, 64)
		if err != nil && last == 'u' {
			var u uint64
			u, err = strconv.ParseUint(v[:len(v)-1], 10, 64)
			return float64(u), err == nil, err
		}
		return float64(n), err == nil, err
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil, err
}

// splitUnescaped splits s at each sep that isn't escaped with a backslash
// or, with quotes, inside a double-quoted string.
func splitUnescaped(s string, sep byte, quotes bool) []string {
	var parts []string
	start, inQuote := 0, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\':
			i++
		case quotes && c == '"':
			inQuote = !inQuote
		case c == sep && !inQuote:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// cutUnescaped is strings.Cut at the first unescaped sep.
func cutUnescaped(s string, sep byte) (before, after string, found bool) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case sep:
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// unescape removes the backslashes from escaped commas, spaces and equals
// signs in a name.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\,`, `,`, `\ `, ` `, `\=`, `=`, `\\`, `\`).Replace(s)
}
