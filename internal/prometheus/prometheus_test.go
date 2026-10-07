package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakePrometheus serves canned /api/v1/query responses keyed by the query
// expression.
func fakePrometheus(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			http.NotFound(w, r)
			return
		}
		body, ok := responses[r.URL.Query().Get("query")]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			body = `{"status":"error","errorType":"bad_data","error":"unknown query"}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestQuery(t *testing.T) {
	srv := fakePrometheus(t, map[string]string{
		"scalar": `{"status":"success","data":{"resultType":"scalar","result":[1700000000.1,"2.5"]}}`,
		"vector": `{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{"instance":"a"},"value":[1700000000,"1.5"]},
			{"metric":{"instance":"b"},"value":[1700000000,"3"]},
			{"metric":{"instance":"c"},"value":[1700000000,"NaN"]}
		]}}`,
		"empty":  `{"status":"success","data":{"resultType":"vector","result":[]}}`,
		"matrix": `{"status":"success","data":{"resultType":"matrix","result":[]}}`,
	})
	c := NewClient(srv.URL + "/")
	ctx := context.Background()

	for _, tc := range []struct {
		expr string
		want float64
	}{
		{"scalar", 2.5},
		{"vector", 4.5}, // summed, NaN skipped
		{"empty", 0},
	} {
		got, err := c.Query(ctx, tc.expr)
		if err != nil {
			t.Errorf("Query(%q): %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Query(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}

	for _, expr := range []string{"matrix", "bogus"} {
		if _, err := c.Query(ctx, expr); err == nil {
			t.Errorf("Query(%q): expected an error", expr)
		}
	}
}

func TestSourceKeepsLastGoodValueOnFailure(t *testing.T) {
	var broken atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := "7"
		if r.URL.Query().Get("query") == "flaky" {
			if broken.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"status":"error","errorType":"unavailable","error":"down"}`))
				return
			}
			value = "3"
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"scalar","result":[0,"` + value + `"]}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := Source{
		Client:   NewClient(srv.URL),
		Queries:  map[string]string{"a": "steady", "b": "flaky"},
		Interval: 5 * time.Millisecond,
	}

	ticks := make(chan map[string]float64)
	go s.Run(ctx, func(_ int64, m map[string]float64) {
		select {
		case ticks <- m:
		case <-ctx.Done():
		}
	})

	if first := <-ticks; first["a"] != 7 || first["b"] != 3 {
		t.Fatalf("first tick = %v, want a=7 b=3", first)
	}

	broken.Store(true)
	<-ticks // may have been evaluated before the break
	if got := <-ticks; got["a"] != 7 || got["b"] != 3 {
		t.Errorf("tick after failure = %v, want b to keep its last good value 3", got)
	}
}
