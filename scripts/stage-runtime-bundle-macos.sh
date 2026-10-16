#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
manifest="$repo_root/packaging/runtime/manifest.json"
if [ "$(uname -s)" != Darwin ]; then
  echo "runtime staging: macOS requires a native Darwin host" >&2
  exit 1
fi
case "$(uname -m)" in
  arm64)
    target_id=macos-arm64
    goarch=arm64
    fylo_target=macos-arm64
    ;;
  x86_64)
    target_id=macos-amd64
    goarch=amd64
    fylo_target=macos-x64
    ;;
  *)
    echo "runtime staging: unsupported macOS architecture $(uname -m)" >&2
    exit 1
    ;;
esac

if [ "$#" -ne 1 ]; then
  echo "usage: $0 ABSOLUTE_EMPTY_STAGE_DIRECTORY" >&2
  exit 2
fi

stage_root=$1
case "$stage_root" in
  /*) ;;
  *)
    echo "runtime staging: stage directory must be absolute" >&2
    exit 2
    ;;
esac

if [ -L "$stage_root" ]; then
  echo "runtime staging: stage directory must not be a symlink" >&2
  exit 2
fi
if [ -e "$stage_root" ] && [ ! -d "$stage_root" ]; then
  echo "runtime staging: stage path exists and is not a directory" >&2
  exit 2
fi
if [ -d "$stage_root" ] && [ -n "$(find "$stage_root" -mindepth 1 -maxdepth 1 -print -quit)" ]; then
  echo "runtime staging: stage directory must be empty" >&2
  exit 2
fi

"$repo_root/scripts/check-runtime-bundle.sh" "$manifest"

umask 077
mkdir -p "$stage_root"
download_root="$stage_root/.downloads"
mkdir -p "$download_root"
artifact_rows="$download_root/artifacts.tsv"
notice_rows="$download_root/notices.tsv"

ruby -rjson -e '
  data = JSON.parse(File.read(ARGV.fetch(0)))
  target = data.fetch("targets").find { |item| item.fetch("id") == ARGV.fetch(1) }
  abort "runtime staging: target missing" unless target
  target.fetch("artifacts").each do |artifact|
    puts [artifact.fetch("runtime"), artifact.fetch("asset"), artifact.fetch("downloadUrl"), artifact.fetch("sha256"), artifact.fetch("installRelativePath")].join("\t")
  end
' "$manifest" "$target_id" > "$artifact_rows"

ruby -rjson -e '
  data = JSON.parse(File.read(ARGV.fetch(0)))
  target = data.fetch("targets").find { |item| item.fetch("id") == ARGV.fetch(1) }
  data.fetch("sources").each_value do |source|
    source.fetch("notices").each do |notice|
      puts [source.fetch("name"), notice.fetch("sourcePath"), target.fetch("noticeDirectory")].join("\t")
    end
  end
' "$manifest" "$target_id" > "$notice_rows"

while IFS="	" read -r runtime asset url expected_sha install_path; do
  download_path="$download_root/$asset"
  curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 \
    --output "$download_path.part" "$url"
  actual_sha=$(shasum -a 256 "$download_path.part" | awk '{print $1}')
  if [ "$actual_sha" != "$expected_sha" ]; then
    echo "runtime staging: checksum mismatch for $asset" >&2
    exit 1
  fi
  mv "$download_path.part" "$download_path"
  destination="$stage_root/$install_path"
  mkdir -p "$(dirname -- "$destination")"
  cp "$download_path" "$destination"
  chmod 0755 "$destination"
  echo "runtime staging: verified $runtime $asset"
done < "$artifact_rows"

while IFS="	" read -r source_name source_path notice_directory; do
  destination_directory="$stage_root/$notice_directory/$source_name"
  mkdir -p "$destination_directory"
  cp "$repo_root/$source_path" "$destination_directory/$(basename -- "$source_path")"
done < "$notice_rows"

ty_path="$stage_root/Sluice.app/Contents/Resources/runtime/bin/ty"
fylo_path="$stage_root/Sluice.app/Contents/Resources/runtime/bin/fylo"

validate_macho() {
  program=$1
  label=$2
  description=$(file -b "$program")
  case "$goarch:$description" in
    arm64:*"Mach-O 64-bit executable arm64"*) ;;
    amd64:*"Mach-O 64-bit executable x86_64"*) ;;
    *) echo "runtime staging: $label is not Mach-O $goarch" >&2; exit 1 ;;
  esac
}
validate_macho "$ty_path" ty
validate_macho "$fylo_path" fylo

expected_version=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("releaseVersion")' "$manifest")
actual_ty_version=$("$ty_path" --version)
if [ "$actual_ty_version" != "$expected_version" ]; then
  echo "runtime staging: ty version $actual_ty_version, expected $expected_version" >&2
  exit 1
fi

# shellcheck disable=SC2016
"$fylo_path" version --output json | ruby -rjson -e '
  report = JSON.parse($stdin.read)
  expected_version = ARGV.fetch(0)
  expected_commit = ARGV.fetch(1)
  expected_target = ARGV.fetch(2)
  abort "runtime staging: Fylo version mismatch" unless report["runtimeVersion"] == expected_version
  abort "runtime staging: Fylo commit mismatch" unless report["commit"] == expected_commit
  abort "runtime staging: Fylo build target mismatch" unless report["buildTarget"] == expected_target
' "$expected_version" "e236162508f6789f980fa7ca136a6671938030cb" "$fylo_target"

rm -rf "$download_root"
echo "runtime staging: $target_id payload passed in $stage_root"
