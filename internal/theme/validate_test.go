package theme

import (
	"strings"
	"testing"
)

func TestValidateForestThemePasses(t *testing.T) {
	th, err := Load("../../themes/forest/theme.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if errs := Validate(th); len(errs) != 0 {
		t.Fatalf("expected the real forest theme to validate cleanly, got: %v", errs)
	}
}

func TestValidateMarketThemePasses(t *testing.T) {
	th, err := Load("../../themes/market/theme.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if errs := Validate(th); len(errs) != 0 {
		t.Fatalf("expected the real market theme to validate cleanly, got: %v", errs)
	}
}

func containsMsg(errs []error, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), substr) {
			return true
		}
	}
	return false
}

func TestValidateCatchesMissingInput(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "birds", Type: "probabilistic", Rate: &Rate{Min: 0.1, Max: 1}, Notes: []int{60}},
		},
	}
	errs := Validate(th)
	if !containsMsg(errs, "sound has no input") {
		t.Fatalf("expected missing-input error, got: %v", errs)
	}
}

func TestValidateCatchesUnknownBehaviourType(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "birds", Type: "spooky", Input: "activity"},
		},
	}
	errs := Validate(th)
	if !containsMsg(errs, "unknown behaviour type") {
		t.Fatalf("expected unknown-behaviour error, got: %v", errs)
	}
}

func TestValidateCatchesInvertedRanges(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "birds", Type: "probabilistic", Input: "activity", Rate: &Rate{Min: 5, Max: 1}, Notes: []int{60}},
		},
	}
	errs := Validate(th)
	if !containsMsg(errs, "invalid rate range") {
		t.Fatalf("expected invalid rate range error, got: %v", errs)
	}
}

func TestValidateCatchesInvalidMIDIValues(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "birds", Type: "probabilistic", Input: "activity", Rate: &Rate{Min: 0.1, Max: 1},
				Notes: []int{200}, Channel: 99},
			{Name: "river", Type: "continuous", Input: "activity", Controller: 500, Channel: -1},
		},
	}
	errs := Validate(th)
	if !containsMsg(errs, "invalid MIDI note") {
		t.Fatalf("expected invalid MIDI note error, got: %v", errs)
	}
	if !containsMsg(errs, "invalid MIDI channel") {
		t.Fatalf("expected invalid MIDI channel error, got: %v", errs)
	}
	if !containsMsg(errs, "invalid MIDI CC controller") {
		t.Fatalf("expected invalid CC controller error, got: %v", errs)
	}
}

func TestValidateCatchesMissingSampleGroup(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "birds", Type: "probabilistic", Output: "sample", Input: "activity",
				Rate: &Rate{Min: 0.1, Max: 1}, SampleGroup: "/no/such/directory"},
		},
	}
	errs := Validate(th)
	if !containsMsg(errs, "sample_group") {
		t.Fatalf("expected a sample_group error, got: %v", errs)
	}
}

func TestValidateCatchesDuplicateSoundNames(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "birds", Type: "probabilistic", Input: "activity", Rate: &Rate{Min: 0.1, Max: 1}, Notes: []int{60}},
			{Name: "birds", Type: "probabilistic", Input: "activity", Rate: &Rate{Min: 0.1, Max: 1}, Notes: []int{62}},
		},
	}
	errs := Validate(th)
	if !containsMsg(errs, "duplicate sound name") {
		t.Fatalf("expected duplicate sound name error, got: %v", errs)
	}
}

func TestValidateAllowsInvertedContinuousRampForCrossfade(t *testing.T) {
	th := Theme{
		Name: "test",
		Sounds: []Sound{
			{Name: "river-gentle", Type: "continuous", Input: "flow",
				Controller: 74, MinValue: 1, MaxValue: 0},
		},
	}
	// MinValue > MaxValue is intentional for crossfade layers and must not
	// be flagged as an "invalid range".
	errs := Validate(th)
	if containsMsg(errs, "invalid") {
		t.Fatalf("did not expect an invalid-range error for a crossfade ramp, got: %v", errs)
	}
}
