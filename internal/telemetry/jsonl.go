package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// readJSONL reads one JSON object per line: a timestamp (key timestamp,
// time or ts; a number or an RFC 3339 string) and numbers for the metrics.
// Nested objects are flattened with dots; booleans count as 1 and 0; other
// values (strings, arrays, null) are ignored.
func readJSONL(r io.Reader) ([]sample, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var samples []sample
	for line := 1; sc.Scan(); line++ {
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(text))
		dec.UseNumber()
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}

		var t float64
		found := false
		for _, key := range timeColumns {
			raw, ok := obj[key]
			if !ok {
				continue
			}
			ts, err := parseTime(fmt.Sprint(raw))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			t, found = ts, true
			delete(obj, key)
			break
		}
		if !found {
			return nil, fmt.Errorf("line %d: no timestamp (want a key named %s)", line, strings.Join(timeColumns, ", "))
		}

		flatten("", obj, func(name string, v float64) {
			samples = append(samples, sample{t: t, series: name, name: name, value: v})
		})
	}
	return samples, sc.Err()
}

func flatten(prefix string, obj map[string]any, emit func(string, float64)) {
	for k, v := range obj {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		switch v := v.(type) {
		case json.Number:
			if f, err := v.Float64(); err == nil {
				emit(name, f)
			}
		case bool:
			if v {
				emit(name, 1)
			} else {
				emit(name, 0)
			}
		case map[string]any:
			flatten(name, v, emit)
		}
	}
}
