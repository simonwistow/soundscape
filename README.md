# Soundscape

A Go-based generative soundscape driven by live telemetry. Busy systems sound like a busy forest
(or market, or road): more traffic, more birdsong; more data flowing, a more turbulent river.

    data source (simulation, Fastly, Prometheus, Wikipedia edits, ...)
          |
          v
    mapping: source metrics -> named theme inputs (smoothing, normalisation to 0..1)
          |
          v
    theme engine: inputs -> abstract sound events (Poisson-scheduled)
          |
          +----> console output (text, no audio)
          |
          +----> WAV sample player -> speakers (self-contained, no extra files needed)
          |
          +----> SoundFont synth -> speakers (optional, needs an .sf2 file)
          |
          +----> MIDI -> a synth or DAW, live, and/or a .mid file (optional)

Themes say what things sound like, and mappings say what data drives them, so any theme can be
played from any source. Both are YAML, not Go code.

## Quick start

Download a prebuilt release for Linux x86-64 or macOS on Apple silicon from the
[releases page](https://github.com/simonwistow/soundscape/releases), unpack it, and run
`./soundscape` from inside the unpacked directory (it finds `themes/` and `mappings/` there). The
Linux build needs Ubuntu 24.04 or later (glibc 2.39) and the ALSA library (`libasound2`, packaged
as `libasound2t64` on Ubuntu 24.04+).

Or build from source, with Go 1.27+ and a C and C++ compiler (on macOS, the Xcode command line
tools; on Linux, also the ALSA headers: `libasound2-dev`). The examples below use `go run`:

Run it. With no other options this plays the forest theme from a built-in simulation that
cycles slowly between quiet and busy:

    go run ./cmd/soundscape

Listen to Wikipedia being edited, live (no account needed):

    go run ./cmd/soundscape --source wikipedia

Try the other example themes:

    go run ./cmd/soundscape --source wikipedia --theme themes/market/theme.yaml
    go run ./cmd/soundscape --source wikipedia --theme themes/road/theme.yaml

The `pentatonic` theme plays MIDI notes rather than recordings, so it needs a synth to play them;
see [MIDI](#midi) below.

##  Rationale

For a long time I've been fascinated by the ways that humans (and other animals) can
get an extremely quick read of a situation and can often discern that something is wrong
without quite knowing why.

I first started thinking about when I read the MIT Press book "Sources of Power" by Gary Klein
and his work on intuition and decision making. Stories of experienced Fire Fighters "knowing"
that something was wrong even without obvious evidence and evacuating shortly before a building
collapse or how locals know when trouble is brewing in a city because of a change of mood but how
tourists, oblivious to the differences, blunder into riots or revolutions.

Soldiers are told, upon entering a stationary position, that they must remain absolutely still
and silent for several minutes. This waiting period allows their hearing to adjust to the "normal"
sounds of the environment (the baseline) so that variations stand out immediately.

I've done experiments where I've had a small number (3 or 4) of important metrics (e.g requests,
bandwidth and errors) on several TV screens around where developers sit. Most of the developers
said they didn't look at the screens but the number of outages and issues dropped dramatically.

Timelapse video though showed them subconsciously looking at the screens frequently, even if it was
only a quick glance in much the same way that players in sports often constantly scan around them,
probably without thinking about it, to mentally build a map of where team mates and opponents are.

I've tried experiments before back in the early 00s - using Alex McLean's [MIDI::Realtime](https://metacpan.org/pod/MIDI::Realtime)
Perl module to turn metrics into noise which, unfortunately, sounded like cacophonous screeching. But even
in the noise certain patterns could be heard - port knocking sounded like quick, staccato knocks for example.
Alex went on to much greater success with the [Live Coding](https://www.perl.com/pub/2004/08/31/livecode.html/)
and [TOPLAP](https://blog.toplap.org) movements.

20 years later this is another attempt - using a fake but real sounding environment to allow people
to subconsciously detect when things are going wrong. You're sitting in a NOC listening to tranquil
forest sounds and suddenly the rivers starts flowing faster. Or the birds stop chirping. Or you hear
a roll of thunder.

I am not a sound designer though so there are lots of improvements and refinements to be made.

## Data sources

`--source` says where the telemetry comes from, written like an output,
`kind[:target][,option=value]`. It also picks the mapping, the one named after it: `--source
wikipedia:wikis=enwiki` reads `mappings/wikipedia.yaml`. `--mappings` picks another, by name or
path, e.g. to play the Wikipedia mapping against simulated data with `--source simulate
--mappings wikipedia`.

| `--source`                          | Mapping                    | Needs |
|-------------------------------------|----------------------------|-------|
| `simulate`                          | `mappings/simulate.yaml`   | nothing |
| `wikipedia[:wikis=enwiki+dewiki]`   | `mappings/wikipedia.yaml`  | internet access; `wikis` limits it to some wikis |
| `prometheus[:URL][,interval=5s]`    | `mappings/prometheus.yaml` | a Prometheus server; URL defaults to `PROMETHEUS_URL`, else `http://localhost:9090`; it's polled every second unless `interval` says otherwise |
| `fastly[:service=SID]`              | `mappings/fastly.yaml`     | a token, from `--token` or `FASTLY_API_TOKEN`; the service defaults to `FASTLY_SERVICE_ID` |
| `file:PATH[,format=F][,loop=true][,speed=N]` | whichever fits its metrics, by `--mappings`; `apache` for an access log, `pcap` for a capture | a recording, a log or a capture; see [Replaying recordings](#replaying-recordings) |

With no `--source`, a configured `FASTLY_SERVICE_ID` selects `fastly`, and otherwise the
simulation is used.

* **simulate**: deterministic synthetic telemetry, for developing themes without any data.
* **wikipedia**: Wikimedia's public [recent-changes stream](https://stream.wikimedia.org/), with
  every edit, page creation and log action across Wikipedia and its sister projects, counted into
  per-second rates (`edits`, `new_pages`, `human_edits`, `bot_edits`, `bytes_changed`,
  `log_delete`, ...; see `internal/wikipedia`).
* **prometheus**: polls PromQL queries. Each input in the mapping gives a `query` that reduces to
  one number. The example mapping queries Prometheus's own HTTP metrics, so it works against a
  bare Prometheus. Copy it and point the queries at your own metrics.
* **fastly**: Fastly's real-time analytics API
  (`https://rt.fastly.com/v1/channel/<service_id>/ts/<timestamp>`), one-second records.
* **file**: replays recorded telemetry; see below.

### Replaying recordings

`--source file:PATH` replays a recording of a source's metrics, through whichever mapping fits
them, so a period can be heard again, or rendered:

    go run ./cmd/soundscape --source file:monday.csv --mappings fastly

Make one from any source with `--output telemetry:`, which writes each tick's raw metrics, before
the mapping, as JSON Lines or CSV. Alongside the speakers, or on its own to record quietly:

    go run ./cmd/soundscape --source wikipedia --output telemetry:monday.jsonl

Once a metric has turned up, every later tick records it, as 0 when the source left it out (as
Wikipedia does with whatever didn't happen that second), so a recording replays exactly as the run
played; with `--seed`, to the bit. A CSV gains a column when a new metric turns up.

It reads four formats, told apart by the extension, or by `format=` for any other:

| `format=`    | Extensions                    | Looks like |
|--------------|-------------------------------|------------|
| `csv`        | `.csv`                        | a header row, then `timestamp,requests,errors` rows; the time column is `timestamp`, `time` or `ts`, else the first |
| `jsonl`      | `.jsonl`, `.ndjson`, `.json`  | `{"ts": 1696000000, "requests": 4500}` per line; nested objects become `a.b` |
| `influx`     | `.lp`, `.influx`, `.line`     | InfluxDB line protocol: `fastly,service=x requests=4500 1696000000000000000`, a series per field, named `fastly.requests` |
| `prometheus` | `.prom`, `.om`, `.metrics`    | Prometheus or OpenMetrics text, as used for backfilling: `requests{code="200"} 4500 1696000000000` |
| `apache`     | `.log`                        | a web server's access log; see below |
| `pcap`       | `.pcap`, `.pcapng`, `.cap`    | a packet capture, from tcpdump or Wireshark; see below |

Every sample needs a timestamp: Unix seconds, milliseconds, microseconds or nanoseconds (told
apart by size), or, in CSV and JSON, an RFC 3339 date. A recording plays a tick a second; a
series keeps its last value for up to five minutes, so data scraped every 15 seconds plays
smoothly, and a longer gap is skipped. A series with labels or tags can be named exactly, as
`requests{code="200"}` (labels sorted), or as plain `requests`, summed across its labels.
Prometheus counters become per-second rates, as `rate()` would make them.

#### Access logs

An access log has a line per request rather than metrics, so it's counted into them, a second at
a time: `requests`, `bytes`, `status_1xx` to `status_5xx`, `method_get`, `method_post` and so on
(`method_other` for the rest), and `clients` (distinct addresses). If the log records how long
each request took, there's also `response_ms` and `response_ms_max`, the mean and the slowest. A
second with no requests counts 0; a lull of more than five minutes is skipped. `mappings/apache.yaml`
plays these, and is the default for an access log:

    go run ./cmd/soundscape --source file:/var/log/apache2/access.log

It reads Apache's and nginx's Common and Combined Log Formats as they are (nginx's extra fields on
the end are fine). For any other, give the server's `LogFormat` as `logformat=`, quoted for the
shell, e.g. for one with response times:

    go run ./cmd/soundscape --source 'file:access.log,logformat=%h %l %u %t "%r" %>s %b %D'

It needs a time (`%t`, in the default format) and a status (`%>s` or `%s`), and uses `%h` or `%a`
for clients, `%r` or `%m` for methods, `%b`, `%B` or `%O` for bytes, and `%D`, `%T` or `%{ms}T`
for response times; anything else is matched and ignored. The `logformat` can't contain a comma.
Lines that don't fit are skipped (the count is logged), unless most don't.

#### Packet captures

A packet capture, pcap or pcapng, from tcpdump, Wireshark and the like, is counted the same way:
`packets` and `bytes`, the same for `tcp_`, `udp_`, `icmp_` and `other_` (as in `tcp_bytes`),
`tcp_syn`, `tcp_rst` and `tcp_fin` (connections opened, reset and closed), `hosts` (distinct
sources), `flows` (distinct conversations), `dns_queries` and `dns_failures` (answers saying a name
doesn't exist, or the server failed). `mappings/pcap.yaml` plays them, and is the default for a
capture: more conversations, more birdsong; bytes, the river; resets, a storm; failed lookups, the
woodpecker.

    sudo tcpdump -i en0 -w lunch.pcap        # Ctrl+C to stop
    go run ./cmd/soundscape --source file:lunch.pcap

Without an output that plays live, a recording renders as fast as it can, start to finish, with
no `--duration` needed. `loop=true` plays it again and again, for an installation; then
`--duration` says how much to render.

`speed=` makes a time-lapse: `speed=60` plays an hour of the recording in a minute, `speed=0.5`
plays it at half speed. The soundscape keeps its own pace, a tick a second, each tick covering
`speed` seconds of the recording: averaged when faster, so a spike still counts, and repeated when
slower. Its slow parts, such as the forest's storm building, keep their own pace too, so a short
incident at high speed may only start one:

    go run ./cmd/soundscape --source file:access.log,speed=60 --output file:day.mp3

## Themes and mappings

A theme's sounds each follow an **input**, a named value from 0 to 1. A mapping says where each
input comes from and how it's scaled:

    # themes/forest/theme.yaml          # mappings/wikipedia.yaml
    sounds:                             inputs:
      - name: birds                       activity:
        type: probabilistic                 metric: edits
        input: activity                     smoothing: 8
        ...                                 normalise: { min: 1, max: 40 }

Input names are free, but the bundled themes and mappings use a conventional set, so they
interoperate:

| Input      | Meaning                    | Fastly           | Wikipedia       | Forest      | Market       | Road |
|------------|----------------------------|------------------|-----------------|-------------|--------------|------|
| `activity` | how much is happening      | `requests`       | `edits`         | birdsong    | crowd, chatter | traffic, quiet street to highway |
| `flow`     | how much is moving through | `resp_body_bytes`| `bytes_changed` | river, splashes | accordion | motorbikes |
| `trouble`  | things going wrong         | `errors`         | `log_delete`    | –           | dropped bottles | tyre screeches |
| `quirks`   | minor oddities             | `all_status_4xx` | `new_pages`     | woodpecker  | –            | car horns |

The `pentatonic` theme plays a melody on `activity`, a bass line on `flow`, and notes from
outside its scale on `trouble`.

An input the mapping doesn't provide reads as 0 (with a warning at startup).

* In a mapping, `smoothing` is a time constant in seconds, and `normalise` maps the raw value
  logarithmically into 0..1.
* In a theme, `type: probabilistic` turns an input into a Poisson-scheduled event rate,
  `type: continuous` ramps it into `min_value..max_value`, `type: flock` sends groups of
  callers over, which swell in, peak overhead and fade away as they cross from one side to the
  other, and `type: crowd` gathers people who come and go with the input, chat, and now and then
  all react at once.
* A theme's `weather` makes an input of its own from one of the mapping's, with a slow life of
  its own: a storm that builds up, gusts, and takes its time to clear. Sounds use it like any other
  input; the forest's rain and thunder follow a storm driven by `trouble`.
* `output` selects how that's realised: `note`/`cc` (MIDI, played live, through a SoundFont, or
  recorded to a file) or
  `sample`/`sample_loop` (WAV playback, via the sample player). See DEVELOPMENT.md for the full
  vocabulary, including how two `sample_loop` sounds crossfade.

A theme is a directory: `theme.yaml` plus a `samples/` folder, so it's self-contained wherever
it's run from (`sample_group` paths resolve relative to the theme file).

## Outputs

`--output` says where the soundscape goes. With none it plays through the speakers; give it more
than once to send it to several places at once:

| `--output` | |
|---|---|
| `speakers` | plays sample sounds, and note sounds with `--soundfont`, through the audio device |
| `file:PATH[,bitrate=KBPS]` | records what the speakers would play to a `.wav` or `.mp3` file |
| `midi:PORT` | sends note and cc sounds to a MIDI port, live |
| `midi-virtual:NAME` | creates a virtual MIDI port and sends note and cc sounds to it, live |
| `midi-file:PATH` | writes note and cc sounds to a Standard MIDI File |
| `osc:HOST:PORT[,prefix=/PREFIX]` | sends every event as an OSC message over UDP |
| `telemetry:PATH[,format=jsonl\|csv]` | records the source's raw metrics, for `--source file:` to replay |
| `console` | prints every event |

Options follow the target after commas, as `key=value`. Without `speakers` (say, with just
`--output osc:...`) nothing is played here, so another program can play the samples instead.

## Recording

`file:` records the soundscape as it plays, WAV or MP3 by the file's extension. Give
`speakers` too, to hear it while it records:

    go run ./cmd/soundscape --theme themes/forest/theme.yaml --output speakers --output file:forest.mp3

Without `speakers` it records silently, in real time, which needs no sound card, so it can run
on a server. Several files can be written at once. Stop with Ctrl+C; the files are finished on
the way out.

WAV is 16-bit stereo at 44.1 kHz; a WAV can hold about 6.7 hours, after which it stops (and says
so) while the rest carries on. MP3 is constant bitrate, 256 kbps unless the `bitrate` option says
otherwise (`file:forest.mp3,bitrate=192`; 32 to 320). It's encoded in pure Go with
[shine](https://github.com/braheezy/shine-mp3), a simple encoder: at 256 kbps it was
indistinguishable from LAME on the forest storm, but lower bitrates cost it more than they would
LAME.

A recording holds what the speakers play: sample sounds, and note sounds with `--soundfont`. For
the notes themselves, use `midi-file:`.

`--duration` stops a run after that much soundscape. When the source can be replayed
(`simulate`, or a `file`) and every output is offline (`file`, `midi-file`, `console`), it doesn't wait for the
clock: it renders as fast as it can, about a hundred times real time for the forest.

    go run ./cmd/soundscape --theme themes/forest/theme.yaml --duration 1h --seed 42 --output file:forest.mp3

With `--seed`, a render comes out the same, to the bit, every time. A recording that doesn't
loop renders to its end without a `--duration`. With a live source (Fastly,
Prometheus, Wikipedia) or a live output, `--duration` runs in real time and then stops.

## MIDI

A theme's `note`/`cc`-output sounds can be sent, live, to anything that plays MIDI: a hardware
synth, a DAW, or a software instrument. No hardware is needed.

Create a virtual MIDI port called `soundscape`, which other software sees as a MIDI input:

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --output midi-virtual:soundscape

On macOS, GarageBand plays it with no setup: create an empty project with a Software Instrument
track, and it plays whatever arrives on any MIDI input. On Linux, connect the port to a synth such
as FluidSynth with `aconnect` (or watch the raw messages with `aseqdump`). Windows has no virtual
ports.

Or send to a MIDI output port that already exists, such as a synth or macOS's IAC Driver (enable
it in Audio MIDI Setup). A name that matches no port exactly can be part of one name, so `IAC`
finds `IAC Driver Bus 1`:

    go run ./cmd/soundscape midi-ports
    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --output midi:IAC

Each sound's `channel` (0-15) becomes its MIDI channel (1-16 in most software), so a DAW can give
each sound its own instrument.

Play the same sounds through a SoundFont, with no other software. Any General MIDI SoundFont
will do, such as the freely licensed
[GeneralUser GS](https://github.com/mrbumpy409/GeneralUser-GS):

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --soundfont /path/to/your.sf2

A sound's `program` picks its instrument, on a SoundFont or on any General MIDI synth or DAW that
follows program changes (GarageBand doesn't). In General MIDI, 11 is a vibraphone, 32 an acoustic
bass, and so on; list the instruments in a SoundFont, with the `bank` and `program` that select
each, with:

    go run ./cmd/soundscape soundfont-presets /path/to/your.sf2

A program belongs to a MIDI channel, so sounds that share a channel share an instrument.

Or record them as a Standard MIDI File (finalized on exit, including Ctrl+C):

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --output midi-file:session.mid

Live MIDI, a SoundFont and a MIDI file can all be used at once, alongside a theme's samples:
the SoundFont plays through `--output speakers`, the default, so add it back when giving others:

    go run ./cmd/soundscape --theme themes/pentatonic/theme.yaml --soundfont /path/to/your.sf2 \
        --output speakers --output midi-file:session.mid

## OSC

Every event, from every kind of sound, can be sent as an [Open Sound Control](https://opensoundcontrol.stanford.edu/)
message over UDP, for SuperCollider, Max, Pure Data, TouchDesigner or anything else that speaks
OSC to play, or react to, as it likes:

    go run ./cmd/soundscape --theme themes/forest/theme.yaml --output osc:localhost:57120

Each message is addressed by the sound that made it, so a receiver can pick sounds out with one
pattern match:

| Address | Arguments |
|---|---|
| `/soundscape/<sound>/note` | `i` pitch, `i` velocity, `i` duration_ms, `i` channel |
| `/soundscape/<sound>/sample` | `s` group, `f` pitch, `f` gain, `f` pan, `i` duration_ms, `i` channel |
| `/soundscape/<sound>/loop` | `s` group, `f` pitch, `f` gain, `f` pan, `i` channel |
| `/soundscape/<sound>/control` | `f` value, `i` controller, `i` channel |
| `/soundscape/program` | `i` channel, `i` bank, `i` program |

A sample's group is its `sample_group` directory's name (e.g. `birds`) and its pitch a playback
ratio (1 is as recorded); pan runs from -1 (left) to 1 (right). A loop's message comes every tick
while it plays, with its current gain, so it doubles as a continuous control. The `prefix` option
changes `/soundscape`, e.g. `--output osc:localhost:57120,prefix=/forest`.

With just `--output osc:...`, the samples are left to the receiver. To hear them through the
speakers as well, add `--output speakers`.

`examples/osc/supercollider.scd` is a receiver to start from: it plays a theme's samples in
SuperCollider, from the OSC messages, much as the built-in player does.

    /Applications/SuperCollider.app/Contents/MacOS/sclang examples/osc/supercollider.scd themes/forest/samples
    go run ./cmd/soundscape --theme themes/forest/theme.yaml --output osc:localhost:57120

## More options

Pin the random seed for a reproducible run:

    go run ./cmd/soundscape --seed 42

Stop after ten minutes:

    go run ./cmd/soundscape --duration 10m

Log the raw metrics as they arrive:

    go run ./cmd/soundscape --source wikipedia --verbose

Validate a theme, and optionally check it against a mapping. This catches missing or duplicate
names, inverted ranges, out-of-range MIDI values, missing sample directories, unknown behaviour
or output kinds, and theme inputs the mapping doesn't provide:

    go run ./cmd/soundscape validate --source wikipedia themes/forest/theme.yaml

Edit the theme or mapping while a run is going and it hot-reloads automatically. Files are
checked once a second and validated before being applied, so an invalid edit is logged and
ignored rather than crashing or going silent. Tuning a mapping's `normalise` ranges against live
data this way is the quickest route to a good-sounding setup. Switching a mapping's source, or
changing Prometheus queries, needs a restart. Disable with `--watch=false`.

## Current state

* four data sources: simulation, Wikipedia live edits, Prometheus, and Fastly real-time analytics
* source-independent themes: mappings bind source metrics to theme inputs, with exponential
  smoothing and log normalisation
* a Poisson event scheduler: a sound's `rate` is an expected events/second, not a per-tick
  probability, so a tick can naturally produce zero, one, or several events
* an abstract sound-event model (`event.Note`/`event.Sample`/`event.Control`) so the theme engine
  never depends on a specific output backend
* a real WAV sample player (`internal/sampler`): pitch shifting, velocity/pan, looping, click-free
  envelopes, voice stealing, and gain-smoothed crossfades between looping layers
* three complete example themes, `forest-glade`, `farmers-market` and `busy-road`, using real
  recordings (see ATTRIBUTION.md), so no SoundFont or external assets are needed to hear something,
  and a fourth, `pentatonic`, that plays MIDI notes
* an optional SoundFont backend (Go-MeltySynth) for `note`/`cc`-output sounds
* optional MIDI output for `note`/`cc`-output sounds: live, to a MIDI port or a virtual port of
  its own (RtMidi), and/or recorded as a Standard MIDI File
* console mode for development without audio

`cmd/gensamples` procedurally synthesizes placeholder sample assets (tone sweeps for chirps,
filtered noise for rivers/crowds/splashes), which is useful for bootstrapping a new theme before
real recordings are ready:

    go run ./cmd/gensamples --theme forest   # writes to themes/forest/samples
    go run ./cmd/gensamples --theme market   # writes to themes/market/samples

## Architecture

    internal/source      Source interface + the built-in simulation
    internal/telemetry   recorded telemetry: CSV, JSON Lines, Influx, Prometheus, access-log and pcap readers
    internal/wikipedia   Wikimedia recent-changes stream source
    internal/prometheus  Prometheus query source
    internal/fastly      Fastly real-time API source
    internal/mapping     mapping files: source metrics -> conditioned theme inputs
    internal/metrics     smoothing and normalisation
    internal/theme       YAML theme + behaviour engine
    internal/scheduler   Poisson event-rate scheduler
    internal/event       abstract sound-event model (Note/Sample/Control)
    internal/output      sound output abstraction (incl. Multi fan-out)
    internal/audio       the mixer: sources (sampler, synth) into sinks (speakers, files)
    internal/record      WAV and MP3 file sinks
    internal/clock       real and virtual clocks, for live runs and fast renders
    internal/render      faster-than-real-time rendering on a virtual clock
    internal/synth       SoundFont output backend
    internal/sampler     WAV sample-player output backend
    internal/midi        MIDI output backends: live (RtMidi) and Standard MIDI File
    internal/osc         OSC output over UDP
    cmd/soundscape       CLI
    cmd/gensamples       placeholder sample asset generator
    internal/cmd/changelog  CHANGELOG.md linting and release tooling (see RELEASING.md)
    mappings/            bundled mappings, one per source
    themes/              bundled themes

A `source.Source` just emits a timestamped `map[string]float64` each tick; a
`mapping.Conditioner` turns that into theme inputs; and the theme engine only depends on
`internal/event` and `internal/output`'s `Output` interface, never on a concrete source or
backend. To add a source, implement `Run(ctx, emit)` and add a case to `cmd/soundscape`'s
`buildSource`.

`cmd/soundscape` starts a backend for each `--output` (parsed by `output.ParseSpecs`). The
speakers and files get one `audio.Mixer`: the sample player, whenever the theme references any
`sample_group`, and the SoundFont synth, if `--soundfont` is given, render into it. Every
backend that takes events runs at once via `output.Multi`. With none, it falls back to the
text-only Console backend.

## Next steps

1. Add more stochastic actors alongside flocks, crowds and weather.
2. Add MIDI 2.0 output, once synths support it more widely.
3. Embed themes, mappings and optionally assets into a single distributable binary.

## License

Soundscape is free software, licensed under the GNU General Public License version 3; see
[LICENSE](LICENSE). The recordings under `themes/*/samples/` are not covered by it: each keeps
its own license (public domain, CC0 or, for one file, CC BY-SA 3.0), as listed in
[ATTRIBUTION.md](ATTRIBUTION.md).
