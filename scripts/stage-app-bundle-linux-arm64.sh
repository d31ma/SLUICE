#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != aarch64 ]; then
  echo "app bundle staging: requires a native Linux aarch64 environment" >&2
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
    echo "app bundle staging: stage directory must be absolute" >&2
    exit 2
    ;;
esac

if [ -L "$stage_root" ]; then
  echo "app bundle staging: stage directory must not be a symlink" >&2
  exit 2
fi
if [ -e "$stage_root" ] && [ ! -d "$stage_root" ]; then
  echo "app bundle staging: stage path exists and is not a directory" >&2
  exit 2
fi
if [ -d "$stage_root" ] && [ -n "$(find "$stage_root" -mindepth 1 -maxdepth 1 -print -quit)" ]; then
  echo "app bundle staging: stage directory must be empty" >&2
  exit 2
fi

"$repository_root/scripts/check-app-bundle-contract.sh"
"$repository_root/scripts/stage-runtime-bundle-linux-arm64.sh" "$stage_root"

app_root="$stage_root/Sluice.AppDir"
mkdir -p \
  "$app_root/usr/libexec/sluice" \
  "$app_root/usr/share/sluice/tachyon/server/routes/v1/health" \
  "$app_root/usr/share/doc/sluice/licenses/Sluice"

cd "$repository_root"
GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
  go build -trimpath -o "$app_root/AppRun" ./cmd/sluice-desktop
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -o "$app_root/usr/libexec/sluice/sluice-agent" ./cmd/sluice-agent
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -o "$app_root/usr/libexec/sluice/sluice-yon" ./internal/transport/yon/cmd/sluice-yon

cp "$repository_root/packaging/linux/sluice.desktop" "$app_root/sluice.desktop"
cp "$repository_root/packaging/tachyon/server/routes/v1/health/yon.sluice" \
  "$app_root/usr/share/sluice/tachyon/server/routes/v1/health/yon.sluice"
cp "$repository_root/LICENSE" "$app_root/usr/share/doc/sluice/licenses/Sluice/LICENSE"

chmod 0755 \
  "$app_root/AppRun" \
  "$app_root/usr/libexec/sluice/sluice-agent" \
  "$app_root/usr/libexec/sluice/sluice-yon"

for program in \
  "$app_root/AppRun" \
  "$app_root/usr/libexec/sluice/sluice-agent" \
  "$app_root/usr/libexec/sluice/sluice-yon"; do
  case "$(file -b "$program")" in
    *"ELF 64-bit"*"ARM aarch64"*) ;;
    *) echo "app bundle staging: $(basename -- "$program") is not ELF aarch64" >&2; exit 1 ;;
  esac
done

if command -v desktop-file-validate >/dev/null 2>&1; then
  desktop-file-validate "$app_root/sluice.desktop"
fi
if ldd "$app_root/AppRun" | grep -q 'not found'; then
  echo "app bundle staging: desktop has unresolved shared libraries" >&2
  exit 1
fi

test -x "$app_root/usr/lib/sluice/runtime/bin/ty"
test -x "$app_root/usr/lib/sluice/runtime/bin/fylo"
test -f "$app_root/usr/share/doc/sluice/licenses/Sluice/LICENSE"
test -f "$app_root/usr/share/doc/sluice/licenses/Tachyon/LICENSE"
test -f "$app_root/usr/share/doc/sluice/licenses/Tachyon/NOTICE"
test -f "$app_root/usr/share/doc/sluice/licenses/Fylo/LICENSE"

echo "app bundle staging: unsigned Linux arm64 Sluice.AppDir passed in $stage_root"
