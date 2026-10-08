# Development notes

The current prototype intentionally has a small theme vocabulary.

## Sources, mappings and inputs

Data flows source -> mapping -> theme:

* A `source.Source` (`internal/source`, `internal/wikipedia`, `internal/prometheus`,
  `internal/fastly`) emits a timestamped `map[string]float64` of raw metrics per tick. Sources
  log and retry transient failures themselves rather than returning them.
* A mapping (`mappings/*.yaml`, `internal/mapping`) names the source and binds each theme input
  to one of its metrics (or, for Prometheus, a PromQL query), with `smoothing` and `normalise`.
  `mapping.Conditioner` applies that, producing 0..1 inputs.
* The theme engine (`theme.Engine.Process`) only ever sees those inputs; sounds pick one with
  `input:`.

Conditioning lives in the mapping rather than the theme because the right scale is a property of
the data: Wikipedia's ~20 edits/s and a CDN's 5000 req/s can't share a `normalise` range, but the
same forest should work for both. Input names are free-form; the conventional set (`activity`,
`flow`, `trouble`, `quirks`) is documented in the README so bundled themes and mappings mix and
match. `theme.Load` decodes strictly, so a misspelt key (or an old-format theme with
`sources:`/`metric:`) is an error rather than silently ignored.

## Event semantics

`probabilistic` sounds treat `rate` as an expected number of events per second, not a per-tick
probability. Each `Process()` call computes `lambda = rate * elapsed_seconds` and draws an event
count from a Poisson process (`internal/scheduler`), so a tick can naturally produce zero, one, or
several events rather than at most one. `elapsed_seconds` is derived from the actual gap between
record timestamps (clamped to 10s) rather than assumed to always be 1 second, so gaps in polling
don't unleash a flood of catch-up events.

A tick's events are sent as it's processed, so several in one tick sound together, on a beat set
by how often the source reports (once a second, for most). A sound with `spread: true` instead
sends each one after a random delay of up to `elapsed_seconds`, scattering them across the time
until the next tick. That suits a flock of birds or a bass line; a melody can sound better left on
the beat.

Randomness is seeded via `theme.NewEngineWithSeed`; `theme.NewEngine` seeds from the current time
for live variation, and the CLI's `--seed` flag can pin it for reproducible runs.

## Sound output modes

Each `Sound` has a `type` (the scheduling primitive: `probabilistic` or `continuous`) and an
`output` (how that primitive is realised):

* `output: note` (default for `probabilistic`) and `output: cc` (default for `continuous`) emit
  MIDI-style `event.Note`/`event.Control` values, for the SoundFont backend (`internal/synth`)
  and the MIDI backends (`internal/midi`: live to a port, or recorded to a file).
* `output: sample` emits a one-shot `event.Sample`, played by the WAV sample player
  (`internal/sampler`) with a random pick from `sample_group`, optional `pitch_jitter`, and a
  small random pan spread.
* `output: sample_loop` emits a persistent `event.Sample{Loop: true}`: the sample player starts it
  once and thereafter just glides its gain/pitch towards whatever a later event for the same actor
  (`Sound.Name`) requests, rather than retriggering. Its ramped `min_value..max_value` becomes the
  loop's gain, so two `sample_loop` sounds sharing an input with opposite ramps (see
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

Live MIDI (`internal/midi`'s `LiveOutput`) goes through RtMidi, which is C++ and talks to CoreMIDI
or ALSA, so building needs cgo and a C++ compiler on every platform. On Linux that was already
true for Oto's ALSA output; on macOS, Oto alone needed neither.

## Important

This is a prototype, not yet production audio code. In particular, the audio buffering and
probabilistic scheduler should be tightened before using it for a long-running installation.
