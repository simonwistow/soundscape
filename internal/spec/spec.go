// Package spec parses the kind[:target][,option=value...] values that
// --source and --output take, such as
//
//	speakers
//	osc:localhost:57120,prefix=/forest
//	fastly:service=SID
//	file:monday.jsonl,loop=true
//
// The target runs from the first colon to the first comma, so it can hold
// colons (host:port, URLs, ALSA port names) but not commas. A kind that
// takes no target can still take options straight after the colon.
package spec

import (
	"fmt"
	"slices"
	"strings"
)

// Spec is one parsed value.
type Spec struct {
	Kind    string
	Target  string
	Options map[string]string
}

// String writes the spec back out, options sorted.
func (s Spec) String() string {
	out := s.Kind
	parts := []string{}
	if s.Target != "" {
		parts = append(parts, s.Target)
	}
	keys := make([]string, 0, len(s.Options))
	for k := range s.Options {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		parts = append(parts, k+"="+s.Options[k])
	}
	if len(parts) > 0 {
		out += ":" + strings.Join(parts, ",")
	}
	return out
}

// Kind describes one kind of value for parsing and help.
type Kind struct {
	Name string
	// Target says what the target is, for help, e.g. "PATH"; "" means the
	// kind takes none.
	Target string
	// OptionalTarget means the target may be left out.
	OptionalTarget bool
	Options        []string
	Help           string
}

// Set is the kinds one flag takes.
type Set struct {
	Flag  string // e.g. "--output", for messages
	Noun  string // e.g. "output"
	Kinds []Kind
}

// Names returns the kinds' names, for messages.
func (s Set) Names() []string {
	names := make([]string, len(s.Kinds))
	for i, k := range s.Kinds {
		names[i] = k.Name
	}
	return names
}

// Help describes every kind, two lines each, for the flag's usage.
func (s Set) Help() string {
	var b strings.Builder
	for _, k := range s.Kinds {
		form := k.Name
		switch {
		case k.Target != "" && k.OptionalTarget:
			form += "[:" + k.Target + "]"
		case k.Target != "":
			form += ":" + k.Target
		}
		for i, o := range k.Options {
			sep := ","
			if i == 0 && k.Target == "" {
				sep = ":"
			}
			form += "[" + sep + o + "=...]"
		}
		fmt.Fprintf(&b, "\n  %s\n      %s", form, k.Help)
	}
	return b.String()
}

// Parse parses one value.
func (s Set) Parse(v string) (Spec, error) {
	fail := func(format string, args ...any) (Spec, error) {
		return Spec{}, fmt.Errorf("%s %s: %s", s.Flag, v, fmt.Sprintf(format, args...))
	}

	name, rest, hasRest := strings.Cut(v, ":")
	i := slices.IndexFunc(s.Kinds, func(k Kind) bool { return k.Name == name })
	if i < 0 {
		return fail("unknown %s %q (want one of %s)", s.Noun, name, strings.Join(s.Names(), ", "))
	}
	k := s.Kinds[i]

	var parts []string
	if hasRest {
		parts = strings.Split(rest, ",")
	}
	spec := Spec{Kind: name}

	// Does the first part name the target, or is it already an option?
	if k.Target != "" && len(parts) > 0 && !(k.OptionalTarget && isOption(parts[0], k.Options)) {
		spec.Target, parts = parts[0], parts[1:]
	}
	if k.Target != "" && !k.OptionalTarget && spec.Target == "" {
		return fail("%s needs a target: %s:%s", name, name, k.Target)
	}

	for _, opt := range parts {
		key, value, ok := strings.Cut(opt, "=")
		if !ok || key == "" {
			if len(k.Options) == 0 && k.Target == "" {
				return fail("%s takes no target or options", name)
			}
			return fail("option %q isn't key=value", opt)
		}
		if !slices.Contains(k.Options, key) {
			if len(k.Options) == 0 {
				return fail("%s takes no options", name)
			}
			return fail("unknown option %q for %s (want %s)", key, name, strings.Join(k.Options, ", "))
		}
		if _, dup := spec.Options[key]; dup {
			return fail("option %q given twice", key)
		}
		if spec.Options == nil {
			spec.Options = make(map[string]string)
		}
		spec.Options[key] = value
	}
	return spec, nil
}

func isOption(part string, options []string) bool {
	key, _, ok := strings.Cut(part, "=")
	return ok && slices.Contains(options, key)
}
