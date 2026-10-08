# Changelog

Notable changes to `soundscape`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/simonwistow/soundscape/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/simonwistow/soundscape/releases/tag/v0.1.0
