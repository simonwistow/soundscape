#!/bin/sh
# smoke.sh BINARY [DIR]
#
# Proof the built binary runs: it reports its version, and every bundled
# theme validates against every bundled mapping, which loads each theme's
# samples, from disk and, if it was built with -tags embed, from the copies
# built into the binary.
# Nothing here needs an audio device, so it runs on CI runners and in a
# bare container. DIR is where themes/ and mappings/ live (default: the
# current directory).
set -eu

# Resolve the binary before changing directory, so a relative path works.
bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
cd "${2:-.}"

"$bin" version

builtin=$(mktemp -d)
trap 'rmdir "$builtin"' EXIT
embedded=false
# A built-in theme is listed as "  NAME   built in".
if (cd "$builtin" && "$bin" themes | grep -Eq '^  [^ ]+ +built in$'); then
  embedded=true
fi

for theme in themes/*/theme.yaml; do
  for mapping in mappings/*.yaml; do
    name=$(basename "$mapping" .yaml)
    case $name in
      apache) source=file:access.log ;;
      pcap) source=file:capture.pcap ;;
      *) source=$name ;;
    esac
    "$bin" validate --source "$source" --mappings "$mapping" "$theme"
    # The same pair by name, from a directory with no copies on disk.
    if $embedded; then
      (cd "$builtin" && "$bin" validate --source "$source" --mappings "$name" "$(basename "$(dirname "$theme")")")
    fi
  done
done
