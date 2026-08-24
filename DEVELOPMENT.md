# Development notes

The current prototype intentionally has a small theme vocabulary.

## Event semantics

`probabilistic` sounds treat `rate` as an expected number of events per second, not a per-tick
probability. Each `Process()` call computes `lambda = rate * elapsed_seconds` and draws an event
count from a Poisson process (`internal/scheduler`), so a tick can naturally produce zero, one, or
several events rather than at most one. `elapsed_seconds` is derived from the actual gap between
record timestamps (clamped to 10s) rather than assumed to always be 1 second, so gaps in polling
don't unleash a flood of catch-up events.

Randomness is seeded via `theme.NewEngineWithSeed`; `theme.NewEngine` seeds from the current time
for live variation, and the CLI's `--seed` flag can pin it for reproducible runs.

## Sound output modes

Each `Sound` has a `type` (the scheduling primitive: `probabilistic` or `continuous`) and an
`output` (how that primitive is realised):

* `output: note` (default for `probabilistic`) and `output: cc` (default for `continuous`) emit
  MIDI-style `event.Note`/`event.Control` values for the SoundFont backend (`internal/synth`).
* `output: sample` emits a one-shot `event.Sample`, played by the WAV sample player
  (`internal/sampler`) with a random pick from `sample_group`, optional `pitch_jitter`, and a
  small random pan spread.
* `output: sample_loop` emits a persistent `event.Sample{Loop: true}`: the sample player starts it
  once and thereafter just glides its gain/pitch towards whatever a later event for the same actor
  (`Sound.Name`) requests, rather than retriggering. Its ramped `min_value..max_value` becomes the
  loop's gain, so two `sample_loop` sounds sharing a metric with opposite ramps (see
  `themes/forest/theme.yaml`'s `river-gentle`/`river-rushing`) crossfade against each other.

`sample_group` paths are relative to the theme file (resolved in `theme.Load`), so a theme
directory (`theme.yaml` + `samples/`) is self-contained wherever it's run from. `cmd/gensamples`
procedurally synthesizes placeholder WAV assets (tone sweeps, filtered noise) for bootstrapping a
new theme before real recordings are available — see its package comment.

## SoundFonts

An SF2 file can contain ordinary pitched instruments as well as sampled material. For
`note`/`cc`-output sounds the theme selects MIDI notes and velocities. The next useful step is
adding a SoundFont preset/program field to the theme and allowing different actors to select
different presets.

## Native audio

Two output backends render audio without any external hardware: `internal/synth` (Go-MeltySynth,
for SF2-driven sounds) and `internal/sampler` (a from-scratch WAV player, for `sample`/
`sample_loop`-output sounds). Both share `internal/audio`'s renderer->Oto ring buffer and can run
simultaneously via `output.Multi` if a theme mixes both output styles. Oto provides cross-platform
audio output; Go-MeltySynth is pure Go and supports SoundFonts.

The external-MIDI backend is intentionally not bundled into this first prototype because the
cross-platform MIDI device layer has OS-specific dependencies. The output interface is already
designed so it can be added without changing the theme engine.

## Important

This is a prototype, not yet production audio code. In particular, the audio buffering and
probabilistic scheduler should be tightened before using it for a long-running installation.
