// Package assets finds themes and mappings. A path is read from disk; a
// bare name (--theme forest, --mappings wikipedia) is looked for on disk
// first, under themes/ or mappings/ in the current directory, so a local
// copy can be edited and hot reloaded, and then among the ones built into
// the binary, if it was built with -tags embed.
package assets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/simonwistow/soundscape"
)

// ThemesDir and MappingsDir are where bare names are looked up, on disk
// and in the binary alike.
const (
	ThemesDir   = "themes"
	MappingsDir = "mappings"
)

// Disk is the filesystem as the operating system sees it. Unlike
// os.DirFS, it takes paths as they're given, absolute or relative.
type Disk struct{}

func (Disk) Open(name string) (fs.File, error)          { return os.Open(name) }
func (Disk) ReadFile(name string) ([]byte, error)       { return os.ReadFile(name) }
func (Disk) ReadDir(name string) ([]fs.DirEntry, error) { return os.ReadDir(name) }
func (Disk) Stat(name string) (fs.FileInfo, error)      { return os.Stat(name) }

// Builtin is the themes and mappings built into the binary.
var Builtin fs.FS = soundscape.Builtin

// File is a theme or mapping file: the filesystem it's in, and its path
// there.
type File struct {
	FS      fs.FS
	Path    string
	BuiltIn bool
}

func (f File) String() string {
	if f.BuiltIn {
		return f.Path + " (built in)"
	}
	return f.Path
}

// Theme finds a theme by name or path. A path to a directory means the
// theme.yaml in it.
func Theme(name string) (File, error) {
	if isPath(name) {
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			name = filepath.Join(name, "theme.yaml")
		}
		return File{FS: Disk{}, Path: name}, nil
	}
	return find("theme", name, path.Join(ThemesDir, name, "theme.yaml"), ThemeNames)
}

// Mapping finds a mapping by name or path.
func Mapping(name string) (File, error) {
	if isPath(name) {
		return File{FS: Disk{}, Path: name}, nil
	}
	return find("mapping", name, path.Join(MappingsDir, name+".yaml"), MappingNames)
}

// isPath reports whether name looks like a path (it has a separator, or
// ends in .yaml/.yml) rather than a bare name.
func isPath(name string) bool {
	return strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, '/') ||
		strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func find(kind, name, file string, names func(fs.FS) []string) (File, error) {
	if _, err := os.Stat(filepath.FromSlash(file)); err == nil {
		return File{FS: Disk{}, Path: filepath.FromSlash(file)}, nil
	}
	if _, err := fs.Stat(Builtin, file); err == nil {
		return File{FS: Builtin, Path: file, BuiltIn: true}, nil
	}
	return File{}, fmt.Errorf("no %s %q: there's no %s%s", kind, name, filepath.FromSlash(file), builtIn(names))
}

// builtIn describes the built-in themes or mappings, for an error message.
func builtIn(names func(fs.FS) []string) string {
	if !soundscape.Embedded {
		return ", and none are built in (build with -tags embed for that)"
	}
	return ", and none is built in by that name (built in: " + strings.Join(names(Builtin), ", ") + ")"
}

// ThemeNames lists the themes in fsys: the directories under themes/ with
// a theme.yaml.
func ThemeNames(fsys fs.FS) []string {
	matches, _ := fs.Glob(fsys, path.Join(ThemesDir, "*", "theme.yaml"))
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = path.Base(path.Dir(m))
	}
	sort.Strings(names)
	return names
}

// MappingNames lists the mappings in fsys: the .yaml files in mappings/.
func MappingNames(fsys fs.FS) []string {
	matches, _ := fs.Glob(fsys, path.Join(MappingsDir, "*.yaml"))
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = strings.TrimSuffix(path.Base(m), ".yaml")
	}
	sort.Strings(names)
	return names
}

// ExportTheme copies the built-in theme name, samples and all, into dir,
// and returns how many files it wrote. It writes nothing if any of them
// is already there.
func ExportTheme(name, dir string) (int, error) {
	root := path.Join(ThemesDir, name)
	if _, err := fs.Stat(Builtin, path.Join(root, "theme.yaml")); err != nil {
		return 0, fmt.Errorf("no built-in theme %q%s", name, builtIn(ThemeNames))
	}
	var files []string
	err := fs.WalkDir(Builtin, root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return err
	})
	if err != nil {
		return 0, err
	}
	targets := make(map[string]string, len(files))
	for _, f := range files {
		targets[f] = filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(f, root+"/")))
	}
	return len(files), export(targets)
}

// ExportMapping copies the built-in mapping name into dir, as name.yaml,
// and returns the file it wrote, unless that's already there.
func ExportMapping(name, dir string) (string, error) {
	file := path.Join(MappingsDir, name+".yaml")
	if _, err := fs.Stat(Builtin, file); err != nil {
		return "", fmt.Errorf("no built-in mapping %q%s", name, builtIn(MappingNames))
	}
	target := filepath.Join(dir, name+".yaml")
	return target, export(map[string]string{file: target})
}

// export copies built-in files to their targets on disk, refusing to
// overwrite anything.
func export(targets map[string]string) error {
	for _, target := range targets {
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("%s is already there; not overwriting it", target)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	for from, target := range targets {
		b, err := fs.ReadFile(Builtin, from)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
