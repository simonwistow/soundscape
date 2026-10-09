package output

import (
	"fmt"
	"slices"
	"strings"
)

// Spec is one --output: a kind of output, what it goes to, and options for
// it, written kind[:target][,option=value...]. For example
//
//	speakers
//	osc:localhost:57120,prefix=/forest
//	midi-virtual:soundscape
//
// The target runs from the first colon to the first comma, so it can hold
// colons (host:port, ALSA port names) but not commas.
type Spec struct {
	Kind    string
	Target  string
	Options map[string]string
}

// kind describes one kind of output for parsing and help.
type kind struct {
	name    string
	target  string // what the target is, for help; "" means it takes none
	options []string
	help    string
}

var kinds = []kind{
	{name: "speakers", help: "play sample sounds, and note sounds with --soundfont, through the audio device (the default)"},
	{name: "midi-file", target: "PATH", help: "write note and cc sounds to a Standard MIDI File"},
	{name: "midi", target: "PORT", help: "send note and cc sounds to a MIDI port, live (soundscape midi-ports lists them)"},
	{name: "midi-virtual", target: "NAME", help: "create a virtual MIDI port and send note and cc sounds to it, live (macOS and Linux)"},
	{name: "osc", target: "HOST:PORT", options: []string{"prefix"}, help: "send every event as an OSC message over UDP; prefix starts every address (default /soundscape)"},
	{name: "console", help: "print every event"},
}

// Kinds returns the --output kinds, for error messages.
func Kinds() []string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = k.name
	}
	return names
}

// Help describes every kind of --output, one per line, for the flag's usage.
func Help() string {
	var b strings.Builder
	for _, k := range kinds {
		form := k.name
		if k.target != "" {
			form += ":" + k.target
		}
		for _, o := range k.options {
			form += "[," + o + "=...]"
		}
		fmt.Fprintf(&b, "\n  %s\n      %s", form, k.help)
	}
	return b.String()
}

// ParseSpec parses one --output value.
func ParseSpec(s string) (Spec, error) {
	name, rest, hasTarget := strings.Cut(s, ":")
	i := slices.IndexFunc(kinds, func(k kind) bool { return k.name == name })
	if i < 0 {
		return Spec{}, fmt.Errorf("--output %s: unknown output %q (want one of %s)", s, name, strings.Join(Kinds(), ", "))
	}
	k := kinds[i]

	parts := strings.Split(rest, ",")
	spec := Spec{Kind: name, Target: parts[0]}

	switch {
	case k.target == "" && hasTarget:
		return Spec{}, fmt.Errorf("--output %s: %s takes no target or options", s, name)
	case k.target != "" && spec.Target == "":
		return Spec{}, fmt.Errorf("--output %s: %s needs a target: %s:%s", s, name, name, k.target)
	}

	for _, opt := range parts[1:] {
		key, value, ok := strings.Cut(opt, "=")
		if !ok || key == "" {
			return Spec{}, fmt.Errorf("--output %s: option %q isn't key=value", s, opt)
		}
		if !slices.Contains(k.options, key) {
			if len(k.options) == 0 {
				return Spec{}, fmt.Errorf("--output %s: %s takes no options", s, name)
			}
			return Spec{}, fmt.Errorf("--output %s: unknown option %q for %s (want %s)", s, key, name, strings.Join(k.options, ", "))
		}
		if _, dup := spec.Options[key]; dup {
			return Spec{}, fmt.Errorf("--output %s: option %q given twice", s, key)
		}
		if spec.Options == nil {
			spec.Options = make(map[string]string)
		}
		spec.Options[key] = value
	}
	return spec, nil
}

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
		specs = append(specs, spec)
	}
	return specs, nil
}
