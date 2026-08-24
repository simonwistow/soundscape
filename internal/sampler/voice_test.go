package sampler

import "testing"

func constantSample(length int, value float32) *Sample {
	frames := make([]float32, length*2)
	for i := 0; i < length; i++ {
		frames[i*2] = value
		frames[i*2+1] = value
	}
	return &Sample{Name: "test", Frames: frames, Length: length}
}

func TestOneShotVoiceFadesInAndFinishes(t *testing.T) {
	const sampleRate = 1000 // low rate keeps the test buffer small
	s := constantSample(sampleRate, 1.0)
	v := newOneShotVoice(1, s, 1.0, 1.0, 0, 500 /*ms*/, sampleRate)

	out := make([]float32, sampleRate*2)
	v.render(out, sampleRate, sampleRate)

	// First output frame should be faded in (well below full amplitude).
	if out[0] >= 0.5 {
		t.Fatalf("first frame = %v, expected a faded-in start near 0", out[0])
	}

	// A 500ms note at a 1000Hz sample rate should stop within the buffer.
	if !v.finished {
		t.Fatalf("expected voice to finish within a 1-second render at a 500ms duration")
	}

	// Samples at/after the stop point should be silent.
	stopFrame := v.stopAtFrame
	if stopFrame+10 < sampleRate && out[(stopFrame+10)*2] != 0 {
		t.Fatalf("expected silence after stop frame, got %v", out[(stopFrame+10)*2])
	}
}

func TestOneShotVoicePlaysToNaturalEndWithoutDuration(t *testing.T) {
	const sampleRate = 1000
	s := constantSample(200, 1.0)
	v := newOneShotVoice(1, s, 1.0, 1.0, 0, 0 /* no explicit duration */, sampleRate)

	out := make([]float32, 1000*2)
	v.render(out, 1000, sampleRate)

	if !v.finished {
		t.Fatalf("expected voice with no explicit duration to finish at sample end")
	}
}

func TestOneShotVoicePitchShiftChangesDuration(t *testing.T) {
	const sampleRate = 1000
	s := constantSample(200, 1.0)

	slow := newOneShotVoice(1, s, 1.0, 1.0, 0, 0, sampleRate)
	fast := newOneShotVoice(2, s, 2.0, 1.0, 0, 0, sampleRate)

	buf := make([]float32, 1000*2)
	slow.render(buf, 1000, sampleRate)
	fastFramesPlayed := 0
	fast.render(buf, 1000, sampleRate)
	fastFramesPlayed = fast.framesPlayed

	if fastFramesPlayed >= slow.framesPlayed {
		t.Fatalf("double-speed voice should finish in fewer frames: fast=%d slow=%d",
			fastFramesPlayed, slow.framesPlayed)
	}
}

func TestLoopVoiceGlidesTowardsTargetGainAndLoops(t *testing.T) {
	const sampleRate = 1000
	s := constantSample(10, 1.0) // short sample forces multiple loop wraps
	v := newLoopVoice(1, "river", s, 1.0, 1.0, 0, sampleRate)

	out := make([]float32, sampleRate*2)
	v.render(out, sampleRate, sampleRate)

	if v.finished {
		t.Fatalf("loop voices should never finish on their own")
	}
	// Gain should have glided close to the 1.0 target after a full second.
	if v.gain < 0.9 {
		t.Fatalf("gain = %v after 1s of smoothing towards 1.0, expected close to target", v.gain)
	}
	// The sample is only 10 frames long but we rendered 1000 frames, so
	// playback must have wrapped many times without going out of bounds
	// (render would have panicked on an index error otherwise).
}

func TestLoopVoiceRetargetGain(t *testing.T) {
	const sampleRate = 1000
	s := constantSample(50, 1.0)
	v := newLoopVoice(1, "river", s, 1.0, 0.0, 0, sampleRate)

	out := make([]float32, sampleRate*2)
	v.render(out, sampleRate, sampleRate)
	if v.gain > 0.05 {
		t.Fatalf("gain should stay near 0 while target is 0, got %v", v.gain)
	}

	v.targetGain = 1.0
	v.render(out, sampleRate, sampleRate)
	if v.gain < 0.5 {
		t.Fatalf("gain should have moved meaningfully towards new target, got %v", v.gain)
	}
}
