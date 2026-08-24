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

## SoundFonts

An SF2 file can contain ordinary pitched instruments as well as sampled material. For the first
prototype the theme selects MIDI notes and velocities. The next useful step is adding a SoundFont
preset/program field to the theme and allowing different actors to select different presets.

## Native audio

The prototype uses Go-MeltySynth plus Oto. Go-MeltySynth is pure Go and supports SoundFonts;
Oto provides cross-platform audio output.

The external-MIDI backend is intentionally not bundled into this first prototype because the
cross-platform MIDI device layer has OS-specific dependencies. The output interface is already
designed so it can be added without changing the theme engine.

## Important

This is a prototype, not yet production audio code. In particular, the audio buffering and
probabilistic scheduler should be tightened before using it for a long-running installation.
