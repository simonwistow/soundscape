package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/simonwistow/soundscape"
	"github.com/simonwistow/soundscape/internal/assets"
)

// runThemes implements `soundscape themes`: list the themes and mappings
// a name can pick, and where each comes from - the current directory's
// themes/ and mappings/, or the binary, if it was built with -tags embed.
func runThemes() {
	disk := assets.Disk{}
	list := func(heading string, onDisk, builtIn []string, file func(string) string) {
		fmt.Println(heading + ":")
		names := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(onDisk), builtIn...))))
		if len(names) == 0 {
			fmt.Println("  none here, and none built in")
		}
		for _, name := range names {
			where := "built in"
			switch {
			case slices.Contains(onDisk, name) && slices.Contains(builtIn, name):
				where = file(name) + " (instead of the built-in one)"
			case slices.Contains(onDisk, name):
				where = file(name)
			}
			fmt.Printf("  %-12s %s\n", name, where)
		}
	}
	list("themes", assets.ThemeNames(disk), assets.ThemeNames(assets.Builtin), func(name string) string {
		return filepath.Join(assets.ThemesDir, name, "theme.yaml")
	})
	list("mappings", assets.MappingNames(disk), assets.MappingNames(assets.Builtin), func(name string) string {
		return filepath.Join(assets.MappingsDir, name+".yaml")
	})
}

// runExport implements `soundscape export <name> [dir]`: copy a built-in
// theme, samples and all, or a built-in mapping to disk, to edit. By
// default it goes where a name looks first, so the copy takes over from
// the built-in one.
func runExport(args []string) {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: soundscape export <theme or mapping> [directory]")
		os.Exit(2)
	}
	name := args[0]
	if !soundscape.Embedded {
		fmt.Fprintln(os.Stderr, "this soundscape has no built-in themes or mappings to export (build it with -tags embed)")
		os.Exit(1)
	}

	if slices.Contains(assets.ThemeNames(assets.Builtin), name) {
		dir := filepath.Join(assets.ThemesDir, name)
		if len(args) == 2 {
			dir = args[1]
		}
		n, err := assets.ExportTheme(name, dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote the %s theme to %s (%d files)\n", name, dir, n)
		if len(args) == 1 {
			fmt.Printf("--theme %s now plays it, rather than the built-in one\n", name)
		} else {
			fmt.Printf("play it with --theme %s\n", dir)
		}
		return
	}

	if slices.Contains(assets.MappingNames(assets.Builtin), name) {
		dir := assets.MappingsDir
		if len(args) == 2 {
			dir = args[1]
		}
		file, err := assets.ExportMapping(name, dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote the %s mapping to %s\n", name, file)
		if len(args) == 1 {
			fmt.Printf("--mappings %s now uses it, rather than the built-in one\n", name)
		} else {
			fmt.Printf("use it with --mappings %s\n", file)
		}
		return
	}

	fmt.Fprintf(os.Stderr, "no built-in theme or mapping %q; see soundscape themes\n", name)
	os.Exit(1)
}
