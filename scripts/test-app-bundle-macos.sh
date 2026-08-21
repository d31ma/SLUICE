#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
stage_root=$(mktemp -d "${TMPDIR:-/tmp}/sluice-app-stage.XXXXXX")
trap 'rm -rf "$stage_root"' EXIT HUP INT TERM

"$repo_root/scripts/stage-app-bundle-macos.sh" "$stage_root"

app="$stage_root/Sluice.app"
desktop="$app/Contents/MacOS/sluice-desktop"
state_root="$stage_root/qualification-state"

run_smoke() {
  PATH=/usr/bin:/bin "$desktop" \
    --background \
    --native-smoke \
    --qualification-state-root "$state_root" &
  desktop_pid=$!

  session_file="$state_root/run/agent-session.json"
  attempts=0
  while [ ! -f "$session_file" ] && kill -0 "$desktop_pid" 2>/dev/null; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 500 ]; then
      echo "app bundle test: agent descriptor did not appear" >&2
      kill -TERM "$desktop_pid" 2>/dev/null || true
      wait "$desktop_pid" || true
      return 1
    fi
    sleep 0.02
  done

  if [ ! -f "$session_file" ]; then
    echo "app bundle test: desktop exited before publishing the agent descriptor" >&2
    wait "$desktop_pid" || true
    return 1
  fi

  network=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("network")' "$session_file")
  socket=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("address")' "$session_file")
  test "$network" = unix
  test -S "$socket"

  wait "$desktop_pid"
  test ! -e "$session_file"
  test ! -e "$socket"
}

run_smoke
first_token=$(shasum -a 256 "$state_root/run/api-token" | awk '{print $1}')
run_smoke
second_token=$(shasum -a 256 "$state_root/run/api-token" | awk '{print $1}')
test "$first_token" = "$second_token"

if pgrep -f "$app/Contents/Helpers/sluice-agent" >/dev/null || \
   pgrep -f "$app/Contents/Helpers/sluice-yon" >/dev/null || \
   pgrep -f "$app/Contents/Resources/runtime/bin/ty" >/dev/null || \
   pgrep -f "$app/Contents/Resources/runtime/bin/fylo" >/dev/null; then
  echo "app bundle test: a packaged child survived desktop shutdown" >&2
  exit 1
fi

echo "app bundle test: packaged health, native IPC, restart, and clean shutdown passed"
