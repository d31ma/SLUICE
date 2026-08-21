#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
stage_root=$(mktemp -d "${TMPDIR:-/tmp}/sluice-runtime-qualification.XXXXXX")
stage_root=$(CDPATH='' cd -- "$stage_root" && pwd -P)
desktop_pid=
agent_pid=
tachyon_pid=
fylo_pid=
socket_path=
trap 'cleanup' EXIT HUP INT TERM

idle_seconds=${SLUICE_QUALIFICATION_IDLE_SECONDS:-600}
sample_seconds=${SLUICE_QUALIFICATION_SAMPLE_SECONDS:-10}
warmup_seconds=${SLUICE_QUALIFICATION_WARMUP_SECONDS:-10}

require_positive_integer() {
  name=$1
  value=$2
  case "$value" in
    ''|*[!0-9]*|0)
      echo "app runtime qualification: $name must be a positive integer" >&2
      exit 2
      ;;
  esac
}

require_positive_integer SLUICE_QUALIFICATION_IDLE_SECONDS "$idle_seconds"
require_positive_integer SLUICE_QUALIFICATION_SAMPLE_SECONDS "$sample_seconds"
require_positive_integer SLUICE_QUALIFICATION_WARMUP_SECONDS "$warmup_seconds"

if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
  echo "app runtime qualification: requires a native Darwin arm64 host" >&2
  exit 1
fi

cleanup() {
  if [ -n "$desktop_pid" ] && kill -0 "$desktop_pid" 2>/dev/null; then
    kill -TERM "$desktop_pid" 2>/dev/null || true
    wait "$desktop_pid" 2>/dev/null || true
  fi
  rm -rf "$stage_root"
}

now_milliseconds() {
  ruby -e 'puts((Process.clock_gettime(Process::CLOCK_MONOTONIC) * 1000).round)'
}

find_staged_process() {
  executable=$1
  ps -axo pid=,comm= | awk -v executable="$executable" '
    $2 == executable { print $1; exit }
  '
}

find_staged_processes() {
  executable=$1
  ps -axo pid=,comm= | awk -v executable="$executable" '
    $2 == executable { print $1 }
  '
}

wait_for_process() {
  parent=$1
  executable=$2
  attempts=0
  while [ "$attempts" -lt 750 ]; do
    child=$(find_staged_process "$executable")
    if [ -n "$child" ]; then
      printf '%s\n' "$child"
      return 0
    fi
    if ! kill -0 "$parent" 2>/dev/null; then
      return 1
    fi
    attempts=$((attempts + 1))
    sleep 0.02
  done
  return 1
}

wait_for_file() {
  path=$1
  attempts=0
  while [ "$attempts" -lt 750 ]; do
    if [ -f "$path" ]; then
      return 0
    fi
    if ! kill -0 "$desktop_pid" 2>/dev/null; then
      return 1
    fi
    attempts=$((attempts + 1))
    sleep 0.02
  done
  return 1
}

wait_for_tachyon_listener() {
  owner=$1
  executable=$2
  attempts=0
  while [ "$attempts" -lt 750 ]; do
    for candidate in $(find_staged_processes "$executable"); do
      candidate_port=$(lsof -nP -a -p "$candidate" -iTCP -sTCP:LISTEN -Fn 2>/dev/null | awk -F: '
        /^n127[.]0[.]0[.]1:/ { print $NF; exit }
      ')
      if [ -n "$candidate_port" ]; then
        printf '%s %s\n' "$candidate" "$candidate_port"
        return 0
      fi
    done
    if ! kill -0 "$owner" 2>/dev/null; then
      return 1
    fi
    attempts=$((attempts + 1))
    sleep 0.02
  done
  echo "app runtime qualification: staged process snapshot after listener timeout" >&2
  ps -ww -axo pid=,ppid=,command= | awk -v root="$stage_root" '
    index($0, root) != 0 { print }
  ' >&2
  return 1
}

wait_for_health() {
  port=$1
  token=$(tr -d '\r\n' < "$token_file")
  attempts=0
  while [ "$attempts" -lt 750 ]; do
    status=$(curl --silent --show-error --max-time 0.5 \
      --output "$stage_root/health.json" --write-out '%{http_code}' \
      --header "Authorization: Bearer $token" \
      "http://127.0.0.1:$port/v1/health" 2>/dev/null || true)
    if [ "$status" = 200 ] && grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' "$stage_root/health.json"; then
      return 0
    fi
    if ! kill -0 "$desktop_pid" 2>/dev/null; then
      return 1
    fi
    attempts=$((attempts + 1))
    sleep 0.02
  done
  return 1
}

start_topology() {
  start_ms=$(now_milliseconds)
  HOME="$qualification_home" PATH=/usr/bin:/bin "$desktop" --background &
  desktop_pid=$!

  wait_for_file "$session_file" || {
    echo "app runtime qualification: agent descriptor did not appear" >&2
    return 1
  }
  wait_for_file "$token_file" || {
    echo "app runtime qualification: API token did not appear" >&2
    return 1
  }
  agent_pid=$(wait_for_process "$desktop_pid" "$agent_binary") || {
    echo "app runtime qualification: agent process did not appear" >&2
    return 1
  }
  tachyon_listener=$(wait_for_tachyon_listener "$desktop_pid" "$tachyon_binary") || {
    echo "app runtime qualification: Tachyon listener did not appear" >&2
    return 1
  }
  set -- $tachyon_listener
  tachyon_pid=$1
  port=$2
  fylo_pid=$(wait_for_process "$agent_pid" "$fylo_binary") || {
    echo "app runtime qualification: Fylo process did not appear" >&2
    return 1
  }

  network=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("network")' "$session_file")
  socket_path=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("address")' "$session_file")
  test "$network" = unix
  test -S "$socket_path"

  wait_for_health "$port" || {
    echo "app runtime qualification: authenticated health did not become ready" >&2
    return 1
  }
  ready_ms=$(now_milliseconds)
  startup_ms=$((ready_ms - start_ms))
}

process_snapshot() {
  pids="$desktop_pid,$agent_pid,$tachyon_pid,$fylo_pid"
  snapshot=$(ps -p "$pids" -o pid=,ppid=,%cpu=,rss=)
  count=$(printf '%s\n' "$snapshot" | awk 'NF { count++ } END { print count + 0 }')
  if [ "$count" -ne 4 ]; then
    echo "app runtime qualification: a long-lived process exited during idle sampling" >&2
    return 1
  fi
  printf '%s\n' "$snapshot" | awk -v desktop="$desktop_pid" -v agent="$agent_pid" '
    NF {
      cpu += $3
      rss += $4
      if ($1 == desktop || $1 == agent) core_rss += $4
    }
    END { printf "%.3f,%d,%d\n", cpu, rss, core_rss }
  '
}

process_cpu_seconds() {
  pids=$1
  ps -p "$pids" -o time= | awk -F: '
    NF == 2 { total += ($1 * 60) + $2 }
    NF == 3 { total += ($1 * 3600) + ($2 * 60) + $3 }
    END { printf "%.2f\n", total }
  '
}

sample_idle_resources() {
  metrics_file="$stage_root/idle-metrics.csv"
  printf 'elapsed_seconds,aggregate_cpu_percent,aggregate_rss_kib,desktop_agent_rss_kib\n' > "$metrics_file"
  all_pids="$desktop_pid,$agent_pid,$tachyon_pid,$fylo_pid"
  agent_stack_pids="$agent_pid,$fylo_pid"
  cpu_start=$(process_cpu_seconds "$all_pids")
  agent_cpu_start=$(process_cpu_seconds "$agent_pid")
  agent_stack_cpu_start=$(process_cpu_seconds "$agent_stack_pids")
  elapsed=0
  while :; do
    values=$(process_snapshot)
    printf '%s,%s\n' "$elapsed" "$values" >> "$metrics_file"
    if [ "$elapsed" -ge "$idle_seconds" ]; then
      break
    fi
    remaining=$((idle_seconds - elapsed))
    delay=$sample_seconds
    if [ "$remaining" -lt "$delay" ]; then
      delay=$remaining
    fi
    sleep "$delay"
    elapsed=$((elapsed + delay))
  done
  cpu_end=$(process_cpu_seconds "$all_pids")
  agent_cpu_end=$(process_cpu_seconds "$agent_pid")
  agent_stack_cpu_end=$(process_cpu_seconds "$agent_stack_pids")
  aggregate_cpu=$(awk -v start="$cpu_start" -v end="$cpu_end" -v duration="$idle_seconds" \
    'BEGIN { printf "%.3f", ((end - start) / duration) * 100 }')
  agent_cpu=$(awk -v start="$agent_cpu_start" -v end="$agent_cpu_end" -v duration="$idle_seconds" \
    'BEGIN { printf "%.3f", ((end - start) / duration) * 100 }')
  agent_stack_cpu=$(awk -v start="$agent_stack_cpu_start" -v end="$agent_stack_cpu_end" -v duration="$idle_seconds" \
    'BEGIN { printf "%.3f", ((end - start) / duration) * 100 }')
  metrics=$(awk -F, 'NR > 1 {
      samples++
      cpu += $2
      if ($2 > max_cpu) max_cpu = $2
      rss += $3
      if ($3 > max_rss) max_rss = $3
      core += $4
      if ($4 > max_core) max_core = $4
    }
    END {
      printf "%d %.3f %.3f %.0f %.0f %.0f %.0f", samples, cpu/samples, max_cpu,
        rss/samples, max_rss, core/samples, max_core
    }' "$metrics_file")
  set -- $metrics
  sample_count=$1
  sampled_mean_cpu=$2
  sampled_max_cpu=$3
  mean_rss_kib=$4
  max_rss_kib=$5
  mean_core_rss_kib=$6
  max_core_rss_kib=$7
}

wait_for_exit() {
  pid=$1
  attempts=0
  while kill -0 "$pid" 2>/dev/null && [ "$attempts" -lt 400 ]; do
    attempts=$((attempts + 1))
    sleep 0.02
  done
  ! kill -0 "$pid" 2>/dev/null
}

stop_topology() {
  stop_ms=$(now_milliseconds)
  kill -TERM "$desktop_pid"
  wait "$desktop_pid"
  desktop_pid=
  stopped_ms=$(now_milliseconds)
  shutdown_ms=$((stopped_ms - stop_ms))
  test ! -e "$session_file"
  test ! -e "$socket_path"
  for pid in "$agent_pid" "$tachyon_pid" "$fylo_pid"; do
    ! kill -0 "$pid" 2>/dev/null
  done
}

crash_agent_and_stop_desktop() {
  crashed_socket=$socket_path
  kill -KILL "$agent_pid"
  wait_for_exit "$agent_pid"
  wait_for_exit "$fylo_pid"
  crash_stop_ms=$(now_milliseconds)
  kill -TERM "$desktop_pid"
  wait "$desktop_pid" 2>/dev/null || true
  desktop_pid=
  crash_stopped_ms=$(now_milliseconds)
  crash_shutdown_ms=$((crash_stopped_ms - crash_stop_ms))
  test -f "$session_file"
  test -S "$crashed_socket"
  ! kill -0 "$tachyon_pid" 2>/dev/null
}

"$repo_root/scripts/stage-app-bundle-macos.sh" "$stage_root"

app="$stage_root/Sluice.app"
desktop="$app/Contents/MacOS/sluice-desktop"
agent_binary="$app/Contents/Helpers/sluice-agent"
tachyon_binary="$app/Contents/Resources/runtime/bin/ty"
fylo_binary="$app/Contents/Resources/runtime/bin/fylo"
qualification_home="$stage_root/home"
state_root="$qualification_home/Library/Application Support/Sluice"
session_file="$state_root/run/agent-session.json"
token_file="$state_root/run/api-token"
mkdir -p "$qualification_home"

start_topology
first_startup_ms=$startup_ms
token_digest=$(shasum -a 256 "$token_file" | awk '{print $1}')
sleep "$warmup_seconds"
sample_idle_resources
crash_agent_and_stop_desktop

recovery_start_ms=$(now_milliseconds)
start_topology
recovery_ready_ms=$(now_milliseconds)
recovery_ms=$((recovery_ready_ms - recovery_start_ms))
second_token_digest=$(shasum -a 256 "$token_file" | awk '{print $1}')
test "$token_digest" = "$second_token_digest"
test "$socket_path" = "$crashed_socket"
sleep "$warmup_seconds"
stop_topology

cat <<EOF
app runtime qualification: PASS
native target: macOS arm64
authenticated startup: ${first_startup_ms} ms
idle warmup: ${warmup_seconds} s
idle duration: ${idle_seconds} s
idle samples: ${sample_count}
aggregate CPU from process time: ${aggregate_cpu}%
agent CPU from process time: ${agent_cpu}%
agent+Fylo CPU from process time: ${agent_stack_cpu}%
aggregate sampled CPU mean/max: ${sampled_mean_cpu}% / ${sampled_max_cpu}%
all-process RSS mean/max: ${mean_rss_kib} KiB / ${max_rss_kib} KiB
desktop+agent RSS mean/max: ${mean_core_rss_kib} KiB / ${max_core_rss_kib} KiB
crash-path desktop shutdown: ${crash_shutdown_ms} ms
authenticated recovery after agent crash: ${recovery_ms} ms
clean shutdown: ${shutdown_ms} ms
stable token and Fylo root: yes
stale Unix socket recovered: yes
EOF

if [ "$idle_seconds" -lt 600 ]; then
  echo "app runtime qualification: development duration only; not ten-minute evidence" >&2
fi
