#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
stage_root=$(mktemp -d "${TMPDIR:-/tmp}/sluice-runtime-stage.XXXXXX")
trap 'rm -rf "$stage_root"' EXIT HUP INT TERM

"$repo_root/scripts/stage-runtime-bundle-macos.sh" "$stage_root"

test -x "$stage_root/Sluice.app/Contents/Resources/runtime/bin/ty"
test -x "$stage_root/Sluice.app/Contents/Resources/runtime/bin/fylo"
test -f "$stage_root/Sluice.app/Contents/Resources/licenses/Tachyon/LICENSE"
test -f "$stage_root/Sluice.app/Contents/Resources/licenses/Tachyon/NOTICE"
test -f "$stage_root/Sluice.app/Contents/Resources/licenses/Fylo/LICENSE"
test ! -e "$stage_root/.downloads"
cmp -s "$repo_root/third_party/tachyon/LICENSE" \
  "$stage_root/Sluice.app/Contents/Resources/licenses/Tachyon/LICENSE"
cmp -s "$repo_root/third_party/tachyon/NOTICE" \
  "$stage_root/Sluice.app/Contents/Resources/licenses/Tachyon/NOTICE"
cmp -s "$repo_root/third_party/fylo/LICENSE" \
  "$stage_root/Sluice.app/Contents/Resources/licenses/Fylo/LICENSE"

echo "runtime staging test: passed without modifying system paths"
