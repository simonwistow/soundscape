package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThemeByNamePrefersDisk(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeFile(t, filepath.Join("themes", "forest", "theme.yaml"), "name: mine\n")
	f, err := Theme("forest")
	if err != nil {
		t.Fatal(err)
	}
	if f.BuiltIn || f.Path != filepath.Join("themes", "forest", "theme.yaml") {
		t.Fatalf("Theme(forest) = %v, want the copy on disk", f)
	}
}

func TestThemePaths(t *testing.T) {
	dir := t.TempDir()
	for in, want := range map[string]string{
		"my.yaml":               "my.yaml",
		"./local/theme.yaml":    "./local/theme.yaml",
		dir:                     filepath.Join(dir, "theme.yaml"), // a directory
		"/abs/elsewhere.yml":    "/abs/elsewhere.yml",
		"themes/forest/x.yaml":  "themes/forest/x.yaml",
		"does/not/exist/either": "does/not/exist/either",
	} {
		f, err := Theme(in)
		if err != nil || f.BuiltIn || f.Path != want {
			t.Errorf("Theme(%q) = %v, %v; want %q on disk", in, f, err, want)
		}
	}
}

func TestMappingPrefersDisk(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, filepath.Join("mappings", "wikipedia.yaml"), "inputs: {}\n")
	f, err := Mapping("wikipedia")
	if err != nil || f.BuiltIn || f.Path != filepath.Join("mappings", "wikipedia.yaml") {
		t.Errorf("Mapping(wikipedia) = %v, %v; want the copy on disk", f, err)
	}
	if f, err := Mapping("my.yaml"); err != nil || f.BuiltIn || f.Path != "my.yaml" {
		t.Errorf("Mapping(my.yaml) = %v, %v; want the path on disk", f, err)
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
