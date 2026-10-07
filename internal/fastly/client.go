package fastly

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/simonwistow/soundscape/internal/source"
)

type Client struct {
	Token     string
	ServiceID string
	BaseURL   string
	HTTP      *http.Client
}

type Response struct {
	Data           []Record `json:"Data"`
	Timestamp      int64    `json:"Timestamp"`
	AggregateDelay int64    `json:"AggregateDelay"`
}

type Record struct {
	Recorded int64 `json:"recorded"`
	// Aggregated is decoded as raw JSON rather than map[string]float64
	// because Fastly's real-time analytics API mixes plain numeric metrics
	// with some fields aggregated as nested objects (e.g. per-shield
	// breakdowns); unmarshaling straight into map[string]float64 fails
	// outright the moment any single value isn't a number. Use Metrics()
	// to get the numeric fields.
	Aggregated map[string]json.RawMessage `json:"aggregated"`
}

// Metrics returns the record's numeric aggregated fields, silently
// skipping any value that isn't a plain JSON number (objects, arrays,
// strings). This is the counterpart to Aggregated's raw decoding above.
func (r Record) Metrics() map[string]float64 {
	out := make(map[string]float64, len(r.Aggregated))
	for k, raw := range r.Aggregated {
		var v float64
		if err := json.Unmarshal(raw, &v); err == nil {
			out[k] = v
		}
	}
	return out
}

func NewClient(token, serviceID string) *Client {
	return &Client{
		Token:     token,
		ServiceID: serviceID,
		BaseURL:   "https://rt.fastly.com",
		HTTP:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) Fetch(timestamp int64) (Response, error) {
	url := fmt.Sprintf("%s/v1/channel/%s/ts/%s",
		c.BaseURL, c.ServiceID, strconv.FormatInt(timestamp, 10))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Response{}, err
	}

	req.Header.Set("Fastly-Key", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("Fastly returned HTTP %s", resp.Status)
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Response{}, err
	}

	return result, nil
}

// Source adapts Client to source.Source: it follows the real-time API's
// Timestamp cursor and emits each one-second record as a tick.
type Source struct {
	Client  *Client
	Verbose bool
}

func (s Source) Run(ctx context.Context, emit source.Emit) error {
	var timestamp int64
	for ctx.Err() == nil {
		resp, err := s.Client.Fetch(timestamp)
		if err != nil {
			log.Printf("fastly: %v", err)
			sleep(ctx, time.Second)
			continue
		}

		if s.Verbose {
			log.Printf("fastly: timestamp=%d records=%d delay=%ds",
				resp.Timestamp, len(resp.Data), resp.AggregateDelay)
		}

		for _, record := range resp.Data {
			emit(record.Recorded, record.Metrics())
		}

		timestamp = resp.Timestamp
		sleep(ctx, 200*time.Millisecond)
	}
	return ctx.Err()
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
