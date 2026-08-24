# Fastly Soundscape

A first prototype of a Go-based generative soundscape driven by Fastly real-time analytics.

The prototype has four layers:

    Fastly realtime API
          |
          v
    Metrics -> smoothing/normalisation
          |
          v
    Theme engine -> probabilistic events
          |
          +----> console output
          |
          +----> SoundFont synth -> speakers

The theme is YAML rather than Go code.

## Current prototype

The first version deliberately keeps the scope small:

* Fastly real-time analytics polling
* `requests`, `resp_body_bytes`, `errors`, and `hits`
* exponential smoothing
* configurable normalisation
* probabilistic bird events
* continuous river-like control represented as MIDI CC
* optional SoundFont playback using Go-MeltySynth
* console mode for development without audio
* deterministic simulation mode so the theme can be developed without a Fastly account

The SoundFont engine is an interchangeable output backend. The same events can later be sent to external hardware/DAWs through a MIDI backend. The SoundFont path is already wired for MIDI notes and CC messages.

## Quick start

Install Go 1.23+.

Run the simulation:

    go run ./cmd/soundscape --theme themes/forest.yaml --simulate

Run against Fastly:

    FASTLY_API_TOKEN=... \
    go run ./cmd/soundscape \
      --service-id YOUR_SERVICE_ID \
      --theme themes/forest.yaml

Play through a SoundFont:

    FASTLY_API_TOKEN=... \
    go run ./cmd/soundscape \
      --service-id YOUR_SERVICE_ID \
      --theme themes/forest.yaml \
      --soundfont /path/to/your.sf2

The SoundFont should be an SF2 file. Go-MeltySynth is a pure-Go SoundFont synthesizer.

## Fastly API

The client uses the real-time analytics endpoint:

    https://rt.fastly.com/v1/channel/<service_id>/ts/<timestamp>

The API reports one-second records and returns a `Timestamp` to use for the next request.

## Theme format

See `themes/forest.yaml`.

The important distinction is:

* `source` describes a Fastly metric.
* `smooth` controls temporal smoothing.
* `normalise` maps a real metric into 0..1.
* `probabilistic` turns that value into events.
* `continuous` turns it into a continuous controller.

The theme engine is intended to grow a small vocabulary of reusable behaviours rather than requiring a Go plugin for every theme.

## Architecture

    internal/fastly     Fastly API client
    internal/metrics    smoothing and normalisation
    internal/theme      YAML theme + behaviour engine
    internal/scheduler  Poisson event-rate scheduler
    internal/event      abstract sound-event model (Note/Sample/Control)
    internal/output     sound output abstraction
    internal/audio      shared renderer->Oto ring-buffer plumbing
    internal/synth      SoundFont output backend
    internal/sampler    WAV sample-player output backend
    cmd/soundscape      CLI

The theme engine only depends on `internal/event` and `internal/output`'s
`Output` interface, never on a concrete backend. It turns metrics into
abstract events (`event.Note`, `event.Control`, `event.Sample`); each output
backend decides how to realise them — the SoundFont backend turns a `Note`
into a MIDI note-on/off pair, while `internal/sampler` plays a WAV file with
pitch shifting, velocity/pan, looping, voice stealing, and gain-smoothed
crossfades (an `event.Sample` with `Loop: true` starts a persistent layer
whose gain glides towards whatever value later events for the same actor
request — two such layers with opposing gain curves is how a gentle/rushing
river crossfade will be built).

## Next steps

1. Wire the sample player into a theme (the river, then birds) — needs real
   sample assets or procedurally-generated placeholders.
2. Add a real MIDI output backend using RtMidi.
3. Add richer stochastic actors such as flocks, crowds and weather.
4. Add hot theme reload.
5. Add MIDI 2.0/OSC output.
6. Embed themes and optionally assets into a single distributable binary.
