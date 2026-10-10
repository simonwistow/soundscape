#!/bin/sh
# smoke.sh BINARY [DIR]
#
# Proof the built binary runs: it reports its version, and every bundled
# theme validates against every bundled mapping, which loads each theme's
# samples from disk. Nothing here needs an audio device, so it runs on CI
# runners and in a bare container. DIR is where themes/ and mappings/ live
# (default: the current directory).
set -eu

# Resolve the binary before changing directory, so a relative path works.
bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
cd "${2:-.}"

"$bin" version

for theme in themes/*/theme.yaml; do
  for mapping in mappings/*.yaml; do
    name=$(basename "$mapping" .yaml)
    case $name in
      apache) source=file:access.log ;;
      pcap) source=file:capture.pcap ;;
      *) source=$name ;;
    esac
    "$bin" validate --source "$source" --mappings "$mapping" "$theme"
  done
done
