// Package prometheus is a metrics source that polls a Prometheus server's
// instant-query API. A mapping gives each input a PromQL `query` (see
// mappings/prometheus.yaml), and results are emitted keyed by input name.
//
// Each expression should reduce to a single number. A vector result with
// several series is summed, so an un-aggregated query still yields
// something, but aggregating explicitly in PromQL is clearer.
package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/simonwistow/soundscape/internal/source"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

type queryResponse struct {
	Status    string `json:"status"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
	Data      struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
}

// Query evaluates expr at the current time and reduces the result to one
// number: a scalar as-is, a vector's samples summed. An empty vector is
// 0 (e.g. a rate over a counter that has no series yet). NaN and ±Inf
// samples are skipped rather than poisoning the sum.
func (c *Client) Query(ctx context.Context, expr string) (float64, error) {
	u := c.BaseURL + "/api/v1/query?" + url.Values{"query": {expr}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Prometheus returns a JSON error body on 400/422/503 too, so try to
	// decode before falling back to the bare HTTP status.
	var qr queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		if resp.StatusCode != http.StatusOK {
			return 0, fmt.Errorf("Prometheus returned HTTP %s", resp.Status)
		}
		return 0, err
	}
	if qr.Status != "success" {
		return 0, fmt.Errorf("Prometheus %s: %s", qr.ErrorType, qr.Error)
	}

	switch qr.Data.ResultType {
	case "scalar":
		var sample [2]json.RawMessage
		if err := json.Unmarshal(qr.Data.Result, &sample); err != nil {
			return 0, err
		}
		v, err := parseValue(sample[1])
		if err != nil {
			return 0, err
		}
		if !finite(v) {
			return 0, nil
		}
		return v, nil

	case "vector":
		var series []struct {
			Value [2]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(qr.Data.Result, &series); err != nil {
			return 0, err
		}
		var sum float64
		for _, s := range series {
			v, err := parseValue(s.Value[1])
			if err != nil {
				return 0, err
			}
			if finite(v) {
				sum += v
			}
		}
		return sum, nil

	default:
		return 0, fmt.Errorf("unsupported result type %q (use an expression that returns an instant vector or scalar)", qr.Data.ResultType)
	}
}

// parseValue decodes a Prometheus sample value, which is sent as a JSON
// string ("1.5", "NaN", "+Inf") to preserve special values.
func parseValue(raw json.RawMessage) (float64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("sample value: %w", err)
	}
	return strconv.ParseFloat(s, 64)
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// Source evaluates every query once per Interval and emits the results as
// one tick. A query that fails keeps its last good value, so a transient
// error holds the soundscape steady instead of dropping that metric to
// zero.
type Source struct {
	Client   *Client
	Queries  map[string]string
	Interval time.Duration
	Verbose  bool
}

func (s Source) Run(ctx context.Context, emit source.Emit) error {
	interval := s.Interval
	if interval <= 0 {
		interval = time.Second
	}

	names := make([]string, 0, len(s.Queries))
	for name := range s.Queries {
		names = append(names, name)
	}
	sort.Strings(names)

	last := make(map[string]float64, len(names))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		for _, name := range names {
			v, err := s.Client.Query(ctx, s.Queries[name])
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("prometheus: %s: %v", name, err)
				continue
			}
			last[name] = v
		}

		metrics := make(map[string]float64, len(last))
		for k, v := range last {
			metrics[k] = v
		}
		if s.Verbose {
			log.Printf("prometheus: %v", metrics)
		}
		emit(time.Now().Unix(), metrics)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
