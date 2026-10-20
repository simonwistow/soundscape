//go:build embed

package assets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemeByNameIsBuiltInWithNoCopyOnDisk(t *testing.T) {
	t.Chdir(t.TempDir())
	f, err := Theme("forest")
	if err != nil {
		t.Fatal(err)
	}
	if !f.BuiltIn || f.Path != "themes/forest/theme.yaml" {
		t.Fatalf("Theme(forest) = %v, want the built-in themes/forest/theme.yaml", f)
	}
}

func TestMappingByNameIsBuiltInWithNoCopyOnDisk(t *testing.T) {
	t.Chdir(t.TempDir())
	f, err := Mapping("wikipedia")
	if err != nil || !f.BuiltIn || f.Path != "mappings/wikipedia.yaml" {
		t.Errorf("Mapping(wikipedia) = %v, %v; want the built-in one", f, err)
	}
}

func TestUnknownNameListsTheBuiltIns(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := Theme("jungle")
	if err == nil || !strings.Contains(err.Error(), "forest") {
		t.Fatalf("Theme(jungle) error = %v, want one listing the built-in themes", err)
	}
	_, err = Mapping("nagios")
	if err == nil || !strings.Contains(err.Error(), "wikipedia") {
		t.Fatalf("Mapping(nagios) error = %v, want one listing the built-in mappings", err)
	}
}

func TestExportTheme(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "forest")
	n, err := ExportTheme("forest", dir)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("exported %d files, want theme.yaml and samples", n)
	}
	want, _ := os.ReadFile(filepath.Join("..", "..", "themes", "forest", "theme.yaml"))
	got, err := os.ReadFile(filepath.Join(dir, "theme.yaml"))
	if err != nil || string(got) != string(want) {
		t.Fatalf("exported theme.yaml differs from themes/forest/theme.yaml (%v)", err)
	}
	if _, err := ExportTheme("forest", dir); err == nil || !strings.Contains(err.Error(), "already there") {
		t.Fatalf("exporting over an export: %v, want a refusal", err)
	}
	if _, err := ExportTheme("jungle", dir); err == nil {
		t.Fatal("exporting an unknown theme worked")
	}
}

func TestExportMapping(t *testing.T) {
	dir := t.TempDir()
	p, err := ExportMapping("fastly", dir)
	if err != nil || p != filepath.Join(dir, "fastly.yaml") {
		t.Fatalf("ExportMapping = %q, %v", p, err)
	}
	if _, err := ExportMapping("fastly", dir); err == nil {
		t.Fatal("exporting over an export worked")
	}
}
