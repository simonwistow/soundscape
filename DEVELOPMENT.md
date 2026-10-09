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

`flock` sounds are the first with state that lasts across ticks: the engine keeps the flocks
passing over (`Engine.flocks`), timed against `Engine.clock`, the sum of every tick's
`elapsed_seconds`. Flocks arrive like a probabilistic sound's events, at `rate` per second, each
with `size` members (scaled by the input, give or take a quarter) and a `pass` time picked at
random. Every tick, each member of each passing flock draws a Poisson count of calls (`call_rate`
per second) at random moments before the next tick; a call's gain follows `sin(pi * progress)`
through the pass and, for samples, its pan sweeps from one side to the other. Members keep their
own pitch (samples) or note (notes) for the whole pass. A reload keeps flocks whose sound still
exists, with its new settings, and drops the rest. See `internal/theme/flock.go`.

`crowd` sounds keep a population per sound (`Engine.crowds`). Each tick, every missing person
arrives, and every surplus person leaves, with probability `1 - exp(-dt / 5s)`, so the crowd's
size follows `size` (scaled by the input) with a few seconds' lag. Each person has a fixed voice,
pan and distance (gain), and calls at `call_rate`. An optional `rate` gives outbursts: everyone
calls one to three times within two seconds, bunched towards the start, at the top of the
velocity range. See `internal/theme/crowd.go`.

A theme's `weather` entries derive inputs of their own, before any sound is processed
(`Engine.applyWeather`, which copies the inputs rather than changing the caller's map). Each
follows its `input` with a first-order lag, time constant `build` rising and `clear` falling, and
multiplies that level by `1 + gust`, where `gust` is an Ornstein-Uhlenbeck process (6s time
constant, standard deviation `gusts`), so clear weather stays calm. `theme.Inputs` lists what a
weather follows, not the weather itself, so mappings are checked against what they really need.
Weather can't follow other weather. See `internal/theme/weather.go`.

Randomness is seeded via `theme.NewEngineWithSeed`; `theme.NewEngine` seeds from the current time
for live variation, and the CLI's `--seed` flag can pin it for reproducible runs.

## Sound output modes

Each `Sound` has a `type` (the scheduling primitive: `probabilistic`, `continuous`, `flock` or `crowd`) and an
`output` (how that primitive is realised):

* `output: note` (default for `probabilistic`, `flock` and `crowd`) and `output: cc` (default for `continuous`) emit
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

The OSC backend (`internal/osc`) is the exception: it sends every kind of event, so a receiver
gets everything a theme does, samples included. It encodes OSC 1.0 itself (int32, float32 and
string arguments; no bundles) and writes each message as one UDP datagram, ignoring write errors
so a receiver that isn't running yet just misses what's sent meanwhile. Spread and flock calls
arrive when they're due, not early with a timetag.

`sample_group` paths are relative to the theme file (resolved in `theme.Load`), so a theme
directory (`theme.yaml` + `samples/`) is self-contained wherever it's run from. `cmd/gensamples`
procedurally synthesizes placeholder WAV assets (tone sweeps, filtered noise) for bootstrapping a
new theme before real recordings are available — see its package comment.

## SoundFonts

An SF2 file can contain ordinary pitched instruments as well as sampled material, each a preset
selected by bank and program number. A `note`/`cc`-output sound's `program` (and `bank`, default
0) picks one: the engine sends an `event.Program` for each channel that has one before its first
note, and again after a hot reload. Every MIDI backend turns it into a bank select (CC 0) and a
program change, so it also reaches a live General MIDI synth or a `.mid` file. Programs belong to
channels, so `theme.Validate` rejects sounds on one channel that ask for different ones.
`soundscape soundfont-presets file.sf2` lists a SoundFont's presets.

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
