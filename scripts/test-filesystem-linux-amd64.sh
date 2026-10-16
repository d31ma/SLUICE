#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Linux" || "$(uname -m)" != "x86_64" ]]; then
  echo "filesystem Linux amd64 qualification requires native Linux x86_64" >&2
  exit 1
fi
if ! command -v sudo >/dev/null 2>&1; then
  echo "filesystem Linux amd64 qualification requires sudo for an isolated tmpfs" >&2
  exit 1
fi

repository_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
scratch_parent=${RUNNER_TEMP:-${TMPDIR:-/tmp}}
scratch_root=$(mktemp -d "$scratch_parent/sluice-filesystem-linux-amd64.XXXXXX")
host_tmp="$scratch_root/host-tmp"
tmpfs_root="$scratch_root/enospc"
test_binary="$scratch_root/filesystem.test"
mounted=false

cleanup() {
  if [[ "$mounted" == true ]] && mountpoint -q -- "$tmpfs_root"; then
    sudo umount -- "$tmpfs_root"
  fi
  rm -f -- "$test_binary"
  rmdir -- "$tmpfs_root" "$host_tmp" "$scratch_root" 2>/dev/null || true
}
trap cleanup EXIT

mkdir -p -- "$host_tmp" "$tmpfs_root"
host_filesystem=$(findmnt -n -o FSTYPE --target "$scratch_root")
case "$host_filesystem" in
  ext2|ext3|ext4) ;;
  *)
    echo "filesystem Linux amd64 qualification requires ext2/ext3/ext4, got $host_filesystem" >&2
    exit 1
    ;;
esac

sudo mount -t tmpfs \
  -o "size=64m,nodev,nosuid,noexec,mode=0700,uid=$(id -u),gid=$(id -g)" \
  tmpfs "$tmpfs_root"
mounted=true
if [[ "$(findmnt -n -o FSTYPE --target "$tmpfs_root")" != "tmpfs" ]]; then
  echo "filesystem Linux amd64 qualification did not mount tmpfs" >&2
  exit 1
fi

cd -- "$repository_root"
echo "Linux amd64 qualification kernel: $(uname -srmo)"
echo "Linux amd64 qualification host filesystem: $host_filesystem"
echo "Linux amd64 qualification pressure filesystem: $(findmnt -n -o FSTYPE --target "$tmpfs_root")"
echo "Linux amd64 qualification Go: $(go version)"

go test -c -o "$test_binary" ./internal/filesystem
file "$test_binary" | grep -Eq 'ELF 64-bit.*x86-64'
TMPDIR="$host_tmp" go test -race -count=1 ./internal/filesystem
TMPDIR="$host_tmp" \
SLUICE_FS_ENOSPC_ROOT="$tmpfs_root" \
SLUICE_FS_REQUIRE_RENAME_BOUNDARY=1 \
  "$test_binary" \
    -test.run '^TestAtomicInstaller(CleansUpAfterRealENOSPC|RecoversAfterPreRenameTermination)$' \
    -test.count=1 \
    -test.v

echo "filesystem Linux amd64 qualification: ext4, tmpfs ENOSPC, and pre-rename recovery passed"
