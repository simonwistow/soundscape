package midi

import (
	"strings"
	"testing"

	"gitlab.com/gomidi/midi/v2/drivers"
)

// fakePort is a drivers.Out with only a name; findPort needs nothing more.
type fakePort struct {
	drivers.Out
	name string
}

func (f fakePort) String() string { return f.name }

func TestFindPort(t *testing.T) {
	outs := []drivers.Out{
		fakePort{name: "IAC Driver Bus 1"},
		fakePort{name: "Synth"},
		fakePort{name: "Synth 2"},
	}
	cases := []struct {
		name, want, err string
	}{
		{name: "Synth", want: "Synth"},
		{name: "IAC", want: "IAC Driver Bus 1"},
		{name: "Synth 2", want: "Synth 2"},
		{name: "Syn", err: "more than one port"},
		{name: "Piano", err: `no output port called "Piano"`},
	}
	for _, c := range cases {
		got, err := findPort(outs, c.name)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("findPort(%q): error %v, want one containing %q", c.name, err, c.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("findPort(%q): %v", c.name, err)
			continue
		}
		if got.String() != c.want {
			t.Errorf("findPort(%q) = %q, want %q", c.name, got.String(), c.want)
		}
	}
}

func TestFindPortWithNoPorts(t *testing.T) {
	if _, err := findPort(nil, "IAC"); err == nil || !strings.Contains(err.Error(), "available: none") {
		t.Fatalf("got %v, want an error saying no ports are available", err)
	}
}
