package fastly

import (
	"encoding/json"
	"testing"
)

// Fastly's real-time analytics API mixes plain numeric fields with some
// fields aggregated as nested objects (observed in production: decoding
// straight into map[string]float64 fails with "cannot unmarshal object
// into Go struct field Record.Data.aggregated of type float64").
func TestRecordMetricsSkipsNonNumericFields(t *testing.T) {
	raw := []byte(`{
		"recorded": 1000,
		"aggregated": {
			"requests": 42,
			"resp_body_bytes": 12345.6,
			"errors": 0,
			"some_breakdown": {"edge": 1, "shield": 2},
			"some_list": [1, 2, 3],
			"some_string": "unexpected"
		}
	}`)

	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal Record: %v", err)
	}

	metrics := rec.Metrics()

	want := map[string]float64{
		"requests":        42,
		"resp_body_bytes": 12345.6,
		"errors":          0,
	}
	for k, v := range want {
		if got, ok := metrics[k]; !ok || got != v {
			t.Errorf("metrics[%q] = %v, %v; want %v, true", k, got, ok, v)
		}
	}

	for _, k := range []string{"some_breakdown", "some_list", "some_string"} {
		if _, ok := metrics[k]; ok {
			t.Errorf("expected non-numeric field %q to be skipped, but it was present", k)
		}
	}
}

func TestRecordMetricsEmptyAggregated(t *testing.T) {
	var rec Record
	if metrics := rec.Metrics(); len(metrics) != 0 {
		t.Fatalf("expected empty metrics for zero-value Record, got %v", metrics)
	}
}
