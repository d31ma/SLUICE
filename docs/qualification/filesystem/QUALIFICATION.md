# Filesystem qualification record

Evidence through: 2026-08-22

## Environment exercised

| Item | Value |
| --- | --- |
| Host | macOS 26.5.2 (25F84), Mac16,10 with Apple M4 and arm64 kernel |
| Go toolchain | Go 1.27.0 host tool runs as darwin/amd64 under translation; qualification binaries explicitly target darwin/arm64 |
| Native temporary filesystem | APFS temporary directories plus an isolated 64 MiB APFS disk image for real ENOSPC |
| Sluice source | `f179e8522ff22223d9de51e5828d8bfac9b6203c` |
| Linux evidence | Native arm64 ordinary/race and destructive suites on an ext4-compatible Docker volume plus tmpfs at source `64ae711f9d6ec5b094044ac758ca6858addf92c9`; native amd64 repeat on ext4 plus tmpfs at source `0760adf1b362e04cd52f25d2c781217210734148` |
| Windows evidence | Native amd64 ordinary/race and destructive suites on NTFS at source `0760adf1b362e04cd52f25d2c781217210734148` |

Cross-compilation still proves source compatibility only. The promoted Linux
amd64 and Windows amd64 results below come from native execution.

## Contract corpus

The automated corpus covers:

- canonical single- and multi-segment paths and UTF-8 names;
- empty, absolute Unix, absolute Windows drive, and UNC-style input;
- slash ambiguity, empty segments, dot segments, invalid UTF-8, controls,
  forbidden characters, trailing dots/spaces, and reserved device names;
- absolute and relative root configuration, root-symlink resolution, missing
  roots, and non-directory roots;
- successful verified install, sibling temporary placement, permission mode,
  digest mismatch, source failure, cancellation cleanup, and macOS permission
  loss;
- rejection of linked parent and linked destination fixtures;
- rejection of missing parent directories; and
- concurrent installers proving exactly one no-replace rename succeeds;
- a deterministic macOS parent-component swap during streaming, proving the
  replacement symlink receives no write and the original temporary is removed;
- forced process termination during streaming followed by locked-orphan cleanup
  and a successful retry; and
- a kernel-enforced 1 KiB file-size ceiling producing a write failure and clean
  rollback without consuming the host filesystem;
- conservative NFC plus default-case-fold collision keys that retain both
  original logical paths;
- real APFS case and NFC/NFD collisions that preserve the first file and reject
  the alias without replacement;
- a partial write ending in kernel `ENOSPC` on an isolated APFS image, leaving
  neither a destination nor a temporary file; and
- forced termination after the complete temporary payload exists but before
  `renameatx_np`, followed by ownership-aware orphan cleanup and a successful
  retry.

## Commands and results

```text
go test ./internal/filesystem
PASS

go test -race -count=1 ./internal/filesystem
PASS

go vet ./internal/filesystem
PASS

go tool golangci-lint run ./internal/filesystem/...
PASS (0 issues)

go tool go-crap scan ./internal/filesystem/... --fail-above --threshold 15 \
  --min 15 --missing pessimistic --no-progress
PASS (0 functions above 15; average 3.87)

GOOS=linux GOARCH=amd64 go test -c -o /tmp/sluice-filesystem-linux.test ./internal/filesystem
PASS (compile only)

GOOS=windows GOARCH=amd64 go test -c -o /tmp/sluice-filesystem-windows.test.exe ./internal/filesystem
PASS (compile only)

make filesystem-qualify-macos
PASS
ok github.com/d31ma/sluice/internal/filesystem (ordinary native arm64 suite)
ok github.com/d31ma/sluice/internal/filesystem (race-enabled native arm64 suite)
PASS TestAtomicInstallerCleansUpAfterRealENOSPC
PASS TestAtomicInstallerFailsClosedOnAPFSCollisions/case
PASS TestAtomicInstallerFailsClosedOnAPFSCollisions/normalization
PASS TestAtomicInstallerRecoversAfterPreRenameTermination
filesystem macOS qualification: native arm64, APFS collisions, real ENOSPC,
and pre-rename recovery passed

make linux-qualify-arm64
PASS
ok github.com/d31ma/sluice/internal/filesystem (race-enabled Linux arm64 on ext4-compatible volume)
ok github.com/d31ma/sluice/internal/filesystem (race-enabled Linux arm64 on tmpfs)
PASS TestAtomicInstallerCleansUpAfterRealENOSPC
PASS TestAtomicInstallerRecoversAfterPreRenameTermination
# Hostile parent swap, marked-orphan recovery, permission denial,
# renameat2(RENAME_NOREPLACE), and EINVAL unsupported-filesystem mapping passed

make filesystem-qualify-linux-amd64
PASS
ok github.com/d31ma/sluice/internal/filesystem (race-enabled Linux amd64 on ext4)
PASS TestAtomicInstallerCleansUpAfterRealENOSPC (isolated tmpfs)
PASS TestAtomicInstallerRecoversAfterPreRenameTermination
filesystem Linux amd64 qualification: ext4, real tmpfs ENOSPC, pre-rename
recovery, and race suites passed

make filesystem-qualify-windows
PASS
ok github.com/d31ma/sluice/internal/filesystem (race-enabled Windows amd64 on NTFS)
filesystem Windows qualification: NTFS reparse, junction, ACL, disk-full,
pre-rename recovery, and race suites passed
```

The hosted commands above are recorded in
[Linux amd64 ext4/tmpfs](2026-08-22-linux-amd64-ext4-tmpfs.md) and
[Windows amd64 NTFS](2026-08-22-windows-amd64-ntfs.md).

## Native results

The named Phase 1 filesystem spike is green on its recorded native environments:

| Target | Status and required evidence |
| --- | --- |
| macOS arm64 / APFS | Passed: native ordinary and race suites, no-replace race, handle-relative parent-swap containment, permission denial, real ENOSPC cleanup, case and normalization collisions, and pre-rename termination/restart |
| Windows amd64 / NTFS | Passed: native ordinary/race suites, handle-relative parent-swap containment, reparse point and junction rejection, ACL denial, real VHD disk-full cleanup, no-replace rename, and forced pre-rename termination/restart |
| Linux arm64 / ext4-compatible volume plus tmpfs | Passed: native ordinary/race suites on both filesystems, handle-relative hostile parent swap, permission denial, real tmpfs ENOSPC cleanup, `renameat2(RENAME_NOREPLACE)`, unsupported-filesystem error mapping, and pre-rename termination/restart |
| Linux amd64 / ext4 plus tmpfs | Passed: native ordinary/race suites, handle-relative hostile parent swap, permission denial, real tmpfs ENOSPC cleanup, `renameat2(RENAME_NOREPLACE)`, unsupported-filesystem error mapping, and pre-rename termination/restart |

Network, object-backed, and independently synchronized roots are outside the
qualified filesystem set. The Fylo root has its own stricter placement gate.

## Qualification decision

The portable path grammar, conservative collision key, and stable-file atomic
installation contract pass on the recorded native macOS arm64/APFS, Linux
arm64/amd64 ext4-compatible and tmpfs, and Windows amd64/NTFS environments.
The permission, cancellation, real ENOSPC, no-replace, link/reparse,
forced-pre-rename recovery, and hostile parent-swap properties execute through
the adapters applicable to each named platform.

The installer's documented ambiguous outcome after a successful rename but a
failed directory flush remains an input to the later durable operation-ledger
and reconciliation slices. This narrow Phase 1 installer record does not claim
that later recovery workflow is implemented. See
[README.md](README.md#link-safety-boundary).
