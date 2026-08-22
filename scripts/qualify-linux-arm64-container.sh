#!/bin/sh
set -eu

repository_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
image="golang@sha256:6237d172b66951ce3ebfe0156587ad41a9773772c061fb9a5e3d612f5fd22614"

if ! command -v docker >/dev/null 2>&1; then
  echo "Linux qualification: Docker is required" >&2
  exit 1
fi
if [ "$(docker info --format '{{.OSType}}/{{.Architecture}}')" != "linux/aarch64" ]; then
  echo "Linux qualification: requires a Linux arm64 Docker engine without architecture emulation" >&2
  exit 1
fi

docker run --rm --platform linux/arm64 \
  --mount type=bind,src="$repository_root",dst=/source,readonly \
  --mount type=volume,dst=/qualification,volume-nocopy \
  --tmpfs /qualification-tmpfs:rw,exec,size=67108864,mode=1777 \
  "$image" sh -c '
    set -eu
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -qq -y --no-install-recommends \
      ca-certificates curl dbus-x11 desktop-file-utils file procps ruby \
      xauth xvfb libdecor-0-dev libegl1-mesa-dev libgl1-mesa-dev \
      libwayland-dev libxkbcommon-dev libxkbcommon-x11-dev wayland-protocols \
      xorg-dev >/dev/null

    test "$(uname -s)" = Linux
    test "$(uname -m)" = aarch64
    test "$(stat -f -c %T /qualification)" = ext2/ext3
    test "$(stat -f -c %T /qualification-tmpfs)" = tmpfs

    useradd --create-home --uid 10001 qualifier
    cp -a --no-preserve=ownership /source/. /qualification/sluice
    mkdir -p /qualification/home /qualification/runtime-dir /qualification/state /qualification/tmp
    chown -R qualifier:qualifier /qualification
    chmod 0700 /qualification/runtime-dir

    runuser -u qualifier -- env \
      HOME=/qualification/home \
      XDG_RUNTIME_DIR=/qualification/runtime-dir \
      XDG_STATE_HOME=/qualification/state \
      TMPDIR=/qualification/tmp \
      PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin \
      sh -c "
        set -eu
        cd /qualification/sluice
        echo \"Linux qualification kernel: \$(uname -srmo)\"
        echo \"Linux qualification filesystem: \$(stat -f -c %T /qualification)\"
        echo \"Linux qualification glibc: \$(ldd --version | head -1)\"
        echo \"Linux qualification Go: \$(go version)\"

        runtime_stage=/qualification/runtime-stage
        scripts/stage-runtime-bundle-linux-arm64.sh \"\$runtime_stage\"
        export PATH=\"\$runtime_stage/Sluice.AppDir/usr/lib/sluice/runtime/bin:\$PATH\"
        SLUICE_REQUIRE_INSTALLED_RUNTIMES=1 go test -race -count=1 \
          ./internal/runtime ./internal/state/fylo ./internal/transport/ipc \
          ./internal/agent ./internal/transport/localapi ./internal/transport/yon \
          ./cmd/sluice-agent
        go test -race -count=1 ./internal/filesystem ./internal/platform/autostart
        TMPDIR=/qualification-tmpfs go test -race -count=1 ./internal/filesystem
        go test -c -o /qualification/filesystem.test ./internal/filesystem
        SLUICE_FS_ENOSPC_ROOT=/qualification-tmpfs \
        SLUICE_FS_REQUIRE_RENAME_BOUNDARY=1 \
          /qualification/filesystem.test \
            -test.run \"^TestAtomicInstaller(CleansUpAfterRealENOSPC|RecoversAfterPreRenameTermination)\$\" \
            -test.count=1 \
            -test.v
        scripts/test-app-bundle-linux-arm64.sh
      "
  '

echo "Linux qualification: native arm64 runtime, IPC, filesystem, autostart, and AppDir smoke passed"
