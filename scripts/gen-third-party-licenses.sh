#!/usr/bin/env bash
# Regenerates THIRD_PARTY_LICENSES.md from the module cache for the exact
# dependency versions pinned in go.mod. Run after changing dependencies:
#
#   make licenses
#
set -euo pipefail

cd "$(dirname "$0")/.."
MODCACHE="$(go env GOMODCACHE)"
OUT="THIRD_PARTY_LICENSES.md"

# License classification for modules whose license text isn't plain MIT.
lictype() {
  case "$1" in
    github.com/google/uuid|github.com/remyoudompheng/bigfft|golang.org/x/sys|\
modernc.org/sqlite|modernc.org/libc|modernc.org/mathutil|modernc.org/memory)
      echo "BSD-3-Clause" ;;
    nhooyr.io/websocket) echo "ISC" ;;
    *) echo "MIT" ;;
  esac
}

# Modules actually compiled into the binary (excludes the main module).
# Use a while-read loop for portability (macOS ships bash 3.2 without mapfile).
MODS=()
while IFS= read -r line; do
  [ -n "$line" ] && MODS+=("$line")
done < <(
  go list -deps -f '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{end}}' ./... \
    | grep -v "^github.com/mertgundoganx/gload" | sort -u
)

{
  echo "# Third-Party Licenses"
  echo ""
  echo "gload is distributed as a compiled binary that statically links the Go"
  echo "libraries listed below. Their licenses (all permissive) require that the"
  echo "original copyright and permission notices be preserved in distributions."
  echo "This file reproduces those notices in full."
  echo ""
  echo "Regenerate with \`make licenses\` after changing dependencies."
  echo ""
  echo "## Summary"
  echo ""
  echo "| Module | Version | License |"
  echo "|---|---|---|"
  for entry in "${MODS[@]}"; do
    mod="${entry%% *}"; ver="${entry##* }"
    echo "| \`$mod\` | $ver | $(lictype "$mod") |"
  done
  echo ""
  echo "---"
  echo ""
  for entry in "${MODS[@]}"; do
    mod="${entry%% *}"; ver="${entry##* }"
    esc="$(echo "$mod" | sed 's/\([A-Z]\)/!\L\1/g')"
    dir="$MODCACHE/${esc}@${ver}"
    lic="$(ls "$dir" 2>/dev/null | grep -iE '^(LICENSE|COPYING|LICENCE)(\.md|\.txt)?$' | head -1 || true)"
    echo "## $mod"
    echo ""
    echo "Version: $ver — License: $(lictype "$mod")"
    echo ""
    if [ -n "$lic" ] && [ -f "$dir/$lic" ]; then
      echo '```'
      cat "$dir/$lic"
      echo '```'
    else
      echo "> License file not found in module cache; run \`go mod download $mod\`."
    fi
    echo ""
  done
} > "$OUT"

echo "Wrote $OUT ($(grep -c '^## ' "$OUT") sections)."
