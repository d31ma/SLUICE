#!/bin/sh
set -eu

if [ "$(uname -s)" != "Darwin" ] || [ "$(uname -m)" != "arm64" ]; then
  echo "filesystem macOS qualification requires a native arm64 macOS host" >&2
  exit 1
fi

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scratch_root=$(mktemp -d "${TMPDIR:-/tmp}/sluice-filesystem-macos.XXXXXX")
image_path="$scratch_root/enospc.dmg"
mount_path="$scratch_root/apfs"
test_binary="$scratch_root/filesystem.test"
attached=false

cleanup() {
  if [ "$attached" = true ]; then
    hdiutil detach -quiet "$mount_path" || hdiutil detach -force -quiet "$mount_path" || true
  fi
  case "$scratch_root" in
    "${TMPDIR:-/tmp}"/sluice-filesystem-macos.*) rm -R -- "$scratch_root" ;;
    *) echo "refusing to remove unexpected scratch path: $scratch_root" >&2 ;;
  esac
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$mount_path"
hdiutil create -quiet -size 64m -fs APFS -volname SLUICE_ENOSPC -type UDIF "$image_path"
hdiutil attach -quiet -nobrowse -mountpoint "$mount_path" "$image_path"
attached=true

cd "$repository_root"
GOOS=darwin GOARCH=arm64 go test -c -o "$test_binary" ./internal/filesystem
file "$test_binary" | grep -q 'Mach-O 64-bit executable arm64'

GOOS=darwin GOARCH=arm64 go test -count=1 ./internal/filesystem
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC='clang -arch arm64' \
  go test -race -count=1 ./internal/filesystem

SLUICE_FS_ENOSPC_ROOT="$mount_path" \
SLUICE_FS_REQUIRE_APFS_COLLISIONS=1 \
SLUICE_FS_REQUIRE_RENAME_BOUNDARY=1 \
  "$test_binary" \
    -test.run '^TestAtomicInstaller(CleansUpAfterAPFSENOSPC|FailsClosedOnAPFSCollisions|RecoversAfterPreRenameTermination)$' \
    -test.count=1 \
    -test.v

echo "filesystem macOS qualification: native arm64, APFS collisions, real ENOSPC, and pre-rename recovery passed"
