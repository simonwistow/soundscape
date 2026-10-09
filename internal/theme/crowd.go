package theme

import (
	"math"
	"time"

	"github.com/simonwistow/soundscape/internal/event"
	"github.com/simonwistow/soundscape/internal/metrics"
	"github.com/simonwistow/soundscape/internal/scheduler"
)

// A crowd sound is a population of people who come and go, rather than a
// stream of unrelated calls:
//
//   - size is how many people are there, from size.min when its input is 0
//     to size.max when it's 1. People drift in and out over a few seconds
//     (crowdTurnover) as the input moves, rather than appearing at once;
//   - call_rate is how many times a second each person calls;
//   - rate, if set, is how many outbursts a second the crowd has: moments
//     when everyone calls at once, all together and loud, then tails off.
//
// Each person keeps their own voice (pitch, from pitch_jitter, for
// samples, or note for notes), their own place in the stereo field and
// their own distance, so some are always louder than others.
//
// MIDI has no per-note pan, so note crowds are only spread by distance.

const (
	// crowdTurnover is the time constant, in seconds, of people arriving
	// and leaving as the crowd's size follows its input.
	crowdTurnover = 5.0

	// An outburst lasts crowdBurst seconds, during which each person calls
	// up to crowdBurstCalls times, most of them early on.
	crowdBurst      = 2.0
	crowdBurstCalls = 3

	// crowdNearest and crowdFarthest bound how loud a person is, as a
	// fraction of the sound's velocity, depending on how far away they are.
	crowdNearest  = 1.0
	crowdFarthest = 0.35
)

type crowd struct {
	people []person
}

type person struct {
	pitch float64 // samples: playback ratio
	note  int     // notes: the one they call on
	pan   float64
	gain  float64 // how near they are, crowdFarthest..crowdNearest
}

// processCrowd lets people arrive or leave, so the crowd's size follows
// its input, then schedules their calls, and any outbursts, between now
// and the next tick.
func (e *Engine) processCrowd(sound Sound, value, dt float64) {
	if sound.Size == nil {
		return
	}
	if sound.Output != "sample" && len(sound.Notes) == 0 {
		return
	}

	c := e.crowds[sound.Name]
	if c == nil {
		c = &crowd{}
		e.crowds[sound.Name] = c
	}

	// Each missing person arrives, and each person too many leaves, with the
	// chance an exponential drift towards the target would give them.
	target := int(math.Round(metrics.Lerp(sound.Size.Min, sound.Size.Max, value)))
	p := 1 - math.Exp(-dt/crowdTurnover)
	for i := len(c.people); i < target; i++ {
		if e.rng.Float64() < p {
			c.people = append(c.people, e.newPerson(sound))
		}
	}
	for i := len(c.people); i > target; i-- {
		if e.rng.Float64() < p {
			gone := e.rng.Intn(len(c.people))
			c.people = append(c.people[:gone], c.people[gone+1:]...)
		}
	}

	velocity := 0.8
	if sound.Output != "sample" {
		velocity = 80
	}
	if sound.Velocity != nil {
		velocity = metrics.Lerp(sound.Velocity.Min, sound.Velocity.Max, value)
	}

	for _, who := range c.people {
		for n := scheduler.PoissonCount(e.rng, sound.CallRate*dt); n > 0; n-- {
			e.crowdCall(sound, who, velocity*who.gain, e.rng.Float64()*dt)
		}
	}

	if sound.Rate == nil {
		return
	}
	// An outburst is at the top of the velocity range, however quiet the
	// crowd otherwise is: it's the moment everyone joins in.
	loud := velocity
	if sound.Velocity != nil {
		loud = sound.Velocity.Max
	}
	rate := metrics.Lerp(sound.Rate.Min, sound.Rate.Max, value)
	for n := scheduler.PoissonCount(e.rng, rate*dt); n > 0; n-- {
		start := e.rng.Float64() * dt
		for _, who := range c.people {
			for i := 1 + e.rng.Intn(crowdBurstCalls); i > 0; i-- {
				// Squaring bunches the calls towards the start, so an outburst
				// breaks out together and trails away.
				into := math.Pow(e.rng.Float64(), 2)
				e.crowdCall(sound, who, loud*math.Sqrt(who.gain)*(1-0.6*into), start+into*crowdBurst)
			}
		}
	}
}

func (e *Engine) newPerson(sound Sound) person {
	who := person{
		pitch: 1,
		pan:   e.rng.Float64()*2 - 1,
		gain:  metrics.Lerp(crowdFarthest, crowdNearest, e.rng.Float64()),
	}
	if sound.PitchJitter != nil {
		who.pitch = metrics.Lerp(sound.PitchJitter.Min, sound.PitchJitter.Max, e.rng.Float64())
	}
	if len(sound.Notes) > 0 {
		who.note = sound.Notes[e.rng.Intn(len(sound.Notes))]
	}
	return who
}

// crowdCall schedules one call from who, delay seconds from now.
func (e *Engine) crowdCall(sound Sound, who person, velocity, delay float64) {
	duration := sound.DurationMs
	if duration <= 0 {
		duration = 300
	}

	var ev event.Event
	if sound.Output == "sample" {
		ev = event.Sample{
			Actor:      sound.Name,
			Channel:    sound.Channel,
			Group:      sound.SampleGroup,
			Pitch:      who.pitch * (1 + (e.rng.Float64()*2-1)*callWobble),
			Velocity:   velocity,
			Pan:        who.pan,
			DurationMs: duration,
		}
	} else {
		ev = event.Note{
			Actor:      sound.Name,
			Channel:    sound.Channel,
			Pitch:      who.note,
			Velocity:   int(math.Round(velocity)),
			DurationMs: duration,
		}
	}
	e.after(time.Duration(delay*float64(time.Second)), func() { _ = e.output.Send(ev) })
}

// forgetCrowds drops the crowds of sounds a reload has taken away; a crowd
// whose sound is still there carries on with its new settings.
func (e *Engine) forgetCrowds() {
	for name := range e.crowds {
		gone := true
		for _, s := range e.theme.Sounds {
			if s.Name == name && s.Type == "crowd" {
				gone = false
				break
			}
		}
		if gone {
			delete(e.crowds, name)
		}
	}
}
