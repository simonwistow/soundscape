# Fastly Soundscape

A Go-based generative soundscape driven by Fastly real-time analytics.

    Fastly realtime API
          |
          v
    Metrics -> smoothing/normalisation
          |
          v
    Theme engine -> abstract sound events (Poisson-scheduled)
          |
          +----> console output (text, no audio)
          |
          +----> WAV sample player -> speakers (self-contained, no extra files needed)
          |
          +----> SoundFont synth -> speakers (optional, needs an .sf2 file)
          |
          +----> virtual MIDI -> a .mid file (optional; no live device yet)

The theme is YAML rather than Go code.

## Current state

* Fastly real-time analytics polling
* `requests`, `resp_body_bytes`, `errors`, and `hits`
* exponential smoothing, configurable normalisation
* a Poisson event scheduler: a sound's `rate` is an expected events/second, not a per-tick
  probability, so a tick can naturally produce zero, one, or several events
* an abstract sound-event model (`event.Note`/`event.Sample`/`event.Control`) so the theme engine
  never depends on a specific output backend
* a real WAV sample player (`internal/sampler`): pitch shifting, velocity/pan, looping, click-free
  envelopes, voice stealing, and gain-smoothed crossfades between looping layers
* two complete example themes, `forest-glade` and `farmers-market`, both playing entirely through
  procedurally-generated placeholder samples (`cmd/gensamples`) — no SoundFont or external assets
  required to hear something
* an optional SoundFont backend (Go-MeltySynth) for `note`/`cc`-output sounds
* an optional virtual MIDI backend: writes `note`/`cc`-output sounds as a real, playable Standard
  MIDI File — no live device support yet (that needs cgo; see `internal/midi`'s package doc)
* console mode for development without audio
* deterministic simulation mode so a theme can be developed without a Fastly account

## Quick start

Install Go 1.23+.

Run the simulation — this alone produces audio, no flags needed:

    go run ./cmd/soundscape --simulate

Try the other example theme:

    go run ./cmd/soundscape --simulate --theme themes/market/theme.yaml

Run against Fastly:

    FASTLY_API_TOKEN=... \
    go run ./cmd/soundscape --service-id YOUR_SERVICE_ID

Also drive a SoundFont for any `note`/`cc`-output sounds in the theme:

    go run ./cmd/soundscape --simulate --soundfont /path/to/your.sf2

Or capture those same sounds as a Standard MIDI File (finalized on exit, including Ctrl+C):

    go run ./cmd/soundscape --simulate --midi-out session.mid

Pin the random seed for a reproducible run:

    go run ./cmd/soundscape --simulate --seed 42

Validate a theme (missing/duplicate names, unknown sources, inverted ranges, out-of-range MIDI
values, missing sample directories, unknown behaviour/output kinds):

    go run ./cmd/soundscape validate themes/forest/theme.yaml

Edit `theme.yaml` while a run is going and it hot-reloads automatically (checked once a second,
validated before being applied — an invalid edit is logged and ignored rather than crashing or
going silent). Disable with `--watch=false`.

## Fastly API

The client uses the real-time analytics endpoint:

    https://rt.fastly.com/v1/channel/<service_id>/ts/<timestamp>

The API reports one-second records and returns a `Timestamp` to use for the next request.

## Theme format

See `themes/forest/theme.yaml` and `themes/market/theme.yaml`. A theme is a directory: `theme.yaml`
plus a `samples/` folder, so it's self-contained wherever it's run from (`sample_group` paths
resolve relative to the theme file).

* `source` describes a Fastly metric: `smoothing` controls temporal smoothing, `normalise` maps
  it into 0..1.
* `type: probabilistic` turns a value into a Poisson-scheduled event rate; `type: continuous`
  ramps it into `min_value..max_value`.
* `output` selects how that's realised: `note`/`cc` (MIDI, via the SoundFont backend) or
  `sample`/`sample_loop` (WAV playback, via the sample player). See DEVELOPMENT.md for the full
  vocabulary, including how two `sample_loop` sounds crossfade.

`cmd/gensamples` procedurally synthesizes placeholder sample assets (tone sweeps for chirps,
filtered noise for rivers/crowds/splashes) — useful for bootstrapping a new theme before real
recordings are ready:

    go run ./cmd/gensamples --theme forest   # writes to themes/forest/samples
    go run ./cmd/gensamples --theme market   # writes to themes/market/samples

The theme engine is intended to grow a small vocabulary of reusable behaviours rather than requiring a Go plugin for every theme.

## Architecture

    internal/fastly     Fastly API client
    internal/metrics    smoothing and normalisation
    internal/theme      YAML theme + behaviour engine
    internal/scheduler  Poisson event-rate scheduler
    internal/event      abstract sound-event model (Note/Sample/Control)
    internal/output     sound output abstraction (incl. Multi fan-out)
    internal/audio      shared renderer->Oto ring-buffer plumbing
    internal/synth      SoundFont output backend
    internal/sampler    WAV sample-player output backend
    internal/midi       virtual (file-based) MIDI output backend
    cmd/soundscape      CLI
    cmd/gensamples      placeholder sample asset generator

The theme engine only depends on `internal/event` and `internal/output`'s `Output` interface,
never on a concrete backend. `cmd/soundscape` picks backends automatically: the sample player
starts whenever the theme references any `sample_group` (no flag needed), the SoundFont backend
starts if `--soundfont` is given, the MIDI file backend starts if `--midi-out` is given, and any
combination of these can run at once via `output.Multi`; with none, it falls back to the
text-only Console backend.

## Next steps

1. Add a live MIDI output backend (cgo + RtMidi, behind a build tag — see `internal/midi`'s
   package doc for why that's a bigger step than everything else here).
2. Add richer stochastic actors such as flocks, crowds and weather.
3. Add MIDI 2.0/OSC output.
4. Embed themes and optionally assets into a single distributable binary.
5. Replace the procedurally-generated placeholder samples with real recordings.
