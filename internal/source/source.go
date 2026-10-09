// Package source defines where telemetry comes from. The theme engine only
// ever wants a timestamped map of named numbers per tick
// (theme.Engine.Process), so a Source is anything that can produce those:
// the Fastly real-time API, a Prometheus server, a public event feed, or
// the built-in simulation.
package source

import (
	"context"
	"math"
	"time"
)

// Emit receives one tick of metrics. timestamp is Unix seconds.
type Emit func(timestamp int64, metrics map[string]float64)

// Source produces metrics until ctx is cancelled. Transient failures (a
// dropped connection, a failed query) should be logged and retried inside
// Run rather than returned, so a long-running soundscape doesn't stop on a
// network blip; Run returns an error only when it can't continue at all.
type Source interface {
	Run(ctx context.Context, emit Emit) error
}

// Stepper is a Source that can also produce its ticks on demand, as fast
// as they're asked for, so a soundscape can be rendered faster than real
// time. Live feeds can't; a simulation or a recording can.
type Stepper interface {
	Source
	// Step returns the next tick; ok is false once there are no more.
	Step() (timestamp int64, metrics map[string]float64, ok bool)
}

// Ender is a Stepper that reports whether it will run out, as a recording
// that doesn't loop will. One that will can be rendered to its end without
// a --duration.
type Ender interface {
	Ends() bool
}

// Simulation generates deterministic synthetic telemetry on a deliberately
// slow quiet -> busy -> quiet cycle, so a theme can be developed without
// any real data source. Its metric names and scales match Fastly's;
// mappings/simulate.yaml binds them to theme inputs.
type Simulation struct {
	t    float64
	next int64 // the next Step's timestamp; 0 until the first
}

func (s *Simulation) Run(ctx context.Context, emit Emit) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			emit(now.Unix(), s.metrics())
		}
	}
}

// Step returns the simulation's next second, starting from now; it never
// runs out.
func (s *Simulation) Step() (int64, map[string]float64, bool) {
	if s.next == 0 {
		s.next = time.Now().Unix()
	}
	ts := s.next
	s.next++
	return ts, s.metrics(), true
}

// metrics returns the current point in the cycle, and moves on.
func (s *Simulation) metrics() map[string]float64 {
	t := s.t
	s.t += 0.12

	requests := 50.0 + (1+math.Sin(t))*4500.0
	bandwidth := requests * (1000 + 8000*(1+math.Sin(t*0.7))/2)
	errors := 1.0 + 10.0*(1+math.Sin(t*1.7))/2
	status4xx := 4.0 * math.Max(0, math.Sin(t*0.45))

	return map[string]float64{
		"requests":        requests,
		"resp_body_bytes": bandwidth,
		"errors":          errors,
		"hits":            requests * 0.85,
		"all_status_4xx":  status4xx,
	}
}
