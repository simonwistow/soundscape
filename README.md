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
* In a theme, `type: probabilistic` turns an input into a Poisson-scheduled event rate,
  `type: continuous` ramps it into `min_value..max_value`, `type: flock` sends groups of
  callers over, which swell in, peak overhead and fade away as they cross from one side to the
  other, and `type: crowd` gathers people who come and go with the input, chat, and now and then
  all react at once.
* A theme's `weather` makes an input of its own from one of the mapping's, with a slow life of
  its own: a storm that builds up, gusts, and takes its time to clear. Sounds use it like any other
  input; the forest's rain and thunder follow a storm driven by `trouble`.
* `output` selects how that's realised: `note`/`cc` (MIDI, played live, through a SoundFont, or
  recorded to a file) or
  `sample`/`sample_loop` (WAV playback, via the sample player). See DEVELOPMENT.md for the full
  vocabulary, including how two `sample_loop` sounds crossfade.

A theme is a directory: `theme.yaml` plus a `samples/` folder, so it's self-contained wherever
it's run from (`sample_group` paths resolve relative to the theme file).

## Outputs

`--output` says where the soundscape goes. With none it plays through the speakers; give it more
than once to send it to several places at once:

| `--output` | |
|---|---|
| `speakers` | plays sample sounds, and note sounds with `--soundfont`, through the audio device |
| `midi:PORT` | sends note and cc sounds to a MIDI port, live |
| `midi-virtual:NAME` | creates a virtual MIDI port and sends note and cc sounds to it, live |
| `midi-file:PATH` | writes note and cc sounds to a Standard MIDI File |
| `osc:HOST:PORT[,prefix=/PREFIX]` | sends every event as an OSC message over UDP |
| `console` | prints every event |

Options follow the target after commas, as `key=value`. Without `speakers` (say, with just
`--output osc:...`) nothing is played here, so another program can play the samples instead.

## MIDI

A theme's `note`/`cc`-output sounds can be sent, live, to anything that plays MIDI: a hardware
synth, a DAW, or a software instrument. No hardware is needed.

Create a virtual MIDI port called `soundscape`, which other software sees as a MIDI input:

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --output midi-virtual:soundscape

On macOS, GarageBand plays it with no setup: create an empty project with a Software Instrument
track, and it plays whatever arrives on any MIDI input. On Linux, connect the port to a synth such
as FluidSynth with `aconnect` (or watch the raw messages with `aseqdump`). Windows has no virtual
ports.

Or send to a MIDI output port that already exists, such as a synth or macOS's IAC Driver (enable
it in Audio MIDI Setup). A name that matches no port exactly can be part of one name, so `IAC`
finds `IAC Driver Bus 1`:

    go run ./cmd/soundscape midi-ports
    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --output midi:IAC

Each sound's `channel` (0-15) becomes its MIDI channel (1-16 in most software), so a DAW can give
each sound its own instrument.

Play the same sounds through a SoundFont, with no other software. Any General MIDI SoundFont
will do, such as the freely licensed
[GeneralUser GS](https://github.com/mrbumpy409/GeneralUser-GS):

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --soundfont /path/to/your.sf2

A sound's `program` picks its instrument, on a SoundFont or on any General MIDI synth or DAW that
follows program changes (GarageBand doesn't). In General MIDI, 11 is a vibraphone, 32 an acoustic
bass, and so on; list the instruments in a SoundFont, with the `bank` and `program` that select
each, with:

    go run ./cmd/soundscape soundfont-presets /path/to/your.sf2

A program belongs to a MIDI channel, so sounds that share a channel share an instrument.

Or record them as a Standard MIDI File (finalized on exit, including Ctrl+C):

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --output midi-file:session.mid

Live MIDI, a SoundFont and a MIDI file can all be used at once, alongside a theme's samples:
the SoundFont plays through `--output speakers`, the default, so add it back when giving others:

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --soundfont /path/to/your.sf2 \
        --output speakers --output midi-file:session.mid

## OSC

Every event, from every kind of sound, can be sent as an [Open Sound Control](https://opensoundcontrol.stanford.edu/)
message over UDP, for SuperCollider, Max, Pure Data, TouchDesigner or anything else that speaks
OSC to play, or react to, as it likes:

    go run ./cmd/soundscape --theme themes/forest/theme.yaml --output osc:localhost:57120

Each message is addressed by the sound that made it, so a receiver can pick sounds out with one
pattern match:

| Address | Arguments |
|---|---|
| `/soundscape/<sound>/note` | `i` pitch, `i` velocity, `i` duration_ms, `i` channel |
| `/soundscape/<sound>/sample` | `s` group, `f` pitch, `f` gain, `f` pan, `i` duration_ms, `i` channel |
| `/soundscape/<sound>/loop` | `s` group, `f` pitch, `f` gain, `f` pan, `i` channel |
| `/soundscape/<sound>/control` | `f` value, `i` controller, `i` channel |
| `/soundscape/program` | `i` channel, `i` bank, `i` program |

A sample's group is its `sample_group` directory's name (e.g. `birds`) and its pitch a playback
ratio (1 is as recorded); pan runs from -1 (left) to 1 (right). A loop's message comes every tick
while it plays, with its current gain, so it doubles as a continuous control. The `prefix` option
changes `/soundscape`, e.g. `--output osc:localhost:57120,prefix=/forest`.

With just `--output osc:...`, the samples are left to the receiver. To hear them through the
speakers as well, add `--output speakers`.

`examples/osc/supercollider.scd` is a receiver to start from: it plays a theme's samples in
SuperCollider, from the OSC messages, much as the built-in player does.

    /Applications/SuperCollider.app/Contents/MacOS/sclang examples/osc/supercollider.scd themes/forest/samples
    go run ./cmd/soundscape --theme themes/forest/theme.yaml --output osc:localhost:57120

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
    internal/audio       the mixer: sources (sampler, synth) into sinks (speakers)
    internal/synth       SoundFont output backend
    internal/sampler     WAV sample-player output backend
    internal/midi        MIDI output backends: live (RtMidi) and Standard MIDI File
    internal/osc         OSC output over UDP
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

`cmd/soundscape` starts a backend for each `--output` (parsed by `output.ParseSpecs`). The
speakers get one `audio.Mixer`: the sample player, whenever the theme references any
`sample_group`, and the SoundFont synth, if `--soundfont` is given, render into it. Every
backend that takes events runs at once via `output.Multi`. With none, it falls back to the
text-only Console backend.

## Next steps

1. Add more stochastic actors alongside flocks, crowds and weather.
2. Add MIDI 2.0 output, once synths support it more widely.
3. Embed themes, mappings and optionally assets into a single distributable binary.

## License

Soundscape is free software, licensed under the GNU General Public License version 3; see
[LICENSE](LICENSE). The recordings under `themes/*/samples/` are not covered by it: each keeps
its own license (public domain, CC0 or, for one file, CC BY-SA 3.0), as listed in
[ATTRIBUTION.md](ATTRIBUTION.md).
