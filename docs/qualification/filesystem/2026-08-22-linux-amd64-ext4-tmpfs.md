# Linux amd64 ext4/tmpfs Filesystem: 2026-08-22

## Result

Pass for the Phase 1 Linux amd64 filesystem scope. This record qualifies the
secure atomic installer on the named native ext4 and tmpfs environment; it is
not a graphical desktop, distribution-package, installer, Preview, or
Supported claim.

## Environment

| Item | Value |
| --- | --- |
| CI run | [32548613814](https://github.com/d31ma/sluice/actions/runs/32548613814), job `96971534860` (`Go ubuntu-latest`) |
| Sluice source | `0760adf1b362e04cd52f25d2c781217210734148` |
| Host | Ubuntu 24.04.4 LTS, Linux 6.17.0-1022-azure, x86_64 |
| Runner image | `ubuntu-24.04` version `20260816.277.1` |
| Go toolchain | Go 1.27.0, `linux/amd64` |
| Ordinary fixture filesystem | ext4 runner temporary directory |
| Pressure fixture filesystem | Isolated 64 MiB tmpfs |

The shell harness verified the native architecture and both filesystem types,
mounted the bounded tmpfs, ran the suite, unmounted it, and removed only its
validated runner-temporary scratch directory.

## Command and observations

The hosted workflow executed:

```sh
./scripts/test-filesystem-linux-amd64.sh
```

The harness required all native link and Linux fixtures and recorded:

```text
Linux amd64 qualification host filesystem: ext4
Linux amd64 qualification pressure filesystem: tmpfs
Linux amd64 qualification Go: go version go1.27.0 linux/amd64
ok github.com/d31ma/sluice/internal/filesystem 2.056s
PASS TestAtomicInstallerCleansUpAfterRealENOSPC
PASS TestAtomicInstallerRecoversAfterPreRenameTermination
filesystem Linux amd64 qualification: ext4, tmpfs ENOSPC, and pre-rename
recovery passed
```

The same native job also passed the repository-wide race suite,
`go vet ./...`, and `go build ./...`.

## Native properties proved

- symbolic-link parents and linked destinations fail closed;
- open directory handles contain a deterministic hostile parent swap and
  prevent writes through the replacement link;
- permission denial maps to the stable installer failure contract;
- competing installers preserve `renameat2(RENAME_NOREPLACE)` semantics;
- an actually full tmpfs produces a partial-write failure with neither a
  destination nor an abandoned unowned temporary file;
- forced child termination during streaming and after the complete temporary
  payload exists but before rename is recovered on retry;
- a user-created file resembling a Sluice temporary name is not removed;
- the unsupported-filesystem rename result is classified without falling back
  to a racy replacement; and
- the native race detector reports no data race in the filesystem package.

## Boundary

This evidence applies to the named local ext4 and tmpfs environment. Network,
object-backed, FUSE, independently synchronized, and other Linux filesystem
types are not inferred from it. Real graphical desktop tray/notification,
real-login, and accessibility gates remain open and are tracked separately.
