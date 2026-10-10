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

- Recording. `--output file:forest.mp3` (or `.wav`) records the soundscape as it plays: with `--output speakers` as well to hear it, or silently, in real time, without. MP3 is 256 kbps unless `bitrate` says otherwise (`file:forest.mp3,bitrate=192`), encoded in pure Go, so the binary still needs nothing else installed.
- `--duration 10m` stops a run after that much soundscape. With the `simulate` source and only offline outputs (`file`, `midi-file`, `console`), it renders as fast as it can instead of in real time: an hour of the forest to MP3 takes well under a minute. With `--seed`, a render is the same every time, to the bit.
- OSC output. `--output osc:host:port` sends every event as an OSC message over UDP, addressed by the sound that made it (e.g. `/soundscape/birds/sample`, with its group, pitch, gain, pan and duration), for SuperCollider, Max, Pure Data and the like. Its `prefix` option changes `/soundscape` (`--output osc:localhost:57120,prefix=/forest`). `examples/osc/supercollider.scd` plays a theme's samples in SuperCollider from them.

### Changed

- `--source` picks the source, and the mapping named after it: `--source wikipedia:wikis=enwiki` reads `mappings/wikipedia.yaml`. `--mappings` (which replaces `--aliases`) picks another mapping, by name or path, and is needed for a recording. Mappings no longer name a source: one that still has `source:` stops with a message saying to remove it. `soundscape validate` takes `--source` and `--mappings` the same way.
- Packet captures as a source: `--source file:lunch.pcap` (or `.pcapng`) counts a capture's packets into per-second metrics (packets and bytes, by protocol; TCP connections opened, reset and closed; distinct hosts and conversations; DNS queries and failed lookups) and plays them through the new `mappings/pcap.yaml`.
- Access logs as a source: `--source file:access.log` counts a web server's access log into per-second metrics (requests, bytes, status classes, methods, distinct clients and, where logged, response times) and plays them through the new `mappings/apache.yaml`. It reads Apache's and nginx's Common and Combined Log Formats, and any other given as `logformat=` in Apache's `LogFormat` syntax.
- Recording telemetry: `--output telemetry:monday.jsonl` (or `.csv`) writes each tick's raw metrics from any source, for `--source file:` to replay; a replay with the same `--seed` sounds exactly like the run.
- Replaying recorded telemetry: `--source file:monday.csv --mappings fastly` plays a recording of a source's metrics through a mapping. It reads CSV, JSON Lines, InfluxDB line protocol and Prometheus/OpenMetrics text, by extension or `format=`. A recording plays a tick a second, holding each series for up to five minutes between samples; labelled series can be named exactly or summed, and Prometheus counters become rates. With offline outputs it renders as fast as it can, start to finish; `loop=true` repeats it forever.
- `--source` takes a source the way `--output` takes an output, `kind[:target][,option=value]`: `simulate`, `wikipedia:wikis=enwiki+dewiki`, `prometheus:http://host:9090,interval=5s`, `fastly:service=SID`. It replaces `--simulate`, `--service-id`, `--prometheus-url`, `--prometheus-interval` and `--wikipedia-wikis`, which now stop with a pointer to their replacement. The Fastly token stays with `--token` or `FASTLY_API_TOKEN`, rather than in a source spec that shows up in logs.
- `--output` says where the soundscape goes, and can be given more than once: `speakers` (the default), `midi:PORT`, `midi-virtual:NAME`, `midi-file:PATH`, `osc:HOST:PORT` and `console`, with options after commas (`osc:localhost:57120,prefix=/forest`). It replaces `--midi-port`, `--midi-virtual` and `--midi-out`, which now stop with a pointer to their replacement. Giving outputs without `speakers` leaves the samples unplayed, for another program to play.
- The forest's birdsong is a little quieter.

### Fixed

- The sample player and SoundFont synth no longer drift behind: they rendered audio faster than it was played, so the backlog grew by most of a second every second (sounds lagged further behind the telemetry the longer it ran, and memory grew with it). The audio device now paces them, and a sound is heard about a tenth of a second after it happens (it was half a second at best).
- A MIDI file no longer drifts out of time over a long session: each event's time was rounded down to a whole MIDI tick, and the errors added up.
- A theme with both samples and note sounds can now play them together through `--soundfont`; it used to stop with "oto: context is already created". The sample player and the synth now share one mix.

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
