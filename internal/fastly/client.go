package fastly

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
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
	Recorded   int64              `json:"recorded"`
	Aggregated map[string]float64 `json:"aggregated"`
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
