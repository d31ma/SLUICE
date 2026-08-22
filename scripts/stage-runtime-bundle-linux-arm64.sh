#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
manifest="$repository_root/packaging/runtime/manifest.json"
target_id=linux-arm64

if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != aarch64 ]; then
  echo "runtime staging: linux-arm64 requires a native Linux aarch64 environment" >&2
  exit 1
fi

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

"$repository_root/scripts/check-runtime-bundle.sh" "$manifest"

umask 077
mkdir -p "$stage_root"
app_root="$stage_root/Sluice.AppDir"
download_root="$stage_root/.downloads"
mkdir -p "$app_root" "$download_root"
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

while IFS="	" read -r runtime_name asset url expected_sha install_path; do
  download_path="$download_root/$asset"
  curl --fail --location --silent --show-error --proto '=https' --tlsv1.2 \
    --output "$download_path.part" "$url"
  actual_sha=$(sha256sum "$download_path.part" | awk '{print $1}')
  if [ "$actual_sha" != "$expected_sha" ]; then
    echo "runtime staging: checksum mismatch for $asset" >&2
    exit 1
  fi
  mv "$download_path.part" "$download_path"
  destination="$app_root/$install_path"
  mkdir -p "$(dirname -- "$destination")"
  cp "$download_path" "$destination"
  chmod 0755 "$destination"
  echo "runtime staging: verified $runtime_name $asset"
done < "$artifact_rows"

while IFS="	" read -r source_name source_path notice_directory; do
  destination_directory="$app_root/$notice_directory/$source_name"
  mkdir -p "$destination_directory"
  cp "$repository_root/$source_path" "$destination_directory/$(basename -- "$source_path")"
done < "$notice_rows"

ty_path="$app_root/usr/lib/sluice/runtime/bin/ty"
fylo_path="$app_root/usr/lib/sluice/runtime/bin/fylo"

case "$(file -b "$ty_path")" in
  *"ELF 64-bit"*"ARM aarch64"*) ;;
  *) echo "runtime staging: ty is not an ELF aarch64 executable" >&2; exit 1 ;;
esac
case "$(file -b "$fylo_path")" in
  *"ELF 64-bit"*"ARM aarch64"*) ;;
  *) echo "runtime staging: fylo is not an ELF aarch64 executable" >&2; exit 1 ;;
esac

expected_version=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).fetch("releaseVersion")' "$manifest")
actual_ty_version=$("$ty_path" --version)
if [ "$actual_ty_version" != "$expected_version" ]; then
  echo "runtime staging: ty version $actual_ty_version, expected $expected_version" >&2
  exit 1
fi

"$fylo_path" version --output json | ruby -rjson -e '
  report = JSON.parse($stdin.read)
  expected_version = ARGV.fetch(0)
  expected_commit = ARGV.fetch(1)
  abort "runtime staging: Fylo version mismatch" unless report["runtimeVersion"] == expected_version
  abort "runtime staging: Fylo commit mismatch" unless report["commit"] == expected_commit
  abort "runtime staging: Fylo build target mismatch" unless report["buildTarget"] == "linux-arm64"
' "$expected_version" "e236162508f6789f980fa7ca136a6671938030cb"

rm -R -- "$download_root"
echo "runtime staging: linux-arm64 payload passed in $stage_root"
