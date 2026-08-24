package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"example.com/fastly-soundscape/internal/fastly"
	"example.com/fastly-soundscape/internal/output"
	"example.com/fastly-soundscape/internal/sampler"
	"example.com/fastly-soundscape/internal/synth"
	"example.com/fastly-soundscape/internal/theme"
)

const sampleRate = 44100

func main() {
	if len(os.Args) > 1 && os.Args[1] == "validate" {
		runValidate(os.Args[2:])
		return
	}

	var (
		serviceID = flag.String("service-id", os.Getenv("FASTLY_SERVICE_ID"), "Fastly service ID")
		token     = flag.String("token", os.Getenv("FASTLY_API_TOKEN"), "Fastly API token")
		themePath = flag.String("theme", "themes/forest/theme.yaml", "theme YAML file")
		soundFont = flag.String("soundfont", "", "optional SF2 SoundFont, for note/cc-output sounds")
		simulate  = flag.Bool("simulate", false, "use generated telemetry instead of Fastly")
		verbose   = flag.Bool("verbose", false, "log telemetry")
		seed      = flag.Int64("seed", 0, "random seed for probabilistic events (0 = random each run)")
		watch     = flag.Bool("watch", true, "reload the theme file automatically when it changes")
	)
	flag.Parse()

	th, err := theme.Load(*themePath)
	if err != nil {
		log.Fatal(err)
	}

	out, player, closeOutput, err := buildOutput(th, *soundFont)
	if err != nil {
		log.Fatal(err)
	}
	defer closeOutput()

	var engine *theme.Engine
	if *seed != 0 {
		engine = theme.NewEngineWithSeed(th, out, *seed)
	} else {
		engine = theme.NewEngine(th, out)
	}

	if *watch {
		go watchTheme(*themePath, engine, player)
	}

	if *simulate {
		runSimulation(engine)
		return
	}

	if *serviceID == "" || *token == "" {
		log.Fatal("FASTLY_API_TOKEN and --service-id are required unless --simulate is used")
	}

	client := fastly.NewClient(*token, *serviceID)

	var timestamp int64
	for {
		resp, err := client.Fetch(timestamp)
		if err != nil {
			log.Printf("Fastly: %v", err)
			time.Sleep(time.Second)
			continue
		}

		if *verbose {
			fmt.Printf("timestamp=%d records=%d delay=%ds\n",
				resp.Timestamp, len(resp.Data), resp.AggregateDelay)
		}

		for _, record := range resp.Data {
			engine.Process(record.Recorded, record.Aggregated)
		}

		timestamp = resp.Timestamp
		time.Sleep(200 * time.Millisecond)
	}
}

// runValidate implements `soundscape validate <theme.yaml>`: load the theme
// and report every problem theme.Validate finds, exiting non-zero if any.
func runValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: soundscape validate <theme.yaml>")
		os.Exit(2)
	}
	path := fs.Arg(0)

	th, err := theme.Load(path)
	if err != nil {
		fmt.Printf("%s: failed to load: %v\n", path, err)
		os.Exit(1)
	}

	issues := theme.Validate(th)
	if len(issues) == 0 {
		fmt.Printf("%s: OK (%d sources, %d sounds)\n", path, len(th.Sources), len(th.Sounds))
		return
	}

	fmt.Printf("%s: %d problem(s) found\n", path, len(issues))
	for _, issue := range issues {
		fmt.Printf("  - %v\n", issue)
	}
	os.Exit(1)
}

// buildOutput assembles whichever output backends the theme and flags call
// for: the WAV sample player is started automatically whenever the theme
// references any sample_group (no flag needed, so a sample-only theme is
// self-contained), and the SoundFont backend is added if --soundfont is
// given. A theme can use both at once (e.g. sampled birds alongside a
// SoundFont-driven instrument). With neither, falls back to the Console
// backend so the theme can still be exercised with no audio at all.
func buildOutput(th theme.Theme, soundFontPath string) (output.Output, *sampler.Player, func(), error) {
	var outs []output.Output
	var closers []func()
	var player *sampler.Player

	if groups := sampleGroups(th); len(groups) > 0 {
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

	if soundFontPath != "" {
		sf, err := synth.NewSoundFontOutput(soundFontPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("starting SoundFont output: %w", err)
		}
		outs = append(outs, sf)
		closers = append(closers, sf.Close)
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

// watchTheme polls path for modifications and hot-reloads it into engine
// when it changes: the theme is re-loaded and validated, any sample groups
// it references are (re)loaded into player (if the theme uses one), and
// only if all of that succeeds is it applied. An invalid or broken edit is
// logged and ignored, leaving the previous theme running rather than
// crashing or going silent mid-installation.
func watchTheme(path string, engine *theme.Engine, player *sampler.Player) {
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

		newTheme, err := theme.Load(path)
		if err != nil {
			log.Printf("hot reload: %s: %v (keeping previous theme)", path, err)
			continue
		}

		if issues := theme.Validate(newTheme); len(issues) > 0 {
			log.Printf("hot reload: %s: %d problem(s) found, keeping previous theme:", path, len(issues))
			for _, issue := range issues {
				log.Printf("  - %v", issue)
			}
			continue
		}

		if player != nil {
			if err := reloadSampleGroups(player, newTheme); err != nil {
				log.Printf("hot reload: %s: %v (keeping previous theme)", path, err)
				continue
			}
		}

		engine.Reload(newTheme)
		log.Printf("hot reload: %s: reloaded (%d sources, %d sounds)", path, len(newTheme.Sources), len(newTheme.Sounds))
	}
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

func runSimulation(engine *theme.Engine) {
	// A deliberately slow cycle: quiet -> busy -> quiet.
	var t float64
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		requests := 50.0 + (1+math.Sin(t))*4500.0
		bandwidth := requests * (1000 + 8000*(1+math.Sin(t*0.7))/2)
		errors := 1.0 + 10.0*(1+math.Sin(t*1.7))/2

		engine.Process(int64(time.Now().Unix()), map[string]float64{
			"requests":        requests,
			"resp_body_bytes": bandwidth,
			"errors":          errors,
			"hits":            requests * 0.85,
		})

		t += 0.12
	}
}
