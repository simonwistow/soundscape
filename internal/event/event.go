// Package event defines the abstract sound-event vocabulary that the theme
// engine emits. Events describe what should happen in the ecosystem (a bird
// chirps, a river's volume shifts) without knowing how any particular output
// backend will realise it — as MIDI, a WAV sample, an OSC message, or
// anything else. See PLANS.md's "Improve the event model" section.
package event

import "fmt"

// Event is the common interface for everything the theme engine can emit.
type Event interface {
	// Describe returns a short, backend-independent, human-readable summary
	// of the event (used by the console backend and for logging).
	Describe() string
}

// Note is a discrete pitched sound (a chirp, a strike, a plucked note).
// Pitch and Velocity follow the familiar 0-127 MIDI-style range because
// it's a convenient shared scale, not because the event is MIDI-specific:
// a sample-based backend can just as easily map Pitch to playback speed.
type Note struct {
	Actor      string // logical sound name from the theme, e.g. "birds"
	Channel    int    // polyphony/grouping lane
	Pitch      int    // 0-127
	Velocity   int    // 0-127
	DurationMs int
}

func (n Note) Describe() string {
	return fmt.Sprintf("NOTE actor=%s channel=%d pitch=%d velocity=%d duration=%dms",
		n.Actor, n.Channel, n.Pitch, n.Velocity, n.DurationMs)
}

// Sample requests playback of a recorded sound rather than a synthesized
// note. Group names a directory/collection of samples the output backend
// should pick from (e.g. random bird-call variation).
//
// When Loop is true, this describes a persistent, continuously-playing
// layer (identified by Actor) rather than a one-shot hit: the backend
// starts it once and thereafter just smoothly retargets its gain/pitch on
// each subsequent Sample event with the same Actor. DurationMs is ignored
// for loops. This is how a theme builds a crossfade (e.g. a gentle-stream
// layer and a rushing-stream layer whose gains move in opposite directions
// as bandwidth rises).
type Sample struct {
	Actor      string
	Channel    int
	Group      string
	Loop       bool
	Pitch      float64 // playback pitch-shift ratio; 1.0 = unshifted
	Velocity   float64 // 0..1 gain
	Pan        float64 // -1 (left) .. 1 (right), 0 = centre
	DurationMs int
}

func (s Sample) Describe() string {
	return fmt.Sprintf("SAMPLE actor=%s channel=%d group=%s loop=%t pitch=%.3f velocity=%.3f pan=%.2f duration=%dms",
		s.Actor, s.Channel, s.Group, s.Loop, s.Pitch, s.Velocity, s.Pan, s.DurationMs)
}

// Control is a continuously varying environmental parameter (river volume,
// filter cutoff, crowd density) rather than a discrete event. Value is
// already ramped into the theme-defined target range; Controller is an
// optional backend-specific parameter id (e.g. a MIDI CC number) that
// non-MIDI backends are free to ignore.
type Control struct {
	Actor      string
	Channel    int
	Controller int
	Value      float64
}

func (c Control) Describe() string {
	return fmt.Sprintf("CONTROL actor=%s channel=%d controller=%d value=%.3f",
		c.Actor, c.Channel, c.Controller, c.Value)
}
