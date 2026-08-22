# Filesystem qualification record

Date: 2026-08-21

## Environment exercised

| Item | Value |
| --- | --- |
| Host | macOS 26.5.2 (25F84), Mac16,10 with Apple M4 and arm64 kernel |
| Go toolchain | Go 1.27.0 host tool runs as darwin/amd64 under translation; qualification binaries explicitly target darwin/arm64 |
| Native temporary filesystem | APFS temporary directories plus an isolated 64 MiB APFS disk image for real ENOSPC |
| Sluice source | `f179e8522ff22223d9de51e5828d8bfac9b6203c` |
| Linux evidence | Native arm64 ordinary/race and destructive suites on an ext4-compatible Docker volume plus tmpfs at source `64ae711f9d6ec5b094044ac758ca6858addf92c9`; amd64 compile only |
| Windows evidence | amd64 test executable compiled only |

Cross-compilation proves source compatibility only. It does not promote Linux
or Windows filesystem support.

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
```

## Native gates still required

The macOS arm64 slice passes the recorded Phase 1 filesystem corpus. The spike
is not globally green until equivalent suites run on the named Windows and
Linux filesystems:

| Target | Status and required evidence |
| --- | --- |
| macOS arm64 / APFS | Passed: native ordinary and race suites, no-replace race, handle-relative parent-swap containment, permission denial, real ENOSPC cleanup, case and normalization collisions, and pre-rename termination/restart |
| Windows / NTFS | Pending: native no-replace race, reparse point and junction rejection, ACL/permission denial, disk-full cleanup, forced termination around `MoveFileEx`, and restart verification |
| Linux arm64 / ext4-compatible volume plus tmpfs | Passed: native ordinary/race suites on both filesystems, handle-relative hostile parent swap, permission denial, real tmpfs ENOSPC cleanup, `renameat2(RENAME_NOREPLACE)`, unsupported-filesystem error mapping, and pre-rename termination/restart |
| Linux amd64 | Pending: repeat the complete Linux corpus natively |

Network, object-backed, and independently synchronized roots are outside the
qualified filesystem set. The Fylo root has its own stricter placement gate.

## Qualification decision

The portable path grammar, conservative collision key, and stable-file atomic
installation contract pass on the recorded native arm64 macOS/APFS
environment. The permission, cancellation, real ENOSPC, collision,
forced-pre-rename recovery, and hostile parent-swap gates pass on macOS arm64
and Linux arm64 in their recorded filesystems. Windows and Linux amd64 remain
compile-only.

The installer's documented ambiguous outcome after a successful rename but a
failed directory flush remains an input to the later durable operation-ledger
and reconciliation slices. This narrow Phase 1 installer record does not claim
that later recovery workflow is implemented. See
[README.md](README.md#link-safety-boundary).
