// Package wikipedia is a metrics source fed by Wikimedia's public
// recent-changes stream (https://stream.wikimedia.org), a server-sent-events
// feed of every edit, page creation and log action across Wikipedia and its
// sister projects - about 30 events a second, no account needed.
//
// Events are counted into one-second windows and emitted as per-second
// rates:
//
//	events          everything (after the wiki filter)
//	edits           edits to existing pages
//	new_pages       page creations
//	categorize      category membership changes
//	log_events      log actions (deletions, uploads, blocks, new users, ...)
//	log_<type>      log actions by type, e.g. log_delete, log_upload, log_block
//	bot_edits       edits + new pages by bots
//	human_edits     edits + new pages by everyone else
//	minor_edits     edits flagged minor
//	bytes_changed   total size change of edits + new pages, either direction
//	bytes_added     ... growth only
//	bytes_removed   ... shrinkage only
package wikipedia

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/simonwistow/soundscape/internal/source"
)

const StreamURL = "https://stream.wikimedia.org/v2/stream/recentchange"

// Wikimedia asks API clients for a descriptive User-Agent.
const userAgent = "soundscape (https://github.com/simonwistow/soundscape)"

// Change is the subset of a recentchange event this source uses.
type Change struct {
	Type    string `json:"type"`
	Wiki    string `json:"wiki"`
	Bot     bool   `json:"bot"`
	Minor   bool   `json:"minor"`
	LogType string `json:"log_type"`
	Length  *struct {
		Old *int `json:"old"`
		New *int `json:"new"`
	} `json:"length"`
	Meta struct {
		Domain string `json:"domain"`
	} `json:"meta"`
}

type Source struct {
	URL string
	// Wikis restricts counting to these wiki IDs (e.g. "enwiki",
	// "dewiki", "wikidatawiki"); empty means every wiki.
	Wikis   []string
	HTTP    *http.Client
	Verbose bool
}

func (s Source) Run(ctx context.Context, emit source.Emit) error {
	url := s.URL
	if url == "" {
		url = StreamURL
	}
	client := s.HTTP
	if client == nil {
		// No overall timeout: the response is an endless stream.
		client = &http.Client{}
	}

	c := newCounter(s.Wikis)

	// Read the stream in the background, reconnecting with a backoff when
	// it drops. A reconnect deliberately starts from "now" rather than
	// resuming via Last-Event-ID: replaying the missed backlog would land
	// in one window as a burst of activity that never happened.
	go func() {
		backoff := time.Second
		for ctx.Err() == nil {
			start := time.Now()
			err := stream(ctx, client, url, c.add)
			if ctx.Err() != nil {
				return
			}
			if time.Since(start) > time.Minute {
				backoff = time.Second
			}
			log.Printf("wikipedia: stream ended (%v); reconnecting in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, 30*time.Second)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			metrics := c.flush(now.Sub(last).Seconds())
			last = now
			if s.Verbose {
				log.Printf("wikipedia: %v", metrics)
			}
			emit(now.Unix(), metrics)
		}
	}
}

// stream reads server-sent events from url, calling handle for each
// decoded change, until the connection ends or ctx is cancelled.
func stream(ctx context.Context, client *http.Client, url string, handle func(Change)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	return readEvents(resp.Body, func(data string) {
		var ch Change
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return // not every event need be a change; skip what doesn't parse
		}
		handle(ch)
	})
}

// readEvents parses an SSE stream, calling handle with each event's data
// (multiple data: lines joined by newlines, per the SSE spec).
func readEvents(r io.Reader, handle func(data string)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var data []string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if len(data) > 0 {
				handle(strings.Join(data, "\n"))
				data = data[:0]
			}
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		// event:, id:, retry: and :comments are ignored.
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// counter accumulates changes between ticks.
type counter struct {
	wikis map[string]bool

	mu     sync.Mutex
	counts map[string]float64
}

func newCounter(wikis []string) *counter {
	c := &counter{counts: make(map[string]float64)}
	if len(wikis) > 0 {
		c.wikis = make(map[string]bool, len(wikis))
		for _, w := range wikis {
			c.wikis[w] = true
		}
	}
	return c
}

func (c *counter) add(ch Change) {
	// Wikimedia injects synthetic "canary" events to monitor the stream.
	if ch.Meta.Domain == "canary" {
		return
	}
	if c.wikis != nil && !c.wikis[ch.Wiki] {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts["events"]++
	switch ch.Type {
	case "edit", "new":
		if ch.Type == "edit" {
			c.counts["edits"]++
		} else {
			c.counts["new_pages"]++
		}
		if ch.Bot {
			c.counts["bot_edits"]++
		} else {
			c.counts["human_edits"]++
		}
		if ch.Minor {
			c.counts["minor_edits"]++
		}
		if ch.Length != nil && ch.Length.New != nil {
			old := 0
			if ch.Length.Old != nil {
				old = *ch.Length.Old
			}
			if delta := *ch.Length.New - old; delta >= 0 {
				c.counts["bytes_added"] += float64(delta)
				c.counts["bytes_changed"] += float64(delta)
			} else {
				c.counts["bytes_removed"] += float64(-delta)
				c.counts["bytes_changed"] += float64(-delta)
			}
		}
	case "categorize":
		c.counts["categorize"]++
	case "log":
		c.counts["log_events"]++
		if ch.LogType != "" {
			c.counts["log_"+ch.LogType]++
		}
	}
}

// flush returns the counts accumulated since the last flush as per-second
// rates over elapsed seconds, and resets them.
func (c *counter) flush(elapsed float64) map[string]float64 {
	if elapsed <= 0 {
		elapsed = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string]float64, len(c.counts))
	for k, v := range c.counts {
		out[k] = v / elapsed
	}
	c.counts = make(map[string]float64)
	return out
}
