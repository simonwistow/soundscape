package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/simonwistow/soundscape/internal/audio"
	"github.com/simonwistow/soundscape/internal/clock"
	"github.com/simonwistow/soundscape/internal/fastly"
	"github.com/simonwistow/soundscape/internal/mapping"
	"github.com/simonwistow/soundscape/internal/midi"
	"github.com/simonwistow/soundscape/internal/osc"
	"github.com/simonwistow/soundscape/internal/output"
	"github.com/simonwistow/soundscape/internal/prometheus"
	"github.com/simonwistow/soundscape/internal/record"
	"github.com/simonwistow/soundscape/internal/render"
	"github.com/simonwistow/soundscape/internal/sampler"
	"github.com/simonwistow/soundscape/internal/source"
	"github.com/simonwistow/soundscape/internal/synth"
	"github.com/simonwistow/soundscape/internal/telemetry"
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
		aliases    = flag.String("aliases", "", "mapping from source metrics to theme inputs: a name in mappings/ or a path (default: the one named after --source's kind if given, else fastly if FASTLY_SERVICE_ID is set, else simulate)")
		sourceFlag = flag.String("source", "", "where the telemetry comes from, overriding the mapping's source:"+source.Help())
		token      = flag.String("token", os.Getenv("FASTLY_API_TOKEN"), "Fastly API token")

		themePath = flag.String("theme", "themes/forest/theme.yaml", "theme YAML file")
		soundFont = flag.String("soundfont", "", "optional SF2 SoundFont, to play note and cc sounds through the speakers and into recordings")
		verbose   = flag.Bool("verbose", false, "log telemetry")
		seed      = flag.Int64("seed", 0, "random seed for probabilistic events (0 = random each run)")
		watch     = flag.Bool("watch", true, "reload the theme and mapping files automatically when they change")
		duration  = flag.Duration("duration", 0, "stop after this much soundscape (e.g. 10m; 0 = run until stopped, or a recording ends). With a source that can be replayed (simulate, file) and only offline outputs (file, midi-file, console), it renders as fast as it can instead of in real time")
	)
	var outputs stringList
	flag.Var(&outputs, "output", "where the soundscape goes; repeat for several:"+output.Help())
	if err := checkRemovedFlags(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	flag.Parse()

	specs, err := output.ParseSpecs(outputs)
	if err != nil {
		log.Fatal(err)
	}

	if *sourceFlag != "" {
		if _, err := source.ParseSpec(*sourceFlag); err != nil {
			log.Fatal(err)
		}
	}
	if *aliases == "" {
		switch {
		case *sourceFlag != "":
			kind, _, _ := strings.Cut(*sourceFlag, ":")
			if kind == "file" {
				log.Fatalf("--source %s: say whose metrics the file holds with --aliases, e.g. --aliases fastly", *sourceFlag)
			}
			*aliases = kind
		case os.Getenv("FASTLY_SERVICE_ID") != "":
			*aliases = "fastly"
		default:
			*aliases = "simulate"
		}
	}
	mappingPath := mapping.Resolve(*aliases)

	m, err := loadMapping(mappingPath, *sourceFlag)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("source: %s, mapping: %s", m.Source, mappingPath)

	src, err := buildSource(m, *token, *verbose)
	if err != nil {
		log.Fatal(err)
	}

	th, err := theme.Load(*themePath)
	if err != nil {
		log.Fatal(err)
	}
	warnUnbound(m, th)

	// Render faster than real time when there's an end (a --duration, or
	// a recording that doesn't loop), the source can be stepped, and
	// nothing has to keep pace with the world. Everything then times
	// itself by a virtual clock that the render moves along.
	stepper, steppable := src.(source.Stepper)
	ender, _ := src.(source.Ender)
	ends := *duration > 0 || (ender != nil && ender.Ends())
	fast := ends && steppable && offline(specs)
	var clk clock.Clock = clock.Real{}
	var virtual *clock.Virtual
	if fast {
		virtual = clock.NewVirtual(time.Now())
		clk = virtual
	}

	outs, err := buildOutput(th, specs, outputOptions{
		soundFont: *soundFont,
		clock:     clk,
		seed:      *seed,
		start:     !fast,
	})
	if err != nil {
		log.Fatal(err)
	}
	closeOutput := outs.close
	defer closeOutput()

	var engine *theme.Engine
	if *seed != 0 {
		engine = theme.NewEngineWithSeed(th, outs.events, *seed)
	} else {
		engine = theme.NewEngine(th, outs.events)
	}
	engine.UseClock(clk)
	conditioner := mapping.NewConditioner(m)
	process := func(ts int64, raw map[string]float64) {
		outs.tick(ts, raw)
		engine.Process(ts, conditioner.Apply(raw))
	}

	if fast {
		renderFast(stepper, process, virtual, outs.mixer, *duration)
		return
	}

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

	if *watch {
		r := &reloader{
			themePath: *themePath, mappingPath: mappingPath, sourceOverride: *sourceFlag,
			engine: engine, conditioner: conditioner, player: outs.player,
			theme: th, mapping: m,
		}
		go watchFile(*themePath, r.reloadTheme)
		go watchFile(mappingPath, r.reloadMapping)
	}

	ctx := context.Background()
	if *duration > 0 {
		log.Printf("running for %v, in real time", *duration)
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *duration)
		defer cancel()
	}
	err = src.Run(ctx, process)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Fatal(err)
	}
}

// offline reports whether every output can take events as fast as they
// come, rather than as they'd happen.
func offline(specs []output.Spec) bool {
	for _, spec := range specs {
		switch spec.Kind {
		case "file", "midi-file", "console", "telemetry":
		default:
			return false
		}
	}
	return true
}

// renderFast renders duration of soundscape (0: until the source ends) as
// fast as it can, logging its progress, until it's done or Ctrl+C stops it
// early. The caller then finishes the outputs.
func renderFast(src source.Stepper, process func(int64, map[string]float64), clk *clock.Virtual, mixer *audio.Mixer, duration time.Duration) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	of := ""
	if duration > 0 {
		of = fmt.Sprintf(" of %v", duration)
		log.Printf("rendering %v, faster than real time", duration)
	} else {
		duration = 100 * 365 * 24 * time.Hour // the source will end first
		log.Printf("rendering to the end of the source, faster than real time")
	}
	began := time.Now()
	lastLog := began
	rendered := render.Run(ctx, render.Config{
		Source:     src,
		Process:    process,
		Clock:      clk,
		Mixer:      mixer,
		SampleRate: sampleRate,
		Duration:   duration,
		Progress: func(done time.Duration) {
			if time.Since(lastLog) >= 5*time.Second {
				lastLog = time.Now()
				log.Printf("rendered %v%s", done.Round(time.Second), of)
			}
		},
	})

	took := time.Since(began)
	speed := ""
	if took > 0 {
		speed = fmt.Sprintf(", %.0fx real time", rendered.Seconds()/took.Seconds())
	}
	if ctx.Err() != nil {
		log.Printf("stopped after rendering %v%s", rendered.Round(time.Second), of)
		return
	}
	if took < time.Second {
		took = took.Round(time.Millisecond)
	} else {
		took = took.Round(100 * time.Millisecond)
	}
	log.Printf("rendered %v in %v%s", rendered.Round(time.Second), took, speed)
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

// buildSource starts the source the mapping's (already validated) source
// spec names.
func buildSource(m mapping.Mapping, token string, verbose bool) (source.Source, error) {
	spec, err := source.ParseSpec(m.Source)
	if err != nil {
		return nil, err
	}

	switch spec.Kind {
	case "simulate":
		return &source.Simulation{}, nil

	case "fastly":
		service := spec.Options["service"]
		if service == "" {
			service = os.Getenv("FASTLY_SERVICE_ID")
		}
		if service == "" || token == "" {
			return nil, fmt.Errorf("the fastly source needs a service (fastly:service=SID or FASTLY_SERVICE_ID) and a token (--token or FASTLY_API_TOKEN)")
		}
		return fastly.Source{Client: fastly.NewClient(token, service), Verbose: verbose}, nil

	case "prometheus":
		url := spec.Target
		if url == "" {
			url = envOr("PROMETHEUS_URL", "http://localhost:9090")
		}
		interval := time.Second
		if v, ok := spec.Options["interval"]; ok {
			interval, err = time.ParseDuration(v)
			if err != nil || interval <= 0 {
				return nil, fmt.Errorf("prometheus interval %q isn't a duration such as 5s", v)
			}
		}
		return prometheus.Source{
			Client:   prometheus.NewClient(url),
			Queries:  m.Queries(),
			Interval: interval,
			Verbose:  verbose,
		}, nil

	case "wikipedia":
		var wikis []string
		for _, w := range strings.Split(spec.Options["wikis"], "+") {
			if w = strings.TrimSpace(w); w != "" {
				wikis = append(wikis, w)
			}
		}
		return wikipedia.Source{Wikis: wikis, Verbose: verbose}, nil

	case "file":
		loop := false
		if v, ok := spec.Options["loop"]; ok {
			loop, err = strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("file loop=%q: want true or false", v)
			}
		}
		recording, err := telemetry.Read(spec.Target, spec.Options["format"])
		if err != nil {
			return nil, err
		}
		log.Printf("file: %s: %s", spec.Target, recording.Describe())
		return &telemetry.Source{Recording: recording, Loop: loop, Verbose: verbose}, nil

	default:
		return nil, fmt.Errorf("source %q is not implemented yet", spec.Kind)
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

// removedFlags maps the flags --source and --output replaced to their new
// form.
var removedFlags = map[string]string{
	"simulate":            "--source simulate",
	"service-id":          "--source fastly:service=SID",
	"prometheus-url":      "--source prometheus:URL",
	"prometheus-interval": "--source prometheus:interval=5s",
	"wikipedia-wikis":     "--source wikipedia:wikis=enwiki+dewiki",
	"midi-out":            "--output midi-file:PATH",
	"midi-port":           "--output midi:PORT",
	"midi-virtual":        "--output midi-virtual:NAME",
	"osc":                 "--output osc:HOST:PORT",
	"osc-prefix":          "--output osc:HOST:PORT,prefix=/PREFIX",
	"sample-player":       "--output without speakers (e.g. just --output osc:HOST:PORT)",
}

// checkRemovedFlags points anyone using an old flag at its replacement.
func checkRemovedFlags(args []string) error {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		if replacement, ok := removedFlags[name]; ok {
			return fmt.Errorf("%s has been replaced: use %s", arg, replacement)
		}
	}
	return nil
}

// stringList is a flag that can be given more than once.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, " ") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

type outputOptions struct {
	soundFont string
	// clock times the outputs' notes: the wall clock, or a render's.
	clock clock.Clock
	seed  int64
	// start the mixer in time; a render steps it instead.
	start bool
}

// outputs is everything buildOutput started.
type outputs struct {
	// events takes the theme engine's events.
	events output.Output
	// player is the sample player, if any, for hot reloads to update.
	player *sampler.Player
	// mixer mixes the speakers and audio files, if there are any.
	mixer *audio.Mixer
	// tick records a tick's raw metrics, for --output telemetry.
	tick func(timestamp int64, metrics map[string]float64)
	// close finishes everything; it's safe to call more than once.
	close func()
}

// buildOutput starts the outputs the --output specs ask for. Speakers and
// files get one mix of the sample player, when the theme has samples, and
// the SoundFont synth, when one is given. Telemetry files get the raw
// metrics. The other kinds take events directly. With nothing that takes
// events (a note-only theme played to the speakers with no SoundFont, say)
// it falls back to printing them, so a theme can still be exercised with
// no audio at all.
func buildOutput(th theme.Theme, specs []output.Spec, opts outputOptions) (*outputs, error) {
	var outs []output.Output
	var closers []func()
	var player *sampler.Player
	var mixer *audio.Mixer
	soundFont := opts.soundFont

	// Close whatever was already started if a later output fails.
	fail := func(err error) (*outputs, error) {
		for _, c := range closers {
			c()
		}
		return nil, err
	}

	// Telemetry files are written from the source's goroutine and closed
	// from a signal handler's, so they share a lock.
	var telemetryMu sync.Mutex
	var recorders []*telemetry.Writer
	recordingTelemetry := false

	var audioSpecs []output.Spec
	for _, spec := range specs {
		switch spec.Kind {
		case "speakers":
			audioSpecs = append(audioSpecs, spec)

		case "file":
			if err := record.Check(spec.Target, sampleRate, spec.Options); err != nil {
				return fail(err)
			}
			audioSpecs = append(audioSpecs, spec)

		case "console":
			outs = append(outs, output.NewConsole())

		case "telemetry":
			recordingTelemetry = true
			w, err := telemetry.CreateWriter(spec.Target, spec.Options["format"])
			if err != nil {
				return fail(err)
			}
			log.Printf("recording telemetry to %s", spec.Target)
			recorders = append(recorders, w)
			path := spec.Target
			closers = append(closers, func() {
				telemetryMu.Lock()
				defer telemetryMu.Unlock()
				if err := w.Close(); err != nil {
					log.Printf("telemetry: %v", err)
					return
				}
				log.Printf("telemetry: wrote %s", path)
			})

		case "midi-file":
			path := spec.Target
			m := midi.NewVirtualOutput(path, opts.clock)
			outs = append(outs, m)
			closers = append(closers, func() {
				if err := m.Close(); err != nil {
					log.Printf("midi: failed to write %s: %v", path, err)
					return
				}
				log.Printf("midi: wrote %s", path)
			})

		case "midi", "midi-virtual":
			var live *midi.LiveOutput
			var err error
			if spec.Kind == "midi" {
				live, err = midi.OpenPort(spec.Target)
			} else {
				live, err = midi.OpenVirtual(spec.Target)
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

		case "osc":
			prefix := "/soundscape"
			if p, ok := spec.Options["prefix"]; ok {
				prefix = p
			}
			o, err := osc.Dial(spec.Target, prefix)
			if err != nil {
				return fail(err)
			}
			log.Printf("osc: sending to %s", o)
			outs = append(outs, o)
			closers = append(closers, func() { _ = o.Close() })
		}
	}

	if soundFont != "" && len(audioSpecs) == 0 {
		return fail(fmt.Errorf("--soundfont plays through the speakers or into a file, but there's no --output speakers or file"))
	}

	// The sample player and the SoundFont synth render into one mix for the
	// speakers and files. Without either, sample sounds are left to the
	// other outputs (another program might play them from OSC).
	groups := sampleGroups(th)
	if len(groups) == 0 && soundFont == "" {
		for _, spec := range audioSpecs {
			if spec.Kind == "file" {
				return fail(fmt.Errorf("--output file:%s: nothing to record: the theme has no sample sounds, and note sounds need --soundfont", spec.Target))
			}
		}
	}
	if len(audioSpecs) > 0 && (len(groups) > 0 || soundFont != "") {
		mixer = audio.NewMixer(sampleRate)

		if len(groups) > 0 {
			p := sampler.NewPlayer(sampleRate, opts.seed)
			for _, dir := range groups {
				if err := p.LoadGroup(dir, dir); err != nil {
					return fail(fmt.Errorf("loading sample group %s: %w", dir, err))
				}
			}
			player = p
			outs = append(outs, p)
			mixer.AddSource(p)
		}

		if soundFont != "" {
			sf, err := synth.NewSoundFontOutput(soundFont, sampleRate, opts.clock)
			if err != nil {
				return fail(fmt.Errorf("starting SoundFont output: %w", err))
			}
			outs = append(outs, sf)
			mixer.AddSource(sf)
		}

		// Sinks open last, so nothing above has to close them on failure.
		// Without speakers, the mixer keeps time by the clock.
		var files []string
		for _, spec := range audioSpecs {
			var sink audio.Sink
			var err error
			switch spec.Kind {
			case "speakers":
				sink, err = audio.NewSpeakers(sampleRate)
				if err != nil {
					err = fmt.Errorf("opening the audio device: %w", err)
				}
			case "file":
				sink, err = record.Create(spec.Target, sampleRate, spec.Options)
				files = append(files, spec.Target)
				if err == nil {
					log.Printf("recording to %s", spec.Target)
				}
			}
			if err != nil {
				_ = mixer.Close()
				return fail(err)
			}
			mixer.AddSink(sink)
		}

		if opts.start {
			mixer.Start()
		}
		closers = append(closers, func() {
			if err := mixer.Close(); err != nil {
				log.Printf("audio: %v", err)
			}
			for _, f := range files {
				log.Printf("wrote %s", f)
			}
		})
	}

	var events output.Output
	switch {
	case len(outs) == 0 && recordingTelemetry:
		events = output.Discard{} // just recording telemetry
	case len(outs) == 0:
		events = output.NewConsole()
	case len(outs) == 1:
		events = outs[0]
	default:
		events = output.NewMulti(outs...)
	}

	// A recorder that fails stops, with a message, and the rest carry on.
	failed := make(map[*telemetry.Writer]bool)
	tick := func(ts int64, raw map[string]float64) {
		telemetryMu.Lock()
		defer telemetryMu.Unlock()
		for _, w := range recorders {
			if failed[w] {
				continue
			}
			if err := w.Write(ts, raw); err != nil {
				log.Printf("telemetry: %v (stopped recording it)", err)
				failed[w] = true
			}
		}
	}

	var once sync.Once
	closeAll := func() {
		once.Do(func() {
			for _, c := range closers {
				c()
			}
		})
	}

	return &outputs{events: events, player: player, mixer: mixer, tick: tick, close: closeAll}, nil
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
	if strings.HasPrefix(m.Source, "prometheus") && !sameQueries(m, r.mapping) {
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
