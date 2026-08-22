# Windows amd64 NTFS Filesystem: 2026-08-22

## Result

Pass for the Phase 1 Windows filesystem scope. This record qualifies the
secure atomic installer on the named native Windows amd64 and NTFS environment;
it is not a Windows desktop, runtime-package, installer, Preview, or Supported
claim.

## Environment

| Item | Value |
| --- | --- |
| CI run | [32548613814](https://github.com/d31ma/sluice/actions/runs/32548613814), job `96971534800` (`Go windows-latest`) |
| Sluice source | `0760adf1b362e04cd52f25d2c781217210734148` |
| Host | Microsoft Windows Server 2025 Datacenter, 10.0.26100, AMD64 |
| Runner image | `windows-2025-vs2026` version `20260818.207.1` |
| Go toolchain | Go 1.27.0, `windows/amd64` |
| Qualification filesystem | NTFS on an isolated disposable 64 MiB VHDX |
| Disk utility | DiskPart 10.0.26100.1150 |

The PowerShell harness selected an unused drive letter, created and attached
the VHDX, created one primary partition, formatted it as NTFS, ran the suite,
detached it, and removed only its validated runner-temporary scratch directory.

## Command and observations

The hosted workflow executed:

```powershell
./scripts/test-filesystem-windows.ps1
```

The harness required all native fixtures and ran:

```text
go test -race -count=1 ./internal/filesystem
ok github.com/d31ma/sluice/internal/filesystem 5.071s
filesystem Windows qualification: NTFS reparse, junction, ACL, disk-full,
pre-rename recovery, and race suites passed
```

The same native job also passed the repository-wide race suite, the Windows
filesystem CRAP ceiling of 15 (no functions listed above the threshold;
average 3.70), `go vet ./...`, and `go build ./...`.

## Native properties proved

- symbolic-link and junction/reparse parents fail closed;
- held no-reparse directory handles contain a deterministic hostile parent
  swap and prevent writes through the replacement path;
- current-user DACL denial returns a stable safe failure and the fixture ACL is
  restored;
- two competing installers preserve no-replace semantics;
- an actually full NTFS VHD produces a partial-write failure with neither a
  destination nor an abandoned unowned temporary file;
- forced child termination during streaming and after the complete temporary
  payload exists but before the handle-relative rename is recovered on retry;
- a user-created file resembling a Sluice temporary name is not removed; and
- the native race detector reports no data race in the filesystem package.

## Boundary

This evidence applies to local NTFS on this native Windows amd64 environment.
Network shares, ReFS, FAT-family filesystems, object-backed roots, and roots
managed by an independent synchronization client are not inferred from it.
The separate Windows packaged runtime remains blocked by the pinned Fylo
artifact's machine-access capability; interactive tray, autostart, real-login,
notification, and accessibility gates also remain open.
