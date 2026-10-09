# Sample attribution

The procedurally-generated placeholder samples (`cmd/gensamples`) have been replaced with real
recordings, sourced from Wikimedia Commons (originally from pdsounds.org or uploaded as own
work) and, where Commons had nothing suitable, CC0 recordings from Freesound. Each was trimmed, converted to 16-bit PCM WAV, and — for the four looping beds — given a
short crossfade at the loop point; the crowd/chatter layers also got light EQ/gain shaping and
(for the chatter one-shots) a small per-clip pitch shift for variety. None of that editing
changes the underlying recording's character.

## themes/forest

| Sample group | Source file | Author | License |
|---|---|---|---|
| `birds` (variant-1..4) | [Bird singing.ogg](https://commons.wikimedia.org/wiki/File:Bird_singing.ogg) | jc | Public domain |
| `birds` (variant-5..7) | [Birds forest.ogg](https://commons.wikimedia.org/wiki/File:Birds_forest.ogg) | Barracuda1983 | Public domain |
| `woodpecker` | [Woodpecker tapping.ogg](https://commons.wikimedia.org/wiki/File:Woodpecker_tapping.ogg) | U.S. Fish and Wildlife Service | Public domain (US federal government work) |
| `splash` | [Bathtub water splashes.ogg](https://commons.wikimedia.org/wiki/File:Bathtub_water_splashes.ogg) | gradha | Public domain |
| `river-gentle` | [Shallow small river with stony riverbed.ogg](https://commons.wikimedia.org/wiki/File:Shallow_small_river_with_stony_riverbed.ogg) | stephan | Public domain |
| `river-rushing` | [Water fall.ogg](https://commons.wikimedia.org/wiki/File:Water_fall.ogg) | **Benzband** | **CC BY-SA 3.0** |
| `rain-light` | [Rain (1).ogg](https://commons.wikimedia.org/wiki/File:Rain_(1).ogg) | ezwa | Public domain |
| `rain-heavy` | [Thunderstorm after hot summer day 17 minutes 01 of 04.ogg](https://commons.wikimedia.org/wiki/File:Thunderstorm_after_hot_summer_day_17_minutes_01_of_04.ogg) | stephan | Public domain |
| `thunder` | [Storm thunderbolts.ogg](https://commons.wikimedia.org/wiki/File:Storm_thunderbolts.ogg) | stephan | Public domain |

The thunder claps were given 4-8 dB of gain into a limiter, to even them out and carry over the
rain.

## themes/market

| Sample group | Source file | Author | License |
|---|---|---|---|
| `crowd-quiet`, `crowd-busy`, `chatter` | [Restaurant ambience.ogg](https://commons.wikimedia.org/wiki/File:Restaurant_ambience.ogg) | stephan | Public domain |
| `bottles` | [Wine glass.ogg](https://commons.wikimedia.org/wiki/File:Wine_glass.ogg) | hugh | Public domain |
| `accordion` | [Accordion registers.ogg](https://commons.wikimedia.org/wiki/File:Accordion_registers.ogg) | Necz0r | Public domain |

## themes/road

| Sample group | Source file | Author | License |
|---|---|---|---|
| `traffic-quiet` | [Sunday in the city street noise1.ogg](https://commons.wikimedia.org/wiki/File:Sunday_in_the_city_street_noise1.ogg) | cori | Public domain |
| `traffic-busy` | [Highway from bridge center.ogg](https://commons.wikimedia.org/wiki/File:Highway_from_bridge_center.ogg) | stephan | Public domain |
| `motorbike` | [Motorbike 1.ogg](https://commons.wikimedia.org/wiki/File:Motorbike_1.ogg) | ezwa | Public domain |
| `horn` | [Car Horn.wav](https://commons.wikimedia.org/wiki/File:Car_Horn.wav) | 15HPanska_Ruttner_Jan | CC0 |
| `screech` | [Tire.ogg](https://freesound.org/people/egomassive/sounds/536769/) (Freesound, from its high-quality MP3 preview) | egomassive | CC0 |

The screech was also given about 7 dB of gain into a limiter so it carries over the highway bed.

## The one non-public-domain asset

`themes/forest/samples/river-rushing/loop.wav` derives from Benzband's "Water fall.ogg", licensed
[CC BY-SA 3.0](https://creativecommons.org/licenses/by-sa/3.0/). That means:

- **Attribution** is required: "Water fall.ogg by Benzband, licensed under CC BY-SA 3.0" (this
  file satisfies that).
- **Share-alike** applies to this derivative: if `river-rushing/loop.wav` is redistributed, it
  should carry the same CC BY-SA 3.0 (or a compatible) license. It does not change the licensing
  of the rest of the project — only this one derived audio file.

If that's not acceptable for how this project is distributed, swap `river-rushing` for a
public-domain source instead (see DEVELOPMENT.md for how sample groups are wired up, and
`cmd/gensamples` for a fallback that needs no external assets at all).

## Procedural placeholders

Every sample group in all three themes uses a real recording rather than `cmd/gensamples`'
procedurally-synthesized placeholders. That tool is still available (`go run ./cmd/gensamples
--theme forest|market`) as a zero-dependency fallback — e.g. for bootstrapping a brand new theme
before sourcing real assets, or regenerating a group if a licensing concern comes up later. The
sample player and theme format don't distinguish real vs. procedural samples; swapping either way
is just replacing files in a `sample_group` directory.
