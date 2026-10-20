//go:build embed

package mapping

import (
	"path"
	"testing"

	"github.com/simonwistow/soundscape/internal/assets"
)

// Every mapping built into the binary should load and validate.
func TestBuiltinMappingsValidate(t *testing.T) {
	names := assets.MappingNames(assets.Builtin)
	if len(names) == 0 {
		t.Fatal("no built-in mappings")
	}
	for _, name := range names {
		p := path.Join(assets.MappingsDir, name+".yaml")
		m, err := Load(assets.Builtin, p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		for _, e := range Validate(m) {
			t.Errorf("%s: %v", p, e)
		}
	}
}
