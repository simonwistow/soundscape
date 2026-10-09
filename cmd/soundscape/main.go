package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/simonwistow/soundscape/internal/fastly"
	"github.com/simonwistow/soundscape/internal/mapping"
	"github.com/simonwistow/soundscape/internal/midi"
	"github.com/simonwistow/soundscape/internal/osc"
	"github.com/simonwistow/soundscape/internal/output"
	"github.com/simonwistow/soundscape/internal/prometheus"
	"github.com/simonwistow/soundscape/internal/sampler"
	"github.com/simonwistow/soundscape/internal/source"
	"github.com/simonwistow/soundscape/internal/synth"
	"github.com/simonwistow/soundscape/internal/theme"
	"github.com/simonwistow/soundscape/internal/wikipedia"
)

const sampleRate = 44100

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "validate":
			runValidate(os.Args[2:])
			return
		case "soundfont-presets":
			runSoundFontPresets(os.Args[2:])
			return
		case "midi-ports":
			runMIDIPorts()
			return
		case "version", "--version":
			fmt.Println("soundscape", buildVersion())
			return
		}
	}

	var (
		aliases    = flag.String("aliases", "", "mapping from source metrics to theme inputs: a name in mappings/ or a path (default: the one named after --source if given, else fastly if a service ID is set, else simulate)")
		sourceName = flag.String("source", "", "override the mapping's source: "+strings.Join(mapping.Sources, ", "))
		simulate   = flag.Bool("simulate", false, "shorthand for --source simulate")

		serviceID = flag.String("service-id", os.Getenv("FASTLY_SERVICE_ID"), "Fastly service ID")
		token     = flag.String("token", os.Getenv("FASTLY_API_TOKEN"), "Fastly API token")

		promURL      = flag.String("prometheus-url", envOr("PROMETHEUS_URL", "http://localhost:9090"), "Prometheus server URL")
		promInterval = flag.Duration("prometheus-interval", time.Second, "how often to evaluate the Prometheus queries")

		wikis = flag.String("wikipedia-wikis", "", "comma-separated wiki IDs to count (e.g. enwiki,dewiki); default all Wikimedia wikis")

		themePath = flag.String("theme", "themes/forest/theme.yaml", "theme YAML file")
		soundFont = flag.String("soundfont", "", "optional SF2 SoundFont, for note/cc-output sounds")
		midiOut   = flag.String("midi-out", "", "optional path to write a Standard MIDI File (.mid) of note/cc-output sounds")
		midiPort  = flag.String("midi-port", "", "send note/cc-output sounds to this MIDI output port, live (see `soundscape midi-ports`)")
		midiVirt  = flag.String("midi-virtual", "", "create a virtual MIDI port with this name and send note/cc-output sounds to it, live (macOS and Linux)")
		oscAddr   = flag.String("osc", "", "send every event as an OSC message over UDP to this host:port, e.g. localhost:57120 for SuperCollider")
		oscPrefix = flag.String("osc-prefix", "/soundscape", "the start of every OSC address")
		samples   = flag.Bool("sample-player", true, "play sample/sample_loop sounds through the built-in sample player; false leaves them to other outputs, such as --osc")
		verbose   = flag.Bool("verbose", false, "log telemetry")
		seed      = flag.Int64("seed", 0, "random seed for probabilistic events (0 = random each run)")
		watch     = flag.Bool("watch", true, "reload the theme and mapping files automatically when they change")
	)
	flag.Parse()

	if *simulate {
		*sourceName = "simulate"
	}
	if *aliases == "" {
		switch {
		case *sourceName != "":
			*aliases = *sourceName
		case *serviceID != "":
			*aliases = "fastly"
		default:
			*aliases = "simulate"
		}
	}
	mappingPath := mapping.Resolve(*aliases)

	m, err := loadMapping(mappingPath, *sourceName)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("source: %s, mapping: %s", m.Source, mappingPath)

	src, err := buildSource(m, sourceConfig{
		serviceID:    *serviceID,
		token:        *token,
		promURL:      *promURL,
		promInterval: *promInterval,
		wikis:        *wikis,
		verbose:      *verbose,
	})
	if err != nil {
		log.Fatal(err)
	}

	th, err := theme.Load(*themePath)
	if err != nil {
		log.Fatal(err)
	}
	warnUnbound(m, th)

	out, player, closeOutput, err := buildOutput(th, outputConfig{
		soundFont:   *soundFont,
		midiOut:     *midiOut,
		midiPort:    *midiPort,
		midiVirtual: *midiVirt,
		osc:         *oscAddr,
		oscPrefix:   *oscPrefix,
		noSamples:   !*samples,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer closeOutput()

	// A theme run typically loops forever, so make sure Ctrl+C/SIGTERM
	// still finalizes output that needs an explicit close (notably the
	// MIDI file's end-of-track marker) instead of just getting killed.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("shutting down...")
		closeOutput()
		os.Exit(0)
	}()

	var engine *theme.Engine
	if *seed != 0 {
		engine = theme.NewEngineWithSeed(th, out, *seed)
	} else {
		engine = theme.NewEngine(th, out)
	}
	conditioner := mapping.NewConditioner(m)

	if *watch {
		r := &reloader{
			themePath: *themePath, mappingPath: mappingPath, sourceOverride: *sourceName,
			engine: engine, conditioner: conditioner, player: player,
			theme: th, mapping: m,
		}
		go watchFile(*themePath, r.reloadTheme)
		go watchFile(mappingPath, r.reloadMapping)
	}

	err = src.Run(context.Background(), func(ts int64, raw map[string]float64) {
		engine.Process(ts, conditioner.Apply(raw))
	})
	if err != nil {
		log.Fatal(err)
	}
}

// version is set by release builds with -ldflags "-X main.version=v1.2.3".
var version string

// buildVersion reports which soundscape this is: the release it was built
// as, else the module version `go install ...@v1.2.3` records, else "devel".
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}

// loadMapping loads and validates a mapping file, applying a --source
// override first so the override is what gets validated.
func loadMapping(path, sourceOverride string) (mapping.Mapping, error) {
	m, err := mapping.Load(path)
	if err != nil {
		return mapping.Mapping{}, fmt.Errorf("mapping: %w", err)
	}
	if sourceOverride != "" {
		m.Source = sourceOverride
	}
	if issues := mapping.Validate(m); len(issues) > 0 {
		msgs := make([]string, len(issues))
		for i, issue := range issues {
			msgs[i] = issue.Error()
		}
		return mapping.Mapping{}, fmt.Errorf("mapping %s:\n  - %s", path, strings.Join(msgs, "\n  - "))
	}
	return m, nil
}

// warnUnbound logs theme inputs the mapping doesn't provide. They aren't
// fatal - a mapping may reasonably have nothing for, say, "quirks" - but
// those sounds will sit at their minimum.
func warnUnbound(m mapping.Mapping, th theme.Theme) {
	if missing := mapping.Unbound(m, theme.Inputs(th)); len(missing) > 0 {
		log.Printf("warning: theme %q uses inputs the mapping doesn't provide (they'll read as 0): %s",
			th.Name, strings.Join(missing, ", "))
	}
}

type sourceConfig struct {
	serviceID, token string
	promURL          string
	promInterval     time.Duration
	wikis            string
	verbose          bool
}

func buildSource(m mapping.Mapping, cfg sourceConfig) (source.Source, error) {
	switch m.Source {
	case "simulate":
		return source.Simulation{}, nil

	case "fastly":
		if cfg.serviceID == "" || cfg.token == "" {
			return nil, fmt.Errorf("the fastly source needs FASTLY_API_TOKEN and --service-id")
		}
		return fastly.Source{Client: fastly.NewClient(cfg.token, cfg.serviceID), Verbose: cfg.verbose}, nil

	case "prometheus":
		return prometheus.Source{
			Client:   prometheus.NewClient(cfg.promURL),
			Queries:  m.Queries(),
			Interval: cfg.promInterval,
			Verbose:  cfg.verbose,
		}, nil

	case "wikipedia":
		var wikis []string
		for _, w := range strings.Split(cfg.wikis, ",") {
			if w = strings.TrimSpace(w); w != "" {
				wikis = append(wikis, w)
			}
		}
		return wikipedia.Source{Wikis: wikis, Verbose: cfg.verbose}, nil

	default:
		return nil, fmt.Errorf("source %q is not implemented yet", m.Source)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// runValidate implements `soundscape validate [--aliases X] <theme.yaml>`:
// load the theme and report every problem theme.Validate finds, plus, with
// --aliases, the mapping's own problems and any theme inputs it leaves
// unbound. Exits non-zero if anything is found.
func runValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	aliases := fs.String("aliases", "", "also check a mapping (name in mappings/ or path) against the theme")
	fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: soundscape validate [--aliases mapping] <theme.yaml>")
		os.Exit(2)
	}
	path := fs.Arg(0)

	th, err := theme.Load(path)
	if err != nil {
		fmt.Printf("%s: failed to load: %v\n", path, err)
		os.Exit(1)
	}
	issues := theme.Validate(th)

	if *aliases != "" {
		mappingPath := mapping.Resolve(*aliases)
		m, err := mapping.Load(mappingPath)
		if err != nil {
			fmt.Printf("%s: failed to load: %v\n", mappingPath, err)
			os.Exit(1)
		}
		for _, issue := range mapping.Validate(m) {
			issues = append(issues, fmt.Errorf("%s: %w", mappingPath, issue))
		}
		for _, name := range mapping.Unbound(m, theme.Inputs(th)) {
			issues = append(issues, fmt.Errorf("input %q is used by the theme but not provided by %s", name, mappingPath))
		}
	}

	if len(issues) == 0 {
		fmt.Printf("%s: OK (%d sounds, inputs: %s)\n", path, len(th.Sounds), strings.Join(theme.Inputs(th), ", "))
		return
	}

	fmt.Printf("%s: %d problem(s) found\n", path, len(issues))
	for _, issue := range issues {
		fmt.Printf("  - %v\n", issue)
	}
	os.Exit(1)
}

// runMIDIPorts implements `soundscape midi-ports`: list the MIDI output
// ports --midi-port can send to.
func runMIDIPorts() {
	ports, err := midi.Ports()
	if err != nil {
		log.Fatal(err)
	}
	if len(ports) == 0 {
		fmt.Println("no MIDI output ports (use --midi-virtual to create one)")
		return
	}
	for _, p := range ports {
		fmt.Println(p)
	}
}

// runSoundFontPresets implements `soundscape soundfont-presets <file.sf2>`:
// list the instruments a theme's program and bank can pick from it.
func runSoundFontPresets(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: soundscape soundfont-presets <file.sf2>")
		os.Exit(2)
	}
	presets, err := synth.Presets(args[0])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("bank  program  name")
	for _, p := range presets {
		fmt.Printf("%4d  %7d  %s\n", p.Bank, p.Program, p.Name)
	}
}

type outputConfig struct {
	soundFont             string
	midiOut               string
	midiPort, midiVirtual string
	osc, oscPrefix        string
	noSamples             bool
}

// buildOutput assembles whichever output backends the theme and flags call
// for: the WAV sample player is started automatically whenever the theme
// references any sample_group (no flag needed, so a sample-only theme is
// self-contained), the SoundFont backend is added if --soundfont is given,
// the MIDI file backend if --midi-out is given, a live MIDI port if
// --midi-port or --midi-virtual is, and OSC if --osc is. The sample player
// can be turned off, for when another program plays the samples from OSC
// instead. A theme can use several of these at
// once (e.g. sampled birds alongside a SoundFont-driven instrument). With
// none, falls back to the Console backend so the theme can still be
// exercised with no audio at all.
func buildOutput(th theme.Theme, cfg outputConfig) (output.Output, *sampler.Player, func(), error) {
	var outs []output.Output
	var closers []func()
	var player *sampler.Player

	if groups := sampleGroups(th); len(groups) > 0 && !cfg.noSamples {
		p, err := sampler.NewPlayer(sampleRate)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("starting sample player: %w", err)
		}
		for _, dir := range groups {
			if err := p.LoadGroup(dir, dir); err != nil {
				p.Close()
				return nil, nil, nil, fmt.Errorf("loading sample group %s: %w", dir, err)
			}
		}
		player = p
		outs = append(outs, p)
		closers = append(closers, p.Close)
	}

	// Close whatever was already started if a later backend fails.
	fail := func(err error) (output.Output, *sampler.Player, func(), error) {
		for _, c := range closers {
			c()
		}
		return nil, nil, nil, err
	}

	if cfg.soundFont != "" {
		sf, err := synth.NewSoundFontOutput(cfg.soundFont)
		if err != nil {
			return fail(fmt.Errorf("starting SoundFont output: %w", err))
		}
		outs = append(outs, sf)
		closers = append(closers, sf.Close)
	}

	if cfg.midiOut != "" {
		m := midi.NewVirtualOutput(cfg.midiOut)
		outs = append(outs, m)
		closers = append(closers, func() {
			if err := m.Close(); err != nil {
				log.Printf("midi: failed to write %s: %v", cfg.midiOut, err)
				return
			}
			log.Printf("midi: wrote %s", cfg.midiOut)
		})
	}

	if cfg.midiPort != "" && cfg.midiVirtual != "" {
		return fail(fmt.Errorf("use --midi-port or --midi-virtual, not both"))
	}
	if cfg.midiPort != "" || cfg.midiVirtual != "" {
		var live *midi.LiveOutput
		var err error
		if cfg.midiPort != "" {
			live, err = midi.OpenPort(cfg.midiPort)
		} else {
			live, err = midi.OpenVirtual(cfg.midiVirtual)
		}
		if err != nil {
			return fail(err)
		}
		log.Printf("midi: sending to %s", live)
		outs = append(outs, live)
		closers = append(closers, func() {
			if err := live.Close(); err != nil {
				log.Printf("midi: closing %s: %v", live, err)
			}
		})
	}

	if cfg.osc != "" {
		o, err := osc.Dial(cfg.osc, cfg.oscPrefix)
		if err != nil {
			return fail(err)
		}
		log.Printf("osc: sending to %s", o)
		outs = append(outs, o)
		closers = append(closers, func() { _ = o.Close() })
	}

	if len(outs) == 0 {
		outs = append(outs, output.NewConsole())
	}

	closeAll := func() {
		for _, c := range closers {
			c()
		}
	}

	if len(outs) == 1 {
		return outs[0], player, closeAll, nil
	}
	return output.NewMulti(outs...), player, closeAll, nil
}

// watchFile polls path once a second and calls apply when it changes.
// apply is expected to leave the running state untouched on error, so an
// invalid or broken edit is logged and ignored rather than crashing or
// going silent mid-installation.
func watchFile(path string, apply func() error) {
	var lastMod time.Time
	if info, err := os.Stat(path); err == nil {
		lastMod = info.ModTime()
	}

	for range time.Tick(time.Second) {
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().After(lastMod) {
			continue
		}
		lastMod = info.ModTime()

		if err := apply(); err != nil {
			log.Printf("hot reload: %s: %v", path, err)
		}
	}
}

// reloader applies hot-reloaded theme and mapping files. It remembers the
// current pair so each side can be checked against the other.
type reloader struct {
	themePath, mappingPath, sourceOverride string

	engine      *theme.Engine
	conditioner *mapping.Conditioner
	player      *sampler.Player

	mu      sync.Mutex
	theme   theme.Theme
	mapping mapping.Mapping
}

func (r *reloader) reloadTheme() error {
	newTheme, err := theme.Load(r.themePath)
	if err != nil {
		return fmt.Errorf("%v (keeping previous theme)", err)
	}
	if issues := theme.Validate(newTheme); len(issues) > 0 {
		msgs := make([]string, len(issues))
		for i, issue := range issues {
			msgs[i] = issue.Error()
		}
		return fmt.Errorf("%d problem(s) found, keeping previous theme:\n  - %s", len(issues), strings.Join(msgs, "\n  - "))
	}
	if r.player != nil {
		if err := reloadSampleGroups(r.player, newTheme); err != nil {
			return fmt.Errorf("%v (keeping previous theme)", err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	warnUnbound(r.mapping, newTheme)
	r.engine.Reload(newTheme)
	r.theme = newTheme
	log.Printf("hot reload: %s: reloaded (%d sounds)", r.themePath, len(newTheme.Sounds))
	return nil
}

func (r *reloader) reloadMapping() error {
	m, err := loadMapping(r.mappingPath, r.sourceOverride)
	if err != nil {
		return fmt.Errorf("%v\n(keeping previous mapping)", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// The running source was built from the old mapping (including the
	// Prometheus queries), so only conditioning can change live.
	if m.Source != r.mapping.Source {
		return fmt.Errorf("source changed from %s to %s; restart to switch sources (keeping previous mapping)", r.mapping.Source, m.Source)
	}
	if m.Source == "prometheus" && !sameQueries(m, r.mapping) {
		return fmt.Errorf("Prometheus queries changed; restart to apply them (keeping previous mapping)")
	}
	warnUnbound(m, r.theme)
	r.conditioner.Reload(m)
	r.mapping = m
	log.Printf("hot reload: %s: reloaded (%d inputs)", r.mappingPath, len(m.Inputs))
	return nil
}

func sameQueries(a, b mapping.Mapping) bool {
	qa, qb := a.Queries(), b.Queries()
	if len(qa) != len(qb) {
		return false
	}
	for k, v := range qa {
		if qb[k] != v {
			return false
		}
	}
	return true
}

func reloadSampleGroups(player *sampler.Player, th theme.Theme) error {
	for _, dir := range sampleGroups(th) {
		if err := player.LoadGroup(dir, dir); err != nil {
			return fmt.Errorf("sample group %s: %w", dir, err)
		}
	}
	return nil
}

// sampleGroups returns the distinct sample_group directories a theme
// references, in a stable order.
func sampleGroups(th theme.Theme) []string {
	seen := make(map[string]bool)
	var groups []string
	for _, sound := range th.Sounds {
		if sound.SampleGroup == "" || seen[sound.SampleGroup] {
			continue
		}
		seen[sound.SampleGroup] = true
		groups = append(groups, sound.SampleGroup)
	}
	return groups
}
