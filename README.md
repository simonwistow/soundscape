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
* the `forest-glade` theme plays entirely through procedurally-generated placeholder samples
  (`cmd/gensamples`) — no SoundFont or external assets required to hear something
* an optional SoundFont backend (Go-MeltySynth) for `note`/`cc`-output sounds
* console mode for development without audio
* deterministic simulation mode so a theme can be developed without a Fastly account

## Quick start

Install Go 1.23+.

Run the simulation — this alone produces audio, no flags needed:

    go run ./cmd/soundscape --simulate

Run against Fastly:

    FASTLY_API_TOKEN=... \
    go run ./cmd/soundscape --service-id YOUR_SERVICE_ID

Also drive a SoundFont for any `note`/`cc`-output sounds in the theme:

    go run ./cmd/soundscape --simulate --soundfont /path/to/your.sf2

Pin the random seed for a reproducible run:

    go run ./cmd/soundscape --simulate --seed 42

## Fastly API

The client uses the real-time analytics endpoint:

    https://rt.fastly.com/v1/channel/<service_id>/ts/<timestamp>

The API reports one-second records and returns a `Timestamp` to use for the next request.

## Theme format

See `themes/forest/theme.yaml`. A theme is a directory: `theme.yaml` plus a `samples/` folder,
so it's self-contained wherever it's run from (`sample_group` paths resolve relative to the
theme file).

* `source` describes a Fastly metric: `smoothing` controls temporal smoothing, `normalise` maps
  it into 0..1.
* `type: probabilistic` turns a value into a Poisson-scheduled event rate; `type: continuous`
  ramps it into `min_value..max_value`.
* `output` selects how that's realised: `note`/`cc` (MIDI, via the SoundFont backend) or
  `sample`/`sample_loop` (WAV playback, via the sample player). See DEVELOPMENT.md for the full
  vocabulary, including how two `sample_loop` sounds crossfade.

`cmd/gensamples` procedurally synthesizes placeholder sample assets (tone sweeps for chirps,
filtered noise for rivers/splashes) — useful for bootstrapping a new theme before real recordings
are ready:

    go run ./cmd/gensamples --out themes/forest/samples

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
    cmd/soundscape      CLI
    cmd/gensamples      placeholder sample asset generator

The theme engine only depends on `internal/event` and `internal/output`'s `Output` interface,
never on a concrete backend. `cmd/soundscape` picks backends automatically: the sample player
starts whenever the theme references any `sample_group` (no flag needed), the SoundFont backend
starts if `--soundfont` is given, and both can run at once via `output.Multi`; with neither, it
falls back to the text-only Console backend.

## Next steps

1. Add a real MIDI output backend using RtMidi.
2. Add richer stochastic actors such as flocks, crowds and weather.
3. Add hot theme reload.
4. Add MIDI 2.0/OSC output.
5. Embed themes and optionally assets into a single distributable binary.
6. Replace the procedurally-generated placeholder samples with real recordings.
