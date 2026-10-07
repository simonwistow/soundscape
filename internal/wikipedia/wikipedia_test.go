package wikipedia

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadEvents(t *testing.T) {
	stream := ":ok\n\n" +
		"event: message\nid: [{\"offset\":1}]\ndata: {\"a\":1}\n\n" +
		"data: first\ndata: second\n\n" +
		"data: no-trailing-blank-line"

	var got []string
	readEvents(strings.NewReader(stream), func(d string) { got = append(got, d) })

	want := []string{`{"a":1}`, "first\nsecond"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("readEvents = %q, want %q", got, want)
	}
}

func intp(i int) *int { return &i }

func TestCounter(t *testing.T) {
	c := newCounter(nil)

	edit := Change{Type: "edit", Wiki: "enwiki", Minor: true}
	edit.Length = &struct {
		Old *int `json:"old"`
		New *int `json:"new"`
	}{Old: intp(100), New: intp(150)}
	c.add(edit)

	shrink := Change{Type: "edit", Wiki: "enwiki", Bot: true}
	shrink.Length = &struct {
		Old *int `json:"old"`
		New *int `json:"new"`
	}{Old: intp(100), New: intp(70)}
	c.add(shrink)

	created := Change{Type: "new", Wiki: "dewiki"}
	created.Length = &struct {
		Old *int `json:"old"`
		New *int `json:"new"`
	}{New: intp(40)}
	c.add(created)

	c.add(Change{Type: "log", LogType: "delete"})
	c.add(Change{Type: "categorize"})

	canary := Change{Type: "edit"}
	canary.Meta.Domain = "canary"
	c.add(canary)

	got := c.flush(2) // two seconds' worth -> halve everything
	want := map[string]float64{
		"events":        2.5,
		"edits":         1,
		"new_pages":     0.5,
		"human_edits":   1,
		"bot_edits":     0.5,
		"minor_edits":   0.5,
		"bytes_added":   45, // (50 + 40) / 2
		"bytes_removed": 15,
		"bytes_changed": 60,
		"log_events":    0.5,
		"log_delete":    0.5,
		"categorize":    0.5,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("unexpected extra metrics: %v", got)
	}

	if again := c.flush(1); len(again) != 0 {
		t.Errorf("flush should reset counts, got %v", again)
	}
}

func TestCounterWikiFilter(t *testing.T) {
	c := newCounter([]string{"enwiki"})
	c.add(Change{Type: "edit", Wiki: "enwiki"})
	c.add(Change{Type: "edit", Wiki: "wikidatawiki"})
	if got := c.flush(1)["edits"]; got != 1 {
		t.Fatalf("edits = %v, want only the enwiki one", got)
	}
}

func TestSourceStreamsIntoTicks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "soundscape") {
			t.Errorf("User-Agent = %q, want a descriptive one", ua)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 5; i++ {
			fmt.Fprint(w, "data: {\"type\":\"edit\",\"wiki\":\"enwiki\"}\n\n")
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rates := make(chan float64, 16)
	go Source{URL: srv.URL}.Run(ctx, func(_ int64, m map[string]float64) {
		select {
		case rates <- m["edits"]:
		default:
		}
	})

	// Counts are emitted as per-second rates over a window that's never
	// exactly one second, so allow a little slack.
	total := 0.0
	for total < 4.9 {
		select {
		case r := <-rates:
			total += r
		case <-ctx.Done():
			t.Fatalf("saw %v edits before timing out, want 5", total)
		}
	}
}
