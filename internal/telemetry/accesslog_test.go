package telemetry

import (
	"reflect"
	"strings"
	"testing"
)

func TestAccessLogCombinedAndCommon(t *testing.T) {
	// 1696000000 is 29/Sep/2023:15:06:40 UTC.
	path := write(t, "access.log", `10.0.0.1 - - [29/Sep/2023:15:06:40 +0000] "GET / HTTP/1.1" 200 1024 "-" "curl/8"
10.0.0.2 - frank [29/Sep/2023:17:06:40 +0200] "POST /form HTTP/1.1" 302 - "https://example.com/" "Mozilla/5.0 (\"quoted\")"
10.0.0.1 - - [29/Sep/2023:15:06:40 +0000] "GET /missing HTTP/1.1" 404 512
10.0.0.3 - - [29/Sep/2023:15:06:42 +0000] "BREW /pot HTTP/1.1" 500 10 "-" "-" 0.250 extra
10.0.0.3 - - [29/Sep/2023:15:06:42 +0000] "-" 408 0 "-" "-"
`)
	got := ticks(t, path, "")

	want := map[int64]map[string]float64{
		1696000000: {"requests": 3, "bytes": 1536, "status_2xx": 1, "status_3xx": 1, "status_4xx": 1,
			"method_get": 2, "method_post": 1, "clients": 2},
		// A second with no requests is 0, not the last second held.
		1696000001: {"requests": 0, "bytes": 0, "status_2xx": 0, "status_3xx": 0, "status_4xx": 0,
			"method_get": 0, "method_post": 0, "clients": 0},
		1696000002: {"requests": 2, "bytes": 10, "status_2xx": 0, "status_3xx": 0, "status_4xx": 1, "status_5xx": 1,
			"method_get": 0, "method_post": 0, "method_other": 1, "clients": 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestAccessLogCustomFormat(t *testing.T) {
	path := write(t, "access.log", `example.com 10.0.0.1 [29/Sep/2023:15:06:40 +0000] GET 200 1024 1500
example.com 10.0.0.2 [29/Sep/2023:15:06:40 +0000] GET 200 2048 4500
`)
	r, err := ReadWith(path, Options{LogFormat: `%v %a %t %m %>s %B %D`})
	if err != nil {
		t.Fatal(err)
	}
	ts, m, _ := (&Source{Recording: r}).Step()
	want := map[string]float64{"requests": 2, "bytes": 3072, "status_2xx": 2, "method_get": 2, "clients": 2,
		"response_ms": 3, "response_ms_max": 4.5}
	if ts != 1696000000 || !reflect.DeepEqual(m, want) {
		t.Errorf("tick %d = %v, want %v", ts, m, want)
	}
}

func TestLogFormatTimeUnits(t *testing.T) {
	for _, tc := range []struct {
		format, value string
		ms            float64
	}{
		{"%T", "2", 2000},
		{"%{s}T", "2", 2000},
		{"%{ms}T", "250", 250},
		{"%{us}T", "250000", 250},
		{"%D", "250000", 250},
	} {
		path := write(t, "access.log", "[29/Sep/2023:15:06:40 +0000] 200 "+tc.value+"\n")
		r, err := ReadWith(path, Options{LogFormat: "%t %>s " + tc.format})
		if err != nil {
			t.Fatal(err)
		}
		_, m, _ := (&Source{Recording: r}).Step()
		if m["response_ms"] != tc.ms {
			t.Errorf("%s %s: response_ms = %v, want %v", tc.format, tc.value, m["response_ms"], tc.ms)
		}
	}
}

func TestLogFormatErrors(t *testing.T) {
	for _, tc := range []struct{ format, want string }{
		{`%h "%r" %>s`, "no time"},
		{`%h %t "%r"`, "no status"},
		{`%h %{%Y-%m-%d}t %>s`, "only the default time format"},
		{`%t %>s %{min}T`, "want %T"},
	} {
		_, err := compileLogFormat(tc.format)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.format, err, tc.want)
		}
	}
}

func TestAccessLogMostlyUnreadable(t *testing.T) {
	path := write(t, "error.log", `[Fri Sep 29 15:06:40.123 2023] [core:error] [pid 1] AH00126: Invalid URI
[Fri Sep 29 15:06:41.123 2023] [core:error] [pid 1] AH00126: Invalid URI
10.0.0.1 - - [29/Sep/2023:15:06:40 +0000] "GET / HTTP/1.1" 200 1024
`)
	_, err := Read(path, "")
	if err == nil || !strings.Contains(err.Error(), "2 of 3 lines don't fit") || !strings.Contains(err.Error(), "logformat=") {
		t.Errorf("error = %v", err)
	}
}

func TestLogFormatIsOnlyForLogs(t *testing.T) {
	path := write(t, "a.csv", "time,a\n1000,1\n")
	if _, err := ReadWith(path, Options{LogFormat: "%t %>s"}); err == nil || !strings.Contains(err.Error(), "only for access logs") {
		t.Errorf("error = %v", err)
	}
}
