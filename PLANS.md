# Fastly Soundscape — Project Context

## Overview

Fastly Soundscape is a Go application that turns Fastly real-time analytics into **generative, naturalistic soundscapes**.

The central idea is **not** to directly translate metrics into musical notes. Instead, Fastly telemetry represents the changing environment of an artificial ecosystem.

For example:

* A **forest glade** might have:

  * request rate → frequency of bird activity
  * bandwidth → river intensity
  * errors → occasional woodpecker/crow/disturbance
  * traffic spikes → birds suddenly becoming active
* A **farmers' market** might have:

  * requests → number of people/conversations
  * bandwidth → crowd/bustle level
  * errors → occasional dropped bottles or other events
  * longer-term traffic → market opening, becoming busy, peaking, and winding down

The desired result should feel **organic and environmental**, rather than like a graph being played through a synthesizer.

---

## Architectural goal

The desired architecture is:

```text
                 Fastly Realtime API
                         |
                         v
                 +---------------+
                 | Metrics Input |
                 +-------+-------+
                         |
                         v
              smoothing / normalisation
                         |
                         v
                 +---------------+
                 | Theme Engine  |
                 +-------+-------+
                         |
                  sound events
                         |
             +-----------+-----------+
             |                       |
             v                       v
       Internal Audio            External MIDI
       / SoundFont               / Hardware / DAW
             |                       |
             +-----------+-----------+
                         |
                       audio
                         |
                      speakers
```

The **theme engine must not depend on a particular audio output**.

Potential outputs include:

* Console
* Embedded SoundFont synthesizer
* Embedded sample player
* External MIDI
* OSC
* Eventually other audio/control systems

The same theme should work with any of these.

---

# Why MIDI is involved

MIDI is useful as a **control protocol**, but it is not the thing that should generate the naturalistic audio itself.

For conventional instruments, MIDI notes and controllers are useful.

For environmental sounds, the application should eventually have a richer abstract event model:

```text
BirdChirp
RiverParameterChange
Splash
CrowdEvent
WindGust
etc.
```

An output backend can then decide how to represent those events.

For example:

```text
BirdChirp
    |
    +--> SoundFont -> MIDI Note
    |
    +--> SamplePlayer -> WAV sample
    |
    +--> MIDIOutput -> external synthesizer
```

Do **not** make the core theme language MIDI-specific.

---

# Self-contained operation

A major goal is that someone without audio hardware should be able to do something like:

```bash
soundscape run --theme forest
```

and immediately hear the result.

Therefore a conventional DAW should **not** be required.

The preferred architecture is an embedded audio engine consisting of some combination of:

* SoundFont synthesizer
* sample player
* mixer
* basic DSP/effects
* audio output

The prototype uses **Go-MeltySynth** as the SoundFont synthesizer and **Oto** as the audio output layer.

These are intended to make a self-contained Go implementation possible.

External DAWs and hardware remain optional.

---

# SoundFonts

SoundFonts are explicitly supported as an optional part of the design.

They are particularly useful for:

* pitched instruments
* melodic layers
* sampled instruments
* MIDI-oriented installations
* external-synth-like workflows

Raw audio samples are probably preferable for many naturalistic sounds:

* birds
* rivers
* insects
* crowds
* wind
* environmental effects

A theme should eventually be able to use either or both.

Conceptually:

```yaml
instrument:
  type: soundfont
  file: forest.sf2
  preset: ...
```

or:

```yaml
instrument:
  type: samples
  directory: samples/birds
```

The underlying theme engine should not care which is being used.

---

# Themes

A major design goal is that themes should be **declarative**.

Creating a new theme should normally involve:

1. Creating a YAML file.
2. Adding audio samples and/or a SoundFont.
3. Defining mappings and behaviours.

It should **not** require writing Go.

A theme might eventually look like:

```text
themes/
  forest/
    theme.yaml
    samples/
      bird-01.wav
      bird-02.wav
      bird-03.wav
      stream.wav
      stream-rush.wav
      woodpecker.wav

  farmers-market/
    theme.yaml
    samples/
      crowd.wav
      chatter.wav
      accordion.wav
      bottles.wav
```

The configuration should describe:

* Fastly metrics
* smoothing
* normalisation
* derived signals
* actors
* behaviours
* probabilities
* continuous controls
* instruments
* samples
* MIDI channels/controllers where appropriate
* long-term environmental state

---

# Behaviour vocabulary

The theme language should eventually contain a small set of reusable behaviour primitives.

## Continuous

A metric continuously controls a parameter.

Example:

```text
bandwidth -> river volume
```

## Ramp

Map a normalised metric to a parameter range.

Example:

```text
bandwidth 0..1
    ->
filter cutoff 800..8000 Hz
```

## Probabilistic

Convert a metric into stochastic events.

Example:

```text
requests
    ->
expected bird chirps / second
```

## Threshold

Generate an event when a value crosses a threshold.

Example:

```text
errors > 100/sec
    ->
crow
```

## Burst

React to a sudden change rather than an absolute value.

Example:

```text
normal traffic
    ->
large sudden spike
    ->
flock takes flight
```

## State

Maintain longer-lived environmental states.

For example:

```text
quiet
normal
busy
extremely_busy
```

This matters because natural environments shouldn't instantly react to every one-second telemetry fluctuation.

## Group / flock / crowd

Higher-level behaviours composed of multiple actors.

For example:

```text
Forest
  |
  +-- Bird flock
        |
        +-- Robin
        +-- Wren
        +-- Blackbird
```

The individual actors should have their own state and behaviour rather than simply producing independent random events.

---

# Probabilistic model

This is one of the most important aspects of the project.

The initial prototype uses a simple probability-per-tick mechanism.

That is only a placeholder.

The intended semantic model is:

> A configured rate represents an expected number of events per unit time.

For example:

```text
0.4 birds/second
```

should produce something like:

```text
0 birds
1 bird
0 birds
2 birds
1 bird
0 birds
3 birds
...
```

rather than simply giving one event a 40% chance once per second.

A **Poisson process** or equivalent event scheduler is a good starting point.

The scheduler should eventually support:

* events per second
* multiple events per tick
* zero events
* cooldowns
* refractory periods
* event clustering
* bursts
* deterministic random seeds for tests
* configurable random seeds for live operation

The ultimate goal is to produce **natural variation**.

---

# Stateful actors

A major future direction is to make sounds behave like actors in an ecosystem.

For example, a bird might have:

* preferred pitch range
* activity level
* minimum silence between calls
* response probability
* personality/random variation
* relationship to other birds
* response to environmental state

Then a flock can be constructed from several birds.

This should make the result sound more like an ecosystem and less like a random note generator.

---

# Forest example

A possible mapping:

| Fastly metric        | Environmental interpretation | Sound                      |
| -------------------- | ---------------------------- | -------------------------- |
| requests/sec         | activity                     | bird frequency             |
| request acceleration | sudden activity              | birds become excited       |
| bandwidth            | water flow                   | river intensity            |
| cache hit ratio      | environmental character      | timbral variation          |
| errors/sec           | disturbance                  | woodpecker/crow            |
| active POPs          | geographic activity          | size/density of soundscape |
| latency              | environmental tension        | filter/pitch/texture       |

The river is an important example.

Do **not** simply do:

```text
bandwidth -> MIDI note velocity
```

Instead:

```text
bandwidth
    |
    v
normalised energy
    |
    +--> volume
    +--> sample crossfade
    +--> filter cutoff
    +--> splash frequency
    +--> number of overlapping voices
```

For example:

```text
low bandwidth
    -> gentle stream
    -> quiet
    -> soft/dark spectrum

high bandwidth
    -> rushing stream
    -> louder
    -> brighter spectrum
    -> more splashes
```

This is a key design principle:

> **Map infrastructure metrics to perceptual/environmental parameters, rather than directly to musical parameters.**

---

# Current prototype

A first prototype was created as:

```text
fastly-soundscape-v0.zip
```

Its basic structure is:

```text
cmd/soundscape/
    main.go

internal/fastly/
    client.go

internal/metrics/
    metrics.go

internal/output/
    output.go

internal/theme/
    theme.go

internal/synth/
    soundfont.go

themes/
    forest.yaml

README.md
DEVELOPMENT.md
go.mod
```

The prototype currently includes:

* Fastly realtime API client
* metric ingestion
* smoothing
* normalisation
* YAML themes
* probabilistic events
* continuous control events
* console output
* optional SoundFont output
* simulation mode

Simulation mode was intended to allow development without a Fastly account:

```bash
go run ./cmd/soundscape \
    --theme themes/forest.yaml \
    --simulate
```

Fastly mode is intended to look approximately like:

```bash
FASTLY_API_TOKEN=... \
go run ./cmd/soundscape \
    --service-id YOUR_SERVICE_ID \
    --theme themes/forest.yaml
```

SoundFont mode:

```bash
FASTLY_API_TOKEN=... \
go run ./cmd/soundscape \
    --service-id YOUR_SERVICE_ID \
    --theme themes/forest.yaml \
    --soundfont /path/to/forest.sf2
```

---

# Important: verify the prototype

The prototype was generated as a starting scaffold and should **not** be assumed to be production-ready.

First tasks for Codex should include:

```bash
go test ./...
go vet ./...
go build ./cmd/soundscape
```

and fixing any dependency/API/compilation problems.

In particular, verify the current APIs and versions of:

* Go-MeltySynth
* Oto
* YAML library
* Fastly realtime API

The prototype's audio buffering should also be reviewed carefully.

---

# Recommended next steps

## 1. Get the prototype compiling

Before adding significant functionality:

* run tests
* build the application
* fix dependency/API issues
* add basic tests
* verify the Fastly response format
* verify SoundFont playback with an actual `.sf2`

---

## 2. Improve the event model

Separate the conceptual sound event from MIDI.

Something along these lines:

```text
SoundEvent
  |
  +-- NoteEvent
  +-- SampleEvent
  +-- ControlEvent
  +-- ParameterEvent
```

For example:

```text
BirdChirp {
    Actor: robin
    Sample: bird-03
    Pitch: ...
    Velocity: ...
    Duration: ...
}
```

The SoundFont backend could turn that into MIDI.

The sample backend could play a WAV.

The MIDI backend could send a MIDI message.

---

## 3. Implement the proper stochastic scheduler

Replace the current probability-per-tick mechanism with expected event rates.

For example:

```text
requests = 0.0..1.0

    ->
bird activity = 0.1..8.0 events/sec

    ->
Poisson scheduler
```

Make it deterministic when supplied with a seed so tests can reproduce behaviour.

---

## 4. Build a real sample player

This is probably the most important audio feature after the SoundFont prototype.

It should eventually support:

* WAV loading
* multiple samples
* random sample selection
* pitch adjustment
* velocity
* volume
* pan
* envelopes
* looping
* voice management
* voice stealing
* crossfading

Natural environmental sounds will benefit enormously from this.

---

## 5. Implement the river

Use the river as the first sophisticated continuous sound.

Target behaviour:

```text
Fastly bandwidth
       |
       v
normalised energy
       |
       +--> gentle/rushing crossfade
       +--> volume
       +--> filter
       +--> splash event rate
```

This will prove that the system can create a convincing environment rather than just producing notes.

---

## 6. Add stateful actors

Start with something like:

```text
BirdActor
```

and eventually:

```text
FlockActor
```

A flock should contain multiple related actors with:

* cooldowns
* preferred ranges
* correlated activity
* responses to environmental state

---

## 7. Add external MIDI

Implement:

```text
MIDIOutput
```

behind the same output abstraction.

Eventually support commands like:

```bash
soundscape run --theme forest --output midi
```

and device selection.

Do not make the theme language MIDI-specific.

---

## 8. Add SoundFont presets

Allow themes to specify:

* SoundFont
* bank
* program/preset
* channel

This will make SoundFonts much more useful for melodic/tonal themes.

---

## 9. Add theme validation

Eventually:

```bash
soundscape validate themes/forest.yaml
```

should catch:

* missing sources
* invalid metrics
* invalid ranges
* invalid MIDI channels
* invalid CC values
* missing samples
* unknown behaviours
* malformed configuration

---

## 10. Add theme hot reload

For development, it would be extremely useful to change:

```text
theme.yaml
```

and hear the change without restarting the program.

---

# Longer-term ideas

Potential future themes include:

* Forest glade
* Farmers' market
* Ocean/harbour
* City street
* Train station
* Rainforest
* Desert
* Night-time city
* Space station
* Factory
* Haunted house
* Mountain valley

The underlying behaviour engine should be general enough that these are mostly new configurations and assets.

---

# Possible CLI

Eventually aim for something like:

```bash
soundscape run --theme forest

soundscape run --theme forest --output internal

soundscape run --theme forest --output midi

soundscape simulate --theme forest

soundscape validate themes/forest.yaml

soundscape list-themes

soundscape list-midi-devices

soundscape inspect --theme forest
```

---

# Distribution goal

Ultimately a theme should be packageable with:

```text
theme.yaml
samples/
SoundFont
metadata
```

and potentially embedded into the executable using Go's `embed` package.

The ideal end-user experience is something like:

```bash
soundscape --theme forest
```

with the application containing everything needed to play the theme.

External hardware remains optional.

---

# Design principles

These principles should guide future implementation decisions.

## Do not turn metrics directly into notes

Prefer:

```text
metric
  ->
environmental parameter
  ->
behaviour
  ->
sound event
```

## Prefer declarative themes

Adding a new soundscape should normally require YAML and assets, not Go.

## Keep output backends independent

The same theme should work with:

* internal synthesis
* SoundFonts
* samples
* MIDI hardware
* DAWs
* OSC

## Make randomness controllable

Live operation should be organic.

Tests should be deterministic.

## Smooth telemetry

One-second infrastructure metrics can be noisy.

Sound should generally respond over several seconds unless the theme explicitly wants sharp events.

## Preserve history

Useful derived signals include:

* current value
* moving average
* acceleration
* spike
* decay
* sustained high activity
* sustained low activity

The metrics layer should eventually expose these.

## Think like an ecosystem

The ultimate goal is **not a musical visualizer**.

It is:

> **A generative sound environment whose weather is determined by infrastructure telemetry.**

A strong version of the project should let someone who knows YAML and has a directory of interesting sounds create a new artificial ecosystem without writing Go.

---

# Immediate priority

When continuing development, the recommended order is:

1. **Compile and test the existing prototype.**
2. **Fix/verify the SoundFont audio path.**
3. **Introduce a proper abstract sound-event model.**
4. **Implement the Poisson/event scheduler.**
5. **Implement sample playback.**
6. **Make the forest river convincing.**
7. **Add stateful bird/flock behaviour.**
8. **Add external MIDI.**
9. **Expand the theme language.**
10. **Build the farmers' market theme as a second complete example.**

The project should resist the temptation to build a giant audio framework before there is one genuinely convincing soundscape.

The first real milestone should be:

> **Run the application against live Fastly traffic and hear a forest that feels alive, with birds becoming more active as requests increase and a river becoming more turbulent as bandwidth increases.**
