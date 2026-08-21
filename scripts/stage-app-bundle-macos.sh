#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)

if [ "$(uname -s)" != Darwin ] || [ "$(uname -m)" != arm64 ]; then
  echo "app bundle staging: requires a native Darwin arm64 host" >&2
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

"$repo_root/scripts/check-app-bundle-contract.sh"
"$repo_root/scripts/stage-runtime-bundle-macos.sh" "$stage_root"

app_root="$stage_root/Sluice.app"
contents="$app_root/Contents"
mkdir -p "$contents/MacOS" "$contents/Helpers"

cd "$repo_root"
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
  go build -trimpath -o "$contents/MacOS/sluice-desktop" ./cmd/sluice-desktop
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -o "$contents/Helpers/sluice-agent" ./cmd/sluice-agent
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -o "$contents/Helpers/sluice-yon" ./internal/transport/yon/cmd/sluice-yon

cp "$repo_root/packaging/macos/Info.plist" "$contents/Info.plist"
mkdir -p "$contents/Resources/tachyon/server/routes/v1/health"
cp "$repo_root/packaging/tachyon/server/routes/v1/health/yon.sluice" \
  "$contents/Resources/tachyon/server/routes/v1/health/yon.sluice"
mkdir -p "$contents/Resources/licenses/Sluice"
cp "$repo_root/LICENSE" "$contents/Resources/licenses/Sluice/LICENSE"

chmod 0755 \
  "$contents/MacOS/sluice-desktop" \
  "$contents/Helpers/sluice-agent" \
  "$contents/Helpers/sluice-yon"

for program in \
  "$contents/MacOS/sluice-desktop" \
  "$contents/Helpers/sluice-agent" \
  "$contents/Helpers/sluice-yon"; do
  case "$(file -b "$program")" in
    *"Mach-O 64-bit executable arm64"*) ;;
    *) echo "app bundle staging: $(basename -- "$program") is not Mach-O arm64" >&2; exit 1 ;;
  esac
done

plutil -lint "$contents/Info.plist" >/dev/null
test -x "$contents/Resources/runtime/bin/ty"
test -x "$contents/Resources/runtime/bin/fylo"
test -f "$contents/Resources/licenses/Sluice/LICENSE"
test -f "$contents/Resources/licenses/Tachyon/LICENSE"
test -f "$contents/Resources/licenses/Tachyon/NOTICE"
test -f "$contents/Resources/licenses/Fylo/LICENSE"

echo "app bundle staging: unsigned macOS arm64 Sluice.app passed in $stage_root"
