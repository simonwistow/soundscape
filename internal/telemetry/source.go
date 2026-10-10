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
	Loop bool
	// Speed is how many seconds of the recording each tick of the
	// soundscape covers: 60 plays an hour in a minute, 0.5 plays at half
	// speed. 0 means 1. Faster, a tick averages the seconds it covers;
	// slower, a second lasts several ticks. Either way the soundscape keeps
	// its own pace, a tick a second.
	Speed   float64
	Verbose bool

	player *player
	offset int64 // added to every timestamp, for loops

	started bool
	tick    int64 // ticks emitted so far
	first   int64 // the first second of data
	base    int64 // the first tick's timestamp
	pending *dataTick
	last    map[string]float64 // the previous tick's metrics, for slow speeds
	lastAt  int64              // the last data second used, from the first
}

type dataTick struct {
	ts      int64
	metrics map[string]float64
}

// next returns the recording's next second with any data, looping if it
// should.
func (s *Source) next() (*dataTick, bool) {
	if s.pending != nil {
		d := s.pending
		s.pending = nil
		return d, true
	}
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
		return nil, false
	}
	return &dataTick{ts + s.offset, metrics}, true
}

// Step returns the soundscape's next tick: the seconds of the recording
// it covers, averaged, or the last tick again if it covers none (playing
// slowly). A gap in the recording is skipped, as at normal speed.
func (s *Source) Step() (int64, map[string]float64, bool) {
	speed := s.Speed
	if speed <= 0 {
		speed = 1
	}

	d, ok := s.next()
	if !ok {
		// Playing slowly, the last second still has its ticks to run.
		if !s.started || float64(s.tick)*speed >= float64(s.lastAt+1) {
			return 0, nil, false
		}
		return s.emit(s.last)
	}
	if !s.started {
		s.started, s.first, s.base = true, d.ts, d.ts
	}
	// This tick covers data seconds from..to (to excluded), counted from
	// the first.
	from := float64(s.tick) * speed
	to := from + speed
	at := float64(d.ts - s.first)
	if at >= to+1 {
		// A gap: start this tick at the next data instead.
		s.tick = int64(at / speed)
		from = float64(s.tick) * speed
		to = from + speed
	}

	var metrics map[string]float64
	if at >= to {
		// Nothing new in this tick (playing slowly): the last again.
		s.pending = d
		metrics = s.last
	} else {
		sums := make(map[string]float64)
		n := 0
		for ; ok && float64(d.ts-s.first) < to; d, ok = s.next() {
			for k, v := range d.metrics {
				sums[k] += v
			}
			n++
			s.lastAt = d.ts - s.first
		}
		if ok {
			s.pending = d
		}
		metrics = make(map[string]float64, len(sums))
		for k, v := range sums {
			metrics[k] = v / float64(n)
		}
		s.last = metrics
	}
	return s.emit(metrics)
}

func (s *Source) emit(metrics map[string]float64) (int64, map[string]float64, bool) {
	ts := s.base + s.tick
	s.tick++
	if s.Verbose {
		log.Printf("file: %v", metrics)
	}
	return ts, metrics, true
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
