#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
manifest="$repository_root/packaging/runtime/manifest.json"
tool_root="$repository_root/tools/bin"

goos=$(go env GOOS)
goarch=$(go env GOARCH)
case "$goos-$goarch" in
  darwin-arm64) target_id=macos-arm64; fylo_target=macos-arm64 ;;
  darwin-amd64) target_id=macos-amd64; fylo_target=macos-x64 ;;
  linux-arm64) target_id=linux-arm64; fylo_target=linux-arm64 ;;
  linux-amd64) target_id=linux-amd64; fylo_target=linux-x64 ;;
  windows-amd64) target_id=windows-amd64; fylo_target=windows-x64 ;;
  *)
    echo "runtime bootstrap: unsupported host $goos-$goarch" >&2
    exit 1
    ;;
esac

"$repository_root/scripts/check-runtime-bundle.sh" "$manifest"

umask 077
mkdir -p "$tool_root"
artifact_rows=$(mktemp "${TMPDIR:-/tmp}/sluice-runtime-tools.XXXXXX")
trap 'rm -f "$artifact_rows"' EXIT HUP INT TERM

ruby -rjson -e '
  data = JSON.parse(File.read(ARGV.fetch(0)))
  target = data.fetch("targets").find { |item| item.fetch("id") == ARGV.fetch(1) }
  abort "runtime bootstrap: target missing" unless target
  target.fetch("artifacts").each do |artifact|
    puts [artifact.fetch("runtime"), artifact.fetch("downloadUrl"), artifact.fetch("sha256")].join("\t")
  end
' "$manifest" "$target_id" > "$artifact_rows"

while IFS="	" read -r runtime_name download_url expected_sha; do
  case "$runtime_name-$goos" in
    tachyon-windows) destination="$tool_root/ty.exe" ;;
    tachyon-*) destination="$tool_root/ty" ;;
    fylo-windows) destination="$tool_root/fylo.exe" ;;
    fylo-*) destination="$tool_root/fylo" ;;
    *)
      echo "runtime bootstrap: unknown runtime $runtime_name" >&2
      exit 1
      ;;
  esac

  actual_sha=""
  if [ -f "$destination" ]; then
    actual_sha=$(ruby -rdigest -e 'puts Digest::SHA256.file(ARGV.fetch(0)).hexdigest' "$destination")
  fi
  if [ "$actual_sha" = "$expected_sha" ]; then
    echo "runtime bootstrap: reused verified $runtime_name for $target_id"
    continue
  fi

  temporary=$(mktemp "$tool_root/.${runtime_name}.XXXXXX")
  if ! curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 \
    --output "$temporary" "$download_url"; then
    rm -f "$temporary"
    exit 1
  fi
  actual_sha=$(ruby -rdigest -e 'puts Digest::SHA256.file(ARGV.fetch(0)).hexdigest' "$temporary")
  if [ "$actual_sha" != "$expected_sha" ]; then
    rm -f "$temporary"
    echo "runtime bootstrap: checksum mismatch for $runtime_name" >&2
    exit 1
  fi
  chmod 0755 "$temporary"
  mv -f "$temporary" "$destination"
  echo "runtime bootstrap: installed verified $runtime_name for $target_id"
done < "$artifact_rows"

case "$goos" in
  windows)
    ty_path="$tool_root/ty.exe"
    fylo_path="$tool_root/fylo.exe"
    ;;
  *)
    ty_path="$tool_root/ty"
    fylo_path="$tool_root/fylo"
    ;;
esac

expected_version=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("releaseVersion")' "$manifest")
expected_commit=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("sources").fetch("fylo").fetch("sourceCommit")' "$manifest")

if [ "$("$ty_path" --version)" != "$expected_version" ]; then
  echo "runtime bootstrap: Tachyon version mismatch" >&2
  exit 1
fi

# shellcheck disable=SC2016
"$fylo_path" version --output json | ruby -rjson -e '
  report = JSON.parse($stdin.read)
  abort "runtime bootstrap: Fylo version mismatch" unless report["runtimeVersion"] == ARGV.fetch(0)
  abort "runtime bootstrap: Fylo commit mismatch" unless report["commit"] == ARGV.fetch(1)
  abort "runtime bootstrap: Fylo target mismatch" unless report["buildTarget"] == ARGV.fetch(2)
' "$expected_version" "$expected_commit" "$fylo_target"

echo "runtime bootstrap: $target_id tools passed"
