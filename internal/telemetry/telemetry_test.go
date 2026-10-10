package telemetry

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// write puts content in a temporary file called name and returns its path.
func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ticks reads a file and plays every tick.
func ticks(t *testing.T, path, format string) map[int64]map[string]float64 {
	t.Helper()
	r, err := Read(path, format)
	if err != nil {
		t.Fatal(err)
	}
	s := &Source{Recording: r}
	out := make(map[int64]map[string]float64)
	for {
		ts, m, ok := s.Step()
		if !ok {
			return out
		}
		out[ts] = m
	}
}

func TestParseTime(t *testing.T) {
	want := 1696000000.5
	for _, in := range []string{"1696000000.5", "1696000000500", "1696000000500000", "1696000000500000000", "2023-09-29T15:06:40.5Z"} {
		got, err := parseTime(in)
		if err != nil {
			t.Errorf("parseTime(%q): %v", in, err)
			continue
		}
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("parseTime(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := parseTime("yesterday"); err == nil {
		t.Error("parseTime(yesterday): no error")
	}
}

func TestCSV(t *testing.T) {
	// The time column needn't come first, and can mix units and forms.
	path := write(t, "a.csv", `requests,time,errors
# a comment
10,1696000000,1
20,2023-09-29T15:06:41Z,
30,1696000002000,3
`)
	got := ticks(t, path, "")
	want := map[int64]map[string]float64{
		1696000000: {"requests": 10, "errors": 1},
		1696000001: {"requests": 20, "errors": 1}, // an empty cell: errors held
		1696000002: {"requests": 30, "errors": 3},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestCSVErrors(t *testing.T) {
	for _, tc := range []struct{ content, want string }{
		{"time,a\nsoon,1\n", "line 2: timestamp \"soon\""},
		{"time,a\n1000,lots\n", `line 2: a: "lots" isn't a number`},
	} {
		_, err := Read(write(t, "a.csv", tc.content), "")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: error %v, want %q", tc.content, err, tc.want)
		}
	}
}

func TestJSONL(t *testing.T) {
	path := write(t, "a.jsonl", `{"ts": 1696000000, "requests": 5, "cache": {"hit": 4, "miss": 1}, "up": true, "host": "a"}

{"timestamp": "1696000001000000000", "requests": 7}
`)
	got := ticks(t, path, "")
	want := map[int64]map[string]float64{
		1696000000: {"requests": 5, "cache.hit": 4, "cache.miss": 1, "up": 1},
		1696000001: {"requests": 7, "cache.hit": 4, "cache.miss": 1, "up": 1}, // held
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestInflux(t *testing.T) {
	path := write(t, "a.lp", `# a comment
fastly,service=a requests=10i,hit\ ratio=0.9,status="ok, really",healthy=t 1696000000000000000
fastly,service=b requests=30u 1696000000000000000
my\,odd\ measurement,tag\=key=v\ 1 value=1.5 1696000000
`)
	got := ticks(t, path, "")
	want := map[string]float64{
		`fastly.requests{service="a"}`:            10,
		`fastly.requests{service="b"}`:            30,
		`fastly.requests`:                         40,
		`fastly.hit ratio{service="a"}`:           0.9,
		`fastly.hit ratio`:                        0.9,
		`fastly.healthy{service="a"}`:             1,
		`fastly.healthy`:                          1,
		`my,odd measurement.value{tag=key="v 1"}`: 1.5,
		`my,odd measurement.value`:                1.5,
	}
	if !reflect.DeepEqual(got[1696000000], want) {
		t.Errorf("got %v\nwant %v", got[1696000000], want)
	}

	_, err := Read(write(t, "b.lp", "m value=1\n"), "")
	if err == nil || !strings.Contains(err.Error(), "no timestamp") {
		t.Errorf("a line with no timestamp: %v", err)
	}
}

func TestPrometheusCountersBecomeRates(t *testing.T) {
	path := write(t, "a.prom", `# HELP http_requests_total Requests.
# TYPE http_requests_total counter
http_requests_total{code="200"} 100 1696000000000
http_requests_total{code="200"} 130 1696000010000
http_requests_total{code="200"} 20 1696000020000
http_requests_total{code="500"} 0 1696000000000
http_requests_total{code="500"} 10 1696000010000
# TYPE temperature gauge
temperature{room="a \"big\" one"} 21.5 1696000005000
`)
	got := ticks(t, path, "")
	const T = 1696000000

	// No rate until a counter's second sample.
	if _, ok := got[T][`http_requests_total{code="200"}`]; ok {
		t.Errorf("a rate at the first sample: %v", got[T])
	}
	// 30 more in 10 s, held until the next sample.
	if v := got[T+12][`http_requests_total{code="200"}`]; v != 3 {
		t.Errorf("200s at 1012 = %v, want 3/s", v)
	}
	if v := got[T+12][`http_requests_total`]; v != 4 {
		t.Errorf("all requests at 1012 = %v, want 4/s (3 + 1)", v)
	}
	// A reset to 20: it counted 20 from zero in 10 s.
	if v := got[T+20][`http_requests_total{code="200"}`]; v != 2 {
		t.Errorf("after the reset = %v, want 2/s", v)
	}
	if v := got[T+5][`temperature{room="a \"big\" one"}`]; v != 21.5 {
		t.Errorf("gauge with escaped label = %v (tick %v)", v, got[T+5])
	}
}

func TestOpenMetrics(t *testing.T) {
	path := write(t, "a.om", `# TYPE requests counter
requests_total 10 1696000000.0 # {trace_id="x"} 1
requests_created 900 1696000000
requests_total 20 1696000001.5
# TYPE latency histogram
latency_bucket{le="+Inf"} 4 1696000000
latency_bucket{le="+Inf"} 8 1696000002
latency_sum 1.0 1696000000
latency_sum 3.0 1696000002
# EOF
ignored 1 1696000000
`)
	got := ticks(t, path, "")
	const T = 1696000000
	if v := got[T+1]["requests_total"]; math.Abs(v-10/1.5) > 1e-9 {
		t.Errorf("requests_total rate = %v, want %v", v, 10/1.5)
	}
	if _, ok := got[T+1]["requests_created"]; ok {
		t.Error("_created was replayed")
	}
	if v := got[T+2][`latency_bucket{le="+Inf"}`]; v != 2 {
		t.Errorf("bucket rate = %v, want 2/s", v)
	}
	if v := got[T+2]["latency_sum"]; v != 1 {
		t.Errorf("sum rate = %v, want 1/s", v)
	}
	if _, ok := got[T+1]["ignored"]; ok {
		t.Error("read past # EOF")
	}
}

func TestPrometheusNeedsTimestamps(t *testing.T) {
	_, err := Read(write(t, "a.prom", "up 1\n"), "")
	if err == nil || !strings.Contains(err.Error(), "no timestamp") {
		t.Errorf("error = %v", err)
	}
}

func TestScrapesAreHeldForATickASecond(t *testing.T) {
	// Every 15 s, then a gap of ten minutes.
	path := write(t, "a.csv", "time,up\n1000,1\n1015,2\n1630,3\n")
	got := ticks(t, path, "")

	for sec := int64(1000); sec <= 1315; sec++ {
		want := 1.0
		if sec >= 1015 {
			want = 2
		}
		if got[sec]["up"] != want {
			t.Fatalf("second %d: up = %v, want %v", sec, got[sec]["up"], want)
		}
	}
	// Five minutes after its last value, the series goes stale, and the
	// rest of the gap is skipped rather than played as silence.
	if _, ok := got[1316]; ok {
		t.Error("a tick in the gap, after the series went stale")
	}
	if got[1630]["up"] != 3 {
		t.Errorf("after the gap: %v", got[1630])
	}
	if len(got) != 317 {
		t.Errorf("%d ticks, want 317 (1000..1315 and 1630)", len(got))
	}
}

func TestLoopCarriesTimeOn(t *testing.T) {
	r, err := Read(write(t, "a.csv", "time,n\n1000,1\n1001,2\n1002,3\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Source{Recording: r, Loop: true}
	var times []int64
	var values []float64
	for range 7 {
		ts, m, ok := s.Step()
		if !ok {
			t.Fatal("a looping source ran out")
		}
		times = append(times, ts)
		values = append(values, m["n"])
	}
	if want := []int64{1000, 1001, 1002, 1003, 1004, 1005, 1006}; !reflect.DeepEqual(times, want) {
		t.Errorf("times %v, want %v", times, want)
	}
	if want := []float64{1, 2, 3, 1, 2, 3, 1}; !reflect.DeepEqual(values, want) {
		t.Errorf("values %v, want %v", values, want)
	}
}

func TestRunReplaysInRealTime(t *testing.T) {
	r, err := Read(write(t, "a.csv", "time,n\n1000,1\n1001,2\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var at []time.Duration
	err = (&Source{Recording: r}).Run(context.Background(), func(int64, map[string]float64) {
		at = append(at, time.Since(start))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(at) != 2 || at[0] > 100*time.Millisecond || at[1] < 900*time.Millisecond || at[1] > 1200*time.Millisecond {
		t.Errorf("ticks at %v, want about 0s and 1s", at)
	}
	if took := time.Since(start); took < 1900*time.Millisecond {
		t.Errorf("returned after %v, want a second after the last tick", took)
	}
}

func TestFormatErrors(t *testing.T) {
	if _, err := Read(write(t, "a.dat", "x"), ""); err == nil || !strings.Contains(err.Error(), "add format=") {
		t.Errorf("unknown extension: %v", err)
	}
	if _, err := Read(write(t, "a.dat", "x"), "xml"); err == nil || !strings.Contains(err.Error(), `unknown format "xml"`) {
		t.Errorf("unknown format: %v", err)
	}
	if _, err := Read(write(t, "a.prom", "# TYPE c counter\nc 1 1000\n"), ""); err == nil || !strings.Contains(err.Error(), "no samples to play") {
		t.Errorf("a lone counter sample: %v", err)
	}
}

// stepN steps a source n times, or until it ends.
func stepN(t *testing.T, s *Source, n int) (times []int64, values []map[string]float64) {
	t.Helper()
	for range n {
		ts, m, ok := s.Step()
		if !ok {
			break
		}
		times = append(times, ts)
		values = append(values, m)
	}
	return times, values
}

func TestSpeedAveragesWhenFaster(t *testing.T) {
	r, err := Read(write(t, "a.csv", "time,n\n1000,1\n1001,3\n1002,5\n1003,7\n1004,9\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	times, values := stepN(t, &Source{Recording: r, Speed: 2}, 10)
	if want := []int64{1000, 1001, 1002}; !reflect.DeepEqual(times, want) {
		t.Errorf("times %v, want %v (a tick a second, two seconds of data each)", times, want)
	}
	var got []float64
	for _, m := range values {
		got = append(got, m["n"])
	}
	if want := []float64{2, 6, 9}; !reflect.DeepEqual(got, want) {
		t.Errorf("values %v, want %v (each tick's seconds averaged)", got, want)
	}
}

func TestSpeedRepeatsWhenSlower(t *testing.T) {
	r, err := Read(write(t, "a.csv", "time,n\n1000,1\n1001,2\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	times, values := stepN(t, &Source{Recording: r, Speed: 0.5}, 10)
	var got []float64
	for _, m := range values {
		got = append(got, m["n"])
	}
	if want := []float64{1, 1, 2, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("values %v, want %v (each second lasting two ticks)", got, want)
	}
	if want := []int64{1000, 1001, 1002, 1003}; !reflect.DeepEqual(times, want) {
		t.Errorf("times %v, want %v", times, want)
	}
}

func TestSpeedStillSkipsGaps(t *testing.T) {
	// Ten seconds, then nothing for an hour, then ten more.
	content := "time,n\n"
	for i := 0; i < 10; i++ {
		content += strconv.Itoa(1000+i) + ",1\n"
	}
	for i := 0; i < 10; i++ {
		content += strconv.Itoa(4600+i) + ",2\n"
	}
	r, err := Read(write(t, "a.csv", content), "")
	if err != nil {
		t.Fatal(err)
	}
	times, values := stepN(t, &Source{Recording: r, Speed: 5}, 100)
	var got []float64
	for _, m := range values {
		got = append(got, m["n"])
	}
	// The data's first ten seconds, then the hour gap skipped (with the
	// five minutes the series is held at the start of it), then the last
	// ten seconds.
	if got[0] != 1 || got[len(got)-1] != 2 {
		t.Errorf("values %v", got)
	}
	if len(times) > 2+300/5+2+1 {
		t.Errorf("%d ticks: the gap wasn't skipped (%v)", len(times), times)
	}
	for i := 1; i < len(times); i++ {
		if times[i] <= times[i-1] {
			t.Fatalf("times go backwards: %v", times)
		}
	}
}

func TestSpeedWithLoop(t *testing.T) {
	r, err := Read(write(t, "a.csv", "time,n\n1000,1\n1001,3\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	times, values := stepN(t, &Source{Recording: r, Speed: 2, Loop: true}, 3)
	if want := []int64{1000, 1001, 1002}; !reflect.DeepEqual(times, want) {
		t.Errorf("times %v, want %v", times, want)
	}
	for i, m := range values {
		if m["n"] != 2 {
			t.Errorf("tick %d = %v, want each loop averaged to 2", i, m["n"])
		}
	}
}
