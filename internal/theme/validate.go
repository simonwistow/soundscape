package theme

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidationError is one problem found by Validate. Sound names the
// sound it belongs to, or is empty for theme-level problems.
type ValidationError struct {
	Sound string
	Msg   string
}

func (e ValidationError) Error() string {
	if e.Sound == "" {
		return e.Msg
	}
	return fmt.Sprintf("%s: %s", e.Sound, e.Msg)
}

// Validate checks a loaded Theme for the kinds of mistakes that would
// otherwise only surface as a silent no-op or a confusing runtime error:
// missing/duplicate names, sounds with no input, inverted
// ranges, out-of-range MIDI values, unknown behaviour/output kinds, and
// sample_group directories that don't exist or have no .wav files.
//
// Whether a theme's inputs are actually provided depends on the mapping
// it's played with; see mapping.Unbound.
//
// It intentionally does not flag an inverted min_value/max_value on a
// continuous sound: that's how two sample_loop sounds are set up to
// crossfade against each other (see themes/forest/theme.yaml).
func Validate(t Theme) []error {
	var errs []error

	if strings.TrimSpace(t.Name) == "" {
		errs = append(errs, ValidationError{Msg: "theme name is required"})
	}
	if len(t.Sounds) == 0 {
		errs = append(errs, ValidationError{Msg: "theme defines no sounds"})
	}

	soundNames := make(map[string]bool)
	for _, snd := range t.Sounds {
		label := snd.Name
		if label == "" {
			label = "<unnamed sound>"
			errs = append(errs, ValidationError{Msg: "a sound has no name"})
		} else if soundNames[snd.Name] {
			errs = append(errs, ValidationError{Sound: snd.Name, Msg: "duplicate sound name"})
		}
		soundNames[snd.Name] = true

		if snd.Input == "" {
			errs = append(errs, ValidationError{Sound: label, Msg: "sound has no input"})
		}

		switch snd.Type {
		case "probabilistic":
			errs = append(errs, validateProbabilistic(label, snd)...)
		case "continuous":
			errs = append(errs, validateContinuous(label, snd)...)
		default:
			errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
				"unknown behaviour type %q (expected probabilistic or continuous)", snd.Type)})
		}
	}

	return errs
}

func validateProbabilistic(label string, s Sound) []error {
	var errs []error

	if s.Rate == nil {
		errs = append(errs, ValidationError{Sound: label, Msg: "probabilistic sound has no rate"})
	} else if s.Rate.Min > s.Rate.Max {
		errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
			"invalid rate range: min (%v) > max (%v)", s.Rate.Min, s.Rate.Max)})
	}
	if s.Velocity != nil && s.Velocity.Min > s.Velocity.Max {
		errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
			"invalid velocity range: min (%v) > max (%v)", s.Velocity.Min, s.Velocity.Max)})
	}
	if s.PitchJitter != nil && s.PitchJitter.Min > s.PitchJitter.Max {
		errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
			"invalid pitch_jitter range: min (%v) > max (%v)", s.PitchJitter.Min, s.PitchJitter.Max)})
	}

	switch s.Output {
	case "", "note":
		if len(s.Notes) == 0 {
			errs = append(errs, ValidationError{Sound: label, Msg: "note-output probabilistic sound has no notes"})
		}
		for _, n := range s.Notes {
			if n < 0 || n > 127 {
				errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
					"invalid MIDI note %d (expected 0-127)", n)})
			}
		}
		if s.Channel < 0 || s.Channel > 15 {
			errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
				"invalid MIDI channel %d (expected 0-15)", s.Channel)})
		}
	case "sample":
		errs = append(errs, validateSampleGroup(label, s.SampleGroup)...)
	default:
		errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
			"unknown output %q for probabilistic sound (expected note or sample)", s.Output)})
	}

	return errs
}

func validateContinuous(label string, s Sound) []error {
	var errs []error

	if s.Spread {
		errs = append(errs, ValidationError{Sound: label, Msg: "spread only applies to probabilistic sounds"})
	}

	switch s.Output {
	case "", "cc":
		if s.Controller < 0 || s.Controller > 127 {
			errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
				"invalid MIDI CC controller %d (expected 0-127)", s.Controller)})
		}
		if s.Channel < 0 || s.Channel > 15 {
			errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
				"invalid MIDI channel %d (expected 0-15)", s.Channel)})
		}
	case "sample_loop":
		errs = append(errs, validateSampleGroup(label, s.SampleGroup)...)
	default:
		errs = append(errs, ValidationError{Sound: label, Msg: fmt.Sprintf(
			"unknown output %q for continuous sound (expected cc or sample_loop)", s.Output)})
	}

	return errs
}

func validateSampleGroup(label, dir string) []error {
	if dir == "" {
		return []error{ValidationError{Sound: label, Msg: "sample-output sound has no sample_group"}}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return []error{ValidationError{Sound: label, Msg: fmt.Sprintf("sample_group %q: %v", dir, err)}}
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".wav") {
			count++
		}
	}
	if count == 0 {
		return []error{ValidationError{Sound: label, Msg: fmt.Sprintf("sample_group %q has no .wav files", dir)}}
	}
	return nil
}
