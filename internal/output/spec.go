package output

import (
	"fmt"

	"github.com/simonwistow/soundscape/internal/spec"
)

// Spec is one --output, such as speakers, osc:localhost:57120,prefix=/forest
// or midi-virtual:soundscape (see package spec for the syntax).
type Spec = spec.Spec

var outputs = spec.Set{Flag: "--output", Noun: "output", Kinds: []spec.Kind{
	{Name: "speakers", Help: "play sample sounds, and note sounds with --soundfont, through the audio device (the default)"},
	{Name: "file", Target: "PATH", Options: []string{"bitrate"}, Help: "record what the speakers play to a .wav or .mp3 file, even without speakers; bitrate is the MP3's, in kbps (default 256)"},
	{Name: "midi-file", Target: "PATH", Help: "write note and cc sounds to a Standard MIDI File"},
	{Name: "midi", Target: "PORT", Help: "send note and cc sounds to a MIDI port, live (soundscape midi-ports lists them)"},
	{Name: "midi-virtual", Target: "NAME", Help: "create a virtual MIDI port and send note and cc sounds to it, live (macOS and Linux)"},
	{Name: "osc", Target: "HOST:PORT", Options: []string{"prefix"}, Help: "send every event as an OSC message over UDP; prefix starts every address (default /soundscape)"},
	{Name: "console", Help: "print every event"},
}}

// Kinds returns the --output kinds, for error messages.
func Kinds() []string { return outputs.Names() }

// Help describes every kind of --output, for the flag's usage.
func Help() string { return outputs.Help() }

// ParseSpec parses one --output value.
func ParseSpec(s string) (Spec, error) { return outputs.Parse(s) }

// ParseSpecs parses every --output value, defaulting to speakers when
// there are none, and checks they make sense together.
func ParseSpecs(values []string) ([]Spec, error) {
	if len(values) == 0 {
		values = []string{"speakers"}
	}
	var specs []Spec
	seen := make(map[string]bool)
	for _, v := range values {
		spec, err := ParseSpec(v)
		if err != nil {
			return nil, err
		}
		if (spec.Kind == "speakers" || spec.Kind == "console") && seen[spec.Kind] {
			return nil, fmt.Errorf("--output %s given twice", spec.Kind)
		}
		seen[spec.Kind] = true
		if spec.Kind == "file" || spec.Kind == "midi-file" {
			if seen["path:"+spec.Target] {
				return nil, fmt.Errorf("--output %s: more than one output writes to %s", v, spec.Target)
			}
			seen["path:"+spec.Target] = true
		}
		specs = append(specs, spec)
	}
	return specs, nil
}
