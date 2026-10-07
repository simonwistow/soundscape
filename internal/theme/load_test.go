package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResolvesSampleGroupRelativeToThemeFile(t *testing.T) {
	dir := t.TempDir()
	themePath := filepath.Join(dir, "theme.yaml")

	yaml := `
name: test
sounds:
  - name: birds
    type: probabilistic
    output: sample
    input: activity
    rate: {min: 0.1, max: 1}
    sample_group: samples/birds
`
	if err := os.WriteFile(themePath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("writing theme file: %v", err)
	}

	th, err := Load(themePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := filepath.Join(dir, "samples/birds")
	if got := th.Sounds[0].SampleGroup; got != want {
		t.Fatalf("SampleGroup = %q, want %q", got, want)
	}
}

func TestLoadLeavesAbsoluteSampleGroupUnchanged(t *testing.T) {
	dir := t.TempDir()
	themePath := filepath.Join(dir, "theme.yaml")

	yaml := `
name: test
sounds:
  - name: birds
    type: probabilistic
    output: sample
    input: activity
    rate: {min: 0.1, max: 1}
    sample_group: /abs/path/birds
`
	if err := os.WriteFile(themePath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("writing theme file: %v", err)
	}

	th, err := Load(themePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := th.Sounds[0].SampleGroup; got != "/abs/path/birds" {
		t.Fatalf("SampleGroup = %q, want unchanged absolute path", got)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	themePath := filepath.Join(dir, "theme.yaml")

	// The pre-mapping theme format: sources/metric now live in a mapping.
	yaml := `
name: test
sources:
  - name: requests
    metric: requests
sounds: []
`
	if err := os.WriteFile(themePath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("writing theme file: %v", err)
	}

	if _, err := Load(themePath); err == nil {
		t.Fatal("expected an error for the unknown sources field")
	}
}
