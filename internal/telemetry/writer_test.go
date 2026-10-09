package telemetry

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriterRoundTrips(t *testing.T) {
	ticksIn := []struct {
		ts      int64
		metrics map[string]float64
	}{
		{1696000000, map[string]float64{"requests": 4500.25, `errors{code="500"}`: 3, "bytes": 1.5e9}},
		{1696000001, map[string]float64{"requests": 4600, `errors{code="500"}`: 0.1, "bytes": 1.6e9}},
	}
	for _, ext := range []string{".jsonl", ".csv"} {
		path := filepath.Join(t.TempDir(), "rec"+ext)
		w, err := CreateWriter(path, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, tick := range ticksIn {
			if err := w.Write(tick.ts, tick.metrics); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}

		got := ticks(t, path, "")
		for _, tick := range ticksIn {
			if !reflect.DeepEqual(got[tick.ts], tick.metrics) {
				t.Errorf("%s: tick %d read back as %v, want %v", ext, tick.ts, got[tick.ts], tick.metrics)
			}
		}
	}
}

// A source like Wikipedia's leaves out what didn't happen; live, that
// reads as 0, and a recording must replay it as 0, not hold the last value.
func TestWriterRecordsMissingMetricsAsZero(t *testing.T) {
	for _, ext := range []string{".jsonl", ".csv"} {
		path := filepath.Join(t.TempDir(), "rec"+ext)
		w, err := CreateWriter(path, "")
		if err != nil {
			t.Fatal(err)
		}
		w.Write(1696000000, map[string]float64{})
		w.Write(1696000001, map[string]float64{"edits": 3})
		w.Write(1696000002, map[string]float64{"new_pages": 1, "bad": math.NaN()})
		w.Write(1696000003, map[string]float64{})
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}

		got := ticks(t, path, "")
		want := map[int64]map[string]float64{
			1696000001: {"edits": 3, "new_pages": 0, "bad": 0},
			1696000002: {"edits": 0, "new_pages": 1, "bad": 0},
			1696000003: {"edits": 0, "new_pages": 0, "bad": 0},
		}
		if ext == ".jsonl" {
			// JSON Lines doesn't go back: before a metric turns up, it's
			// simply absent, which a replay also reads as 0.
			want[1696000001] = map[string]float64{"edits": 3}
		} else {
			// A CSV's new columns are filled with 0s, the empty first tick
			// included.
			want[1696000000] = map[string]float64{"edits": 0, "new_pages": 0, "bad": 0}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: read back %v\nwant %v", ext, got, want)
		}
	}
}

func TestCSVWriterAddsColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.csv")
	w, err := CreateWriter(path, "")
	if err != nil {
		t.Fatal(err)
	}
	w.Write(1000, map[string]float64{})
	w.Write(1001, map[string]float64{"b": 2})
	w.Write(1002, map[string]float64{"a": 1, "b": 3})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	b, _ := os.ReadFile(path)
	if want := "timestamp,a,b\n1000,0,0\n1001,0,2\n1002,1,3\n"; string(b) != want {
		t.Errorf("wrote %q, want %q", b, want)
	}
}

func TestJSONLWriterFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rec.jsonl")
	w, _ := CreateWriter(path, "")
	w.Write(1696000000, map[string]float64{"b": 2, "a": 0.5, `x{k="v"}`: 1})
	w.Write(1696000001, map[string]float64{"a": 1})
	w.Close()
	b, _ := os.ReadFile(path)
	want := `{"timestamp":1696000000,"a":0.5,"b":2,"x{k=\"v\"}":1}` + "\n" +
		`{"timestamp":1696000001,"a":1,"b":0,"x{k=\"v\"}":0}` + "\n"
	if string(b) != want {
		t.Errorf("wrote\n%s\nwant\n%s", b, want)
	}
}

func TestCheckWriter(t *testing.T) {
	for _, tc := range []struct{ path, format, want string }{
		{"a.jsonl", "", ""},
		{"a.ndjson", "", ""},
		{"a.csv", "", ""},
		{"a.log", "jsonl", ""},
		{"a.log", "", "can't tell the format"},
		{"a.prom", "", "not prometheus"},
		{"a.csv", "influx", "not influx"},
	} {
		err := CheckWriter(tc.path, tc.format)
		if tc.want == "" {
			if err != nil {
				t.Errorf("%s %s: %v", tc.path, tc.format, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s %s: %v, want %q", tc.path, tc.format, err, tc.want)
		}
	}
}
