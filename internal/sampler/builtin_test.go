//go:build embed

package sampler

import (
	"testing"

	"github.com/simonwistow/soundscape/internal/assets"
)

func TestLoadGroupFromTheBinary(t *testing.T) {
	g, err := LoadGroup(assets.Builtin, "themes/forest/samples/birds", 44100)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Samples) == 0 || g.Samples[0].Length == 0 {
		t.Fatalf("loaded %d samples, want some with audio in", len(g.Samples))
	}
}
