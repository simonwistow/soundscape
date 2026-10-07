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

// Simulation generates deterministic synthetic telemetry on a deliberately
// slow quiet -> busy -> quiet cycle, so a theme can be developed without
// any real data source. Its metric names and scales match Fastly's;
// mappings/simulate.yaml binds them to theme inputs.
type Simulation struct{}

func (Simulation) Run(ctx context.Context, emit Emit) error {
	var t float64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			requests := 50.0 + (1+math.Sin(t))*4500.0
			bandwidth := requests * (1000 + 8000*(1+math.Sin(t*0.7))/2)
			errors := 1.0 + 10.0*(1+math.Sin(t*1.7))/2
			status4xx := 4.0 * math.Max(0, math.Sin(t*0.45))

			emit(now.Unix(), map[string]float64{
				"requests":        requests,
				"resp_body_bytes": bandwidth,
				"errors":          errors,
				"hits":            requests * 0.85,
				"all_status_4xx":  status4xx,
			})

			t += 0.12
		}
	}
}
