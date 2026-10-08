# Soundscape

A Go-based generative soundscape driven by live telemetry. Busy systems sound like a busy forest
(or market, or road): more traffic, more birdsong; more data flowing, a more turbulent river.

    data source (simulation, Fastly, Prometheus, Wikipedia edits, ...)
          |
          v
    mapping: source metrics -> named theme inputs (smoothing, normalisation to 0..1)
          |
          v
    theme engine: inputs -> abstract sound events (Poisson-scheduled)
          |
          +----> console output (text, no audio)
          |
          +----> WAV sample player -> speakers (self-contained, no extra files needed)
          |
          +----> SoundFont synth -> speakers (optional, needs an .sf2 file)
          |
          +----> MIDI -> a synth or DAW, live, and/or a .mid file (optional)

Themes say what things sound like, and mappings say what data drives them, so any theme can be
played from any source. Both are YAML, not Go code.

## Quick start

Download a prebuilt release for Linux x86-64 or macOS on Apple silicon from the
[releases page](https://github.com/simonwistow/soundscape/releases), unpack it, and run
`./soundscape` from inside the unpacked directory (it finds `themes/` and `mappings/` there). The
Linux build needs Ubuntu 24.04 or later (glibc 2.39) and the ALSA library (`libasound2`, packaged
as `libasound2t64` on Ubuntu 24.04+).

Or build from source, with Go 1.24+ and a C and C++ compiler (on macOS, the Xcode command line
tools; on Linux, also the ALSA headers: `libasound2-dev`). The examples below use `go run`:

Run it. With no other options this plays the forest theme from a built-in simulation that
cycles slowly between quiet and busy:

    go run ./cmd/soundscape

Listen to Wikipedia being edited, live (no account needed):

    go run ./cmd/soundscape --aliases wikipedia

Try the other example themes:

    go run ./cmd/soundscape --aliases wikipedia --theme themes/market/theme.yaml
    go run ./cmd/soundscape --aliases wikipedia --theme themes/road/theme.yaml

The `pentatonic` theme plays MIDI notes rather than recordings, so it needs a synth to play them;
see [MIDI](#midi) below.

## Data sources

The `--aliases` flag picks a mapping: a bare name means `mappings/<name>.yaml`, or give a path.
The mapping names its source; `--source` overrides that (e.g. `--source simulate` to play a
mapping against simulated data), and `--simulate` is shorthand for `--source simulate`.

| Source       | Mapping                    | Needs                                                  |
|--------------|----------------------------|--------------------------------------------------------|
| `simulate`   | `mappings/simulate.yaml`   | nothing                                                |
| `wikipedia`  | `mappings/wikipedia.yaml`  | internet access; optionally `--wikipedia-wikis enwiki` |
| `prometheus` | `mappings/prometheus.yaml` | a Prometheus server: `--prometheus-url` / `PROMETHEUS_URL` (default `http://localhost:9090`) |
| `fastly`     | `mappings/fastly.yaml`     | `FASTLY_API_TOKEN` and `--service-id` / `FASTLY_SERVICE_ID` |

With no `--aliases`, the mapping defaults to the one named after `--source` (so
`--source wikipedia` uses `mappings/wikipedia.yaml`); with neither, a configured Fastly service ID
selects `fastly`, and otherwise the simulation is used.

* **simulate**: deterministic synthetic telemetry, for developing themes without any data.
* **wikipedia**: Wikimedia's public [recent-changes stream](https://stream.wikimedia.org/), with
  every edit, page creation and log action across Wikipedia and its sister projects, counted into
  per-second rates (`edits`, `new_pages`, `human_edits`, `bot_edits`, `bytes_changed`,
  `log_delete`, ...; see `internal/wikipedia`).
* **prometheus**: polls PromQL queries. Each input in the mapping gives a `query` that reduces to
  one number. The example mapping queries Prometheus's own HTTP metrics, so it works against a
  bare Prometheus. Copy it and point the queries at your own metrics.
* **fastly**: Fastly's real-time analytics API
  (`https://rt.fastly.com/v1/channel/<service_id>/ts/<timestamp>`), one-second records.

## Themes and mappings

A theme's sounds each follow an **input**, a named value from 0 to 1. A mapping says where each
input comes from and how it's scaled:

    # themes/forest/theme.yaml          # mappings/wikipedia.yaml
    sounds:                             source: wikipedia
      - name: birds                     inputs:
        type: probabilistic               activity:
        input: activity                     metric: edits
        ...                                 smoothing: 8
                                            normalise: { min: 1, max: 40 }

Input names are free, but the bundled themes and mappings use a conventional set, so they
interoperate:

| Input      | Meaning                    | Fastly           | Wikipedia       | Forest      | Market       | Road |
|------------|----------------------------|------------------|-----------------|-------------|--------------|------|
| `activity` | how much is happening      | `requests`       | `edits`         | birdsong    | crowd, chatter | traffic, quiet street to highway |
| `flow`     | how much is moving through | `resp_body_bytes`| `bytes_changed` | river, splashes | accordion | motorbikes |
| `trouble`  | things going wrong         | `errors`         | `log_delete`    | –           | dropped bottles | tyre screeches |
| `quirks`   | minor oddities             | `all_status_4xx` | `new_pages`     | woodpecker  | –            | car horns |

The `pentatonic` theme plays a melody on `activity`, a bass line on `flow`, and notes from
outside its scale on `trouble`.

An input the mapping doesn't provide reads as 0 (with a warning at startup).

* In a mapping, `smoothing` is a time constant in seconds, and `normalise` maps the raw value
  logarithmically into 0..1.
* In a theme, `type: probabilistic` turns an input into a Poisson-scheduled event rate, and
  `type: continuous` ramps it into `min_value..max_value`.
* `output` selects how that's realised: `note`/`cc` (MIDI, played live, through a SoundFont, or
  recorded to a file) or
  `sample`/`sample_loop` (WAV playback, via the sample player). See DEVELOPMENT.md for the full
  vocabulary, including how two `sample_loop` sounds crossfade.

A theme is a directory: `theme.yaml` plus a `samples/` folder, so it's self-contained wherever
it's run from (`sample_group` paths resolve relative to the theme file).

## MIDI

A theme's `note`/`cc`-output sounds can be sent, live, to anything that plays MIDI: a hardware
synth, a DAW, or a software instrument. No hardware is needed.

Create a virtual MIDI port called `soundscape`, which other software sees as a MIDI input:

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --midi-virtual soundscape

On macOS, GarageBand plays it with no setup: create an empty project with a Software Instrument
track, and it plays whatever arrives on any MIDI input. On Linux, connect the port to a synth such
as FluidSynth with `aconnect` (or watch the raw messages with `aseqdump`). Windows has no virtual
ports.

Or send to a MIDI output port that already exists, such as a synth or macOS's IAC Driver (enable
it in Audio MIDI Setup). A name that matches no port exactly can be part of one name, so `IAC`
finds `IAC Driver Bus 1`:

    go run ./cmd/soundscape midi-ports
    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --midi-port IAC

Each sound's `channel` (0-15) becomes its MIDI channel (1-16 in most software), so a DAW can give
each sound its own instrument.

Play the same sounds through a SoundFont, with no other software:

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --soundfont /path/to/your.sf2

Or record them as a Standard MIDI File (finalized on exit, including Ctrl+C):

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --midi-out session.mid

Live MIDI, a SoundFont and a MIDI file can all be used at once, alongside a theme's samples.

## More options

Pin the random seed for a reproducible run:

    go run ./cmd/soundscape --seed 42

Log the raw metrics as they arrive:

    go run ./cmd/soundscape --aliases wikipedia --verbose

Validate a theme, and optionally check it against a mapping. This catches missing or duplicate
names, inverted ranges, out-of-range MIDI values, missing sample directories, unknown behaviour
or output kinds, and theme inputs the mapping doesn't provide:

    go run ./cmd/soundscape validate --aliases wikipedia themes/forest/theme.yaml

Edit the theme or mapping while a run is going and it hot-reloads automatically. Files are
checked once a second and validated before being applied, so an invalid edit is logged and
ignored rather than crashing or going silent. Tuning a mapping's `normalise` ranges against live
data this way is the quickest route to a good-sounding setup. Switching a mapping's source, or
changing Prometheus queries, needs a restart. Disable with `--watch=false`.

## Current state

* four data sources: simulation, Wikipedia live edits, Prometheus, and Fastly real-time analytics
* source-independent themes: mappings bind source metrics to theme inputs, with exponential
  smoothing and log normalisation
* a Poisson event scheduler: a sound's `rate` is an expected events/second, not a per-tick
  probability, so a tick can naturally produce zero, one, or several events
* an abstract sound-event model (`event.Note`/`event.Sample`/`event.Control`) so the theme engine
  never depends on a specific output backend
* a real WAV sample player (`internal/sampler`): pitch shifting, velocity/pan, looping, click-free
  envelopes, voice stealing, and gain-smoothed crossfades between looping layers
* three complete example themes, `forest-glade`, `farmers-market` and `busy-road`, using real
  recordings (see ATTRIBUTION.md), so no SoundFont or external assets are needed to hear something,
  and a fourth, `pentatonic`, that plays MIDI notes
* an optional SoundFont backend (Go-MeltySynth) for `note`/`cc`-output sounds
* optional MIDI output for `note`/`cc`-output sounds: live, to a MIDI port or a virtual port of
  its own (RtMidi), and/or recorded as a Standard MIDI File
* console mode for development without audio

`cmd/gensamples` procedurally synthesizes placeholder sample assets (tone sweeps for chirps,
filtered noise for rivers/crowds/splashes), which is useful for bootstrapping a new theme before
real recordings are ready:

    go run ./cmd/gensamples --theme forest   # writes to themes/forest/samples
    go run ./cmd/gensamples --theme market   # writes to themes/market/samples

## Architecture

    internal/source      Source interface + the built-in simulation
    internal/wikipedia   Wikimedia recent-changes stream source
    internal/prometheus  Prometheus query source
    internal/fastly      Fastly real-time API source
    internal/mapping     mapping files: source metrics -> conditioned theme inputs
    internal/metrics     smoothing and normalisation
    internal/theme       YAML theme + behaviour engine
    internal/scheduler   Poisson event-rate scheduler
    internal/event       abstract sound-event model (Note/Sample/Control)
    internal/output      sound output abstraction (incl. Multi fan-out)
    internal/audio       shared renderer->Oto ring-buffer plumbing
    internal/synth       SoundFont output backend
    internal/sampler     WAV sample-player output backend
    internal/midi        MIDI output backends: live (RtMidi) and Standard MIDI File
    cmd/soundscape       CLI
    cmd/gensamples       placeholder sample asset generator
    internal/cmd/changelog  CHANGELOG.md linting and release tooling (see RELEASING.md)
    mappings/            bundled mappings, one per source
    themes/              bundled themes

A `source.Source` just emits a timestamped `map[string]float64` each tick; a
`mapping.Conditioner` turns that into theme inputs; and the theme engine only depends on
`internal/event` and `internal/output`'s `Output` interface, never on a concrete source or
backend. To add a source, implement `Run(ctx, emit)` and add a case to `cmd/soundscape`'s
`buildSource`.

`cmd/soundscape` picks output backends automatically:

* the sample player starts whenever the theme references any `sample_group`, with no flag needed
* the SoundFont backend starts if `--soundfont` is given
* the MIDI file backend starts if `--midi-out` is given
* live MIDI output starts if `--midi-port` or `--midi-virtual` is given

Any combination of these can run at once via `output.Multi`. With none, it falls back to the
text-only Console backend.

## Next steps

1. Add richer stochastic actors such as flocks, crowds and weather.
2. Add MIDI 2.0/OSC output.
3. Embed themes, mappings and optionally assets into a single distributable binary.

## License

Soundscape is free software, licensed under the GNU General Public License version 3; see
[LICENSE](LICENSE). The recordings under `themes/*/samples/` are not covered by it: each keeps
its own license (public domain, CC0 or, for one file, CC BY-SA 3.0), as listed in
[ATTRIBUTION.md](ATTRIBUTION.md).
