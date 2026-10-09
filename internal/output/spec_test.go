package output

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSpec(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Spec
	}{
		{"speakers", Spec{Kind: "speakers"}},
		{"console", Spec{Kind: "console"}},
		{"midi-file:session.mid", Spec{Kind: "midi-file", Target: "session.mid"}},
		{"file:storm.mp3", Spec{Kind: "file", Target: "storm.mp3"}},
		{"file:storm.mp3,bitrate=192", Spec{Kind: "file", Target: "storm.mp3", Options: map[string]string{"bitrate": "192"}}},
		{"midi:IAC Driver Bus 1", Spec{Kind: "midi", Target: "IAC Driver Bus 1"}},
		// ALSA port names have colons of their own.
		{"midi:Midi Through:Midi Through Port-0 14:0", Spec{Kind: "midi", Target: "Midi Through:Midi Through Port-0 14:0"}},
		{"midi-virtual:soundscape", Spec{Kind: "midi-virtual", Target: "soundscape"}},
		{"osc:localhost:57120", Spec{Kind: "osc", Target: "localhost:57120"}},
		{"osc:localhost:57120,prefix=/forest", Spec{Kind: "osc", Target: "localhost:57120", Options: map[string]string{"prefix": "/forest"}}},
	} {
		got, err := ParseSpec(tc.in)
		if err != nil {
			t.Errorf("ParseSpec(%q): %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseSpec(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestParseSpecErrors(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"speaker", `unknown output "speaker"`},
		{"", `unknown output ""`},
		{"speakers:default", "speakers takes no target"},
		{"osc", "osc needs a target: osc:HOST:PORT"},
		{"osc:", "osc needs a target"},
		{"midi-file:a.mid,prefix=/x", "midi-file takes no options"},
		{"osc:localhost:57120,port=1", `unknown option "port" for osc (want prefix)`},
		{"osc:localhost:57120,prefix", `option "prefix" isn't key=value`},
		{"osc:localhost:57120,prefix=/a,prefix=/b", `option "prefix" given twice`},
	} {
		_, err := ParseSpec(tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ParseSpec(%q) error = %v, want it to mention %q", tc.in, err, tc.want)
		}
	}
}

func TestParseSpecsDefaultsToSpeakers(t *testing.T) {
	specs, err := ParseSpecs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []Spec{{Kind: "speakers"}}; !reflect.DeepEqual(specs, want) {
		t.Errorf("ParseSpecs(nil) = %+v, want %+v", specs, want)
	}
}

func TestParseSpecsAllowsSeveralOfAKind(t *testing.T) {
	specs, err := ParseSpecs([]string{"osc:localhost:57120", "osc:otherhost:9000", "speakers"})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 3 {
		t.Errorf("got %d specs, want 3", len(specs))
	}
}

func TestParseSpecsRejectsTwoOutputsToOneFile(t *testing.T) {
	if _, err := ParseSpecs([]string{"file:a.wav", "file:a.wav,bitrate=1"}); err == nil {
		t.Error("two file outputs to a.wav: no error")
	}
	if _, err := ParseSpecs([]string{"file:a.mid", "midi-file:a.mid"}); err == nil {
		t.Error("file and midi-file to a.mid: no error")
	}
	if _, err := ParseSpecs([]string{"file:a.wav", "file:a.mp3"}); err != nil {
		t.Errorf("a.wav and a.mp3: %v", err)
	}
}

func TestParseSpecsRejectsSpeakersTwice(t *testing.T) {
	if _, err := ParseSpecs([]string{"speakers", "speakers"}); err == nil {
		t.Error("speakers twice: no error")
	}
}

func TestHelpListsEveryKind(t *testing.T) {
	help := Help()
	for _, k := range Kinds() {
		if !strings.Contains(help, "\n  "+k) {
			t.Errorf("Help() doesn't describe %s", k)
		}
	}
	if !strings.Contains(help, "osc:HOST:PORT[,prefix=...]") {
		t.Errorf("Help() doesn't show osc's form:\n%s", help)
	}
}
