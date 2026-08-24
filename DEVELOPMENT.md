# Development notes

The current prototype intentionally has a small theme vocabulary.

## Event semantics

`probabilistic` is evaluated once for every Fastly record (normally once per second).

If:

    rate = 0.4

then the behaviour has a 40% chance of producing an event for that second.

This is deliberately simple. A later version should use a Poisson process so a rate can mean
"expected events per second" and can produce zero, one, or several events during a tick.

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
