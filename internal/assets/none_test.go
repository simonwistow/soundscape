//go:build !embed

package assets

import (
	"strings"
	"testing"
)

func TestNothingIsBuiltInWithoutTheTag(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := Theme("forest")
	if err == nil || !strings.Contains(err.Error(), "-tags embed") {
		t.Fatalf("Theme(forest) error = %v, want one saying how to build the themes in", err)
	}
	if _, err := ExportTheme("forest", t.TempDir()); err == nil {
		t.Fatal("exported a theme that isn't built in")
	}
}
