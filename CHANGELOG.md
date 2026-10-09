# Changelog

Notable changes to `soundscape`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `program` (and `bank`) on a note or cc sound picks its instrument, on a SoundFont or any General MIDI synth: `--soundfont`, live MIDI and `--midi-out` all send it as a program change. `soundscape soundfont-presets file.sf2` lists a SoundFont's instruments. The `pentatonic` theme now plays a vibraphone, an acoustic bass and tubular bells.
- `type: flock` sounds send groups of callers over: flocks arrive at `rate` per second, with `size` members that call `call_rate` times a second for the `pass` seconds a flock takes to go by. Each flock swells in, peaks overhead and fades away as it sweeps across the stereo field, and its members keep their own voices. The forest theme has flocks of starlings on `activity`.
- `type: crowd` sounds gather a crowd whose `size` follows the input, people drifting in and out over a few seconds. Each person has their own voice, place and distance, and calls `call_rate` times a second; an optional `rate` gives outbursts, when everyone calls at once and then tails off. The market theme's shoppers are now a crowd.
- A theme's `weather` makes an input of its own from one of the mapping's: it builds up towards that input's level over `build` seconds, takes `clear` seconds to die away, and `gusts` around its level, so a storm has a slow life of its own rather than following the telemetry tick by tick. The forest theme has a storm on `trouble`, with light and heavy rain and thunder.

- OSC output. `--osc host:port` sends every event as an OSC message over UDP, addressed by the sound that made it (e.g. `/soundscape/birds/sample`, with its group, pitch, gain, pan and duration), for SuperCollider, Max, Pure Data and the like. `--osc-prefix` changes `/soundscape`, and `--sample-player=false` turns off the built-in sample player, for when the receiver plays the samples itself. `examples/osc/supercollider.scd` plays a theme's samples in SuperCollider from them.

### Changed

- The forest's birdsong is a little quieter.

### Fixed

- The sample player and SoundFont synth no longer drift behind: they rendered audio faster than it was played, so the backlog grew by most of a second every second (sounds lagged further behind the telemetry the longer it ran, and memory grew with it). The audio device now paces them, about 46 ms ahead.

## [0.2.0] - 2026-10-08

### Added

- Live MIDI output. `--midi-virtual NAME` creates a virtual MIDI port for a synth or DAW to play from (macOS and Linux), and `--midi-port NAME` sends to an existing port, which `soundscape midi-ports` lists. Building from source now needs a C++ compiler on macOS too.
- The `pentatonic` theme, which plays MIDI notes: a melody on `activity`, a bass line on `flow`, and clashing notes on `trouble`.
- `spread: true` on a probabilistic sound scatters each tick's events at random across the time until the next tick, instead of sounding them all together as the tick arrives. The bundled forest, market and road themes use it.
- Licensed under the GNU General Public License version 3. The bundled recordings keep their own licenses, listed in `ATTRIBUTION.md`.

## [0.1.0] - 2026-10-07

### Added

- Data sources:
    - `simulate` — a built-in, slowly cycling simulation, for developing themes with no data at all.
    - `wikipedia` — Wikimedia's public recent-changes stream: live edits, page creations and log actions, optionally limited to particular wikis with `--wikipedia-wikis`.
    - `prometheus` — PromQL queries polled from a Prometheus server.
    - `fastly` — Fastly's real-time analytics API.
- Mappings (`mappings/*.yaml`, chosen with `--aliases`) that bind a source's metrics to a theme's inputs, with smoothing and normalisation, so any theme plays from any source.
- Three themes, built from real public-domain and CC0 recordings:
    - `forest-glade` — birdsong, a river from trickle to torrent, splashes and a woodpecker.
    - `farmers-market` — a crowd from murmur to bustle, chatter, dropped bottles and an accordion.
    - `busy-road` — traffic from a quiet street to a highway, motorbikes, car horns and tyre screeches.
- Output through a built-in WAV sample player, an optional SoundFont synth (`--soundfont`), or a Standard MIDI File (`--midi-out`).
- Poisson-scheduled events, so a sound's rate is an expected number of events per second.
- Hot reloading of the theme and mapping files while running.
- `soundscape validate`, which checks a theme and, with `--aliases`, the mapping it will be played with.
- `soundscape version`.
- Prebuilt releases for Linux x86-64 and macOS on Apple silicon.

### Known limitations

- There is no live MIDI device output yet, only MIDI files.
- On Linux, audio goes through ALSA, so `libasound2` must be installed.

[Unreleased]: https://github.com/simonwistow/soundscape/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/simonwistow/soundscape/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/simonwistow/soundscape/releases/tag/v0.1.0
