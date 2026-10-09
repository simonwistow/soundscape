package telemetry

import (
	"context"
	"log"
	"time"

	"github.com/simonwistow/soundscape/internal/source"
)

// Source replays a recording, a tick a second. It's a source.Stepper, so a
// run with --duration can render it faster than real time.
type Source struct {
	Recording *Recording
	// Loop starts the recording again from the top when it ends, its
	// timestamps carrying on from where they left off.
	Loop    bool
	Verbose bool

	player *player
	offset int64 // added to every timestamp, for loops
}

// Step returns the recording's next second with any data.
func (s *Source) Step() (int64, map[string]float64, bool) {
	if s.player == nil {
		s.player = newPlayer(s.Recording)
	}
	ts, metrics, ok := s.player.step()
	if !ok && s.Loop {
		s.offset += s.Recording.End() - s.Recording.Start() + 1
		s.player = newPlayer(s.Recording)
		ts, metrics, ok = s.player.step()
	}
	if !ok {
		return 0, nil, false
	}
	if s.Verbose {
		log.Printf("file: %v", metrics)
	}
	return ts + s.offset, metrics, true
}

// Run replays the recording in real time: each tick comes as long after
// the first as it was recorded. Without Loop, it returns a second after
// the last.
func (s *Source) Run(ctx context.Context, emit source.Emit) error {
	start := time.Now()
	var first int64
	stepped := false
	for {
		ts, metrics, ok := s.Step()
		if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return nil
			}
		}
		if !stepped {
			first, stepped = ts, true
		}

		due := start.Add(time.Duration(ts-first) * time.Second)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Until(due)):
		}
		emit(ts, metrics)
	}
}

// Ends reports whether the replay will end: whether it doesn't loop.
func (s *Source) Ends() bool { return !s.Loop }

var (
	_ source.Stepper = (*Source)(nil)
	_ source.Ender   = (*Source)(nil)
)
