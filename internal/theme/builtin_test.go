//go:build embed

package theme

import (
	"path"
	"testing"

	"github.com/simonwistow/soundscape/internal/assets"
)

// Every theme built into the binary should load and validate from there,
// samples included.
func TestBuiltinThemesValidate(t *testing.T) {
	names := assets.ThemeNames(assets.Builtin)
	if len(names) == 0 {
		t.Fatal("no built-in themes")
	}
	for _, name := range names {
		p := path.Join(assets.ThemesDir, name, "theme.yaml")
		th, err := Load(assets.Builtin, p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		if th.Files == nil {
			t.Errorf("%s: loaded from the binary, but its samples would be looked for on disk", p)
		}
		for _, e := range Validate(th) {
			t.Errorf("%s: %v", p, e)
		}
	}
}
