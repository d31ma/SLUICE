#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
smoke_flag=--native-smoke
if [ "${1:-}" = --runtime-only ]; then
  smoke_flag=--packaged-runtime-smoke
  shift
fi
case "$#" in
  0)
    stage_root=$(mktemp -d "${TMPDIR:-/tmp}/sluice-app-stage.XXXXXX")
    owns_stage=1
    ;;
  1)
    stage_root=$1
    owns_stage=0
    case "$stage_root" in
      /*) ;;
      *) echo "app bundle test: staged root must be absolute" >&2; exit 2 ;;
    esac
    ;;
  *)
    echo "usage: $0 [--runtime-only] [ABSOLUTE_STAGED_DIRECTORY]" >&2
    exit 2
    ;;
esac

cleanup() {
  if [ "$owns_stage" -eq 1 ]; then
    case "$stage_root" in
      "${TMPDIR:-/tmp}"/sluice-app-stage.*) rm -R -- "$stage_root" ;;
      *) echo "refusing to remove unexpected scratch path: $stage_root" >&2 ;;
    esac
  fi
}
trap cleanup EXIT HUP INT TERM

if [ "$owns_stage" -eq 1 ]; then
  "$repo_root/scripts/stage-app-bundle-macos.sh" "$stage_root"
fi

app="$stage_root/Sluice.app"
desktop="$app/Contents/MacOS/sluice-desktop"
state_root="$stage_root/qualification-state"

run_smoke() {
  PATH=/usr/bin:/bin "$desktop" \
    --background \
    "$smoke_flag" \
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

if [ "$smoke_flag" = --native-smoke ]; then
  echo "app bundle test: packaged health, native IPC, native UI lifecycle, restart, and clean shutdown passed"
else
  echo "app bundle test: packaged health, native IPC, headless runtime lifecycle, restart, and clean shutdown passed"
fi
