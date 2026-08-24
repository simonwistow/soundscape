package sampler

import "math"

// gainSmoothingSeconds controls how quickly a loop voice's gain and pitch
// glide towards a newly-set target, avoiding zipper noise when a
// continuous control value updates every tick.
const gainSmoothingSeconds = 0.15

// fadeSeconds is the one-shot click-avoidance fade applied at the start and
// end of every triggered sample.
const fadeSeconds = 0.008

type voice struct {
	id      uint64
	looping bool
	key     string // Actor name, used to find/update a loop voice
	sample  *Sample

	pos   float64 // playback position in source frames
	pitch float64 // playback rate ratio; 1.0 = unshifted

	gain       float32 // current smoothed gain
	targetGain float32 // loop voices glide towards this
	pan        float32 // -1..1

	// One-shot playback bookkeeping. stopAtFrame is the output-frame index
	// (from voice start) at which the note should be silent; -1 means "play
	// until the sample data runs out".
	stopAtFrame  int
	framesPlayed int
	fadeFrames   int
	finished     bool
}

func newOneShotVoice(id uint64, sample *Sample, pitch float64, gain, pan float32, durationMs, sampleRate int) *voice {
	if pitch <= 0 {
		pitch = 1
	}
	v := &voice{
		id:         id,
		sample:     sample,
		pos:        0,
		pitch:      pitch,
		gain:       gain,
		targetGain: gain,
		pan:        pan,
		fadeFrames: int(fadeSeconds * float64(sampleRate)),
	}
	if durationMs > 0 {
		v.stopAtFrame = int(float64(durationMs) / 1000 * float64(sampleRate))
	} else {
		v.stopAtFrame = -1
	}
	return v
}

func newLoopVoice(id uint64, key string, sample *Sample, pitch float64, gain, pan float32, sampleRate int) *voice {
	if pitch <= 0 {
		pitch = 1
	}
	return &voice{
		id:          id,
		looping:     true,
		key:         key,
		sample:      sample,
		pitch:       pitch,
		gain:        0, // glide up from silence rather than starting with a click
		targetGain:  gain,
		pan:         pan,
		stopAtFrame: -1,
		fadeFrames:  int(fadeSeconds * float64(sampleRate)),
	}
}

// smoothingCoeff returns the per-sample interpolation factor for a one-pole
// filter that reaches ~63% of the way to its target after smoothingSeconds.
func smoothingCoeff(sampleRate int) float32 {
	return float32(1 - math.Exp(-1/(gainSmoothingSeconds*float64(sampleRate))))
}

// render mixes this voice's next `frames` output frames into out (an
// interleaved stereo buffer of length frames*2), advancing its playback
// state. It sets v.finished once a one-shot voice has nothing left to play.
func (v *voice) render(out []float32, frames int, sampleRate int) {
	if v.finished || v.sample == nil || v.sample.Length == 0 {
		return
	}

	coeff := smoothingCoeff(sampleRate)
	angle := (float64(v.pan) + 1) * math.Pi / 4
	panL := float32(math.Cos(angle))
	panR := float32(math.Sin(angle))

	length := v.sample.Length
	data := v.sample.Frames

	for i := 0; i < frames; i++ {
		if v.looping {
			v.gain += (v.targetGain - v.gain) * coeff
		}

		envelope := v.envelopeAt(v.framesPlayed)

		i0 := int(v.pos)
		var i1 int
		if v.looping {
			i0 %= length
			i1 = (i0 + 1) % length
		} else {
			if i0 >= length-1 {
				v.finished = true
				return
			}
			i1 = i0 + 1
			if i1 >= length {
				i1 = length - 1
			}
		}

		frac := float32(v.pos - math.Floor(v.pos))
		l := data[i0*2] + (data[i1*2]-data[i0*2])*frac
		r := data[i0*2+1] + (data[i1*2+1]-data[i0*2+1])*frac

		g := v.gain * envelope
		out[i*2] += l * g * panL
		out[i*2+1] += r * g * panR

		v.pos += v.pitch
		v.framesPlayed++

		if !v.looping && v.stopAtFrame >= 0 && v.framesPlayed >= v.stopAtFrame {
			v.finished = true
			return
		}
	}
}

// envelopeAt returns the fade-in/fade-out multiplier for a one-shot voice at
// the given number of frames played, avoiding clicks at note start/end.
// Loop voices ignore this (their gain is smoothed separately) and always
// return 1.
func (v *voice) envelopeAt(framesPlayed int) float32 {
	if v.looping || v.fadeFrames <= 0 {
		return 1
	}

	env := float32(1)
	if framesPlayed < v.fadeFrames {
		env = float32(framesPlayed) / float32(v.fadeFrames)
	}

	if v.stopAtFrame >= 0 {
		remaining := v.stopAtFrame - framesPlayed
		if remaining < v.fadeFrames {
			out := float32(remaining) / float32(v.fadeFrames)
			if out < env {
				env = out
			}
		}
	}

	if env < 0 {
		env = 0
	}
	return env
}
