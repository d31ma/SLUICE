# Filesystem qualification record

Date: 2026-08-21

## Environment exercised

| Item | Value |
| --- | --- |
| Host | macOS 26.5.2 (25F84), Apple Silicon |
| Go toolchain | Go 1.27.0 darwin/amd64 under translation |
| Native temporary filesystem | APFS via `testing.T.TempDir` |
| Linux evidence | amd64 test binary compiled only |
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
  rollback without consuming the host filesystem.

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
```

## Native gates still required

The Phase 1 filesystem spike is not globally green until the same suite and the
following destructive fixtures run on named native filesystems:

| Target | Required evidence |
| --- | --- |
| Windows | NTFS no-replace race, reparse point and junction rejection, ACL/permission denial, disk-full cleanup, forced termination around `MoveFileEx`, and restart verification |
| macOS | APFS native arm64 run, normalization/case-collision fixtures, real ENOSPC cleanup, and forced termination immediately around `renameatx_np` |
| Linux | ext4 and one additional supported local filesystem, symlink-swap race, permission denial, disk-full cleanup, `renameat2` unsupported-filesystem behavior, forced termination, and restart verification |

Network, object-backed, and independently synchronized roots are outside the
qualified filesystem set. The Fylo root has its own stricter placement gate.

## Qualification decision

The portable path grammar and stable-file atomic installation contract pass on
the recorded macOS/APFS environment. The locally safe permission, cancellation,
kernel write-failure, forced-stream-interruption recovery, and hostile
parent-symlink-swap gates pass. A real disk-full fixture remains necessary
because `RLIMIT_FSIZE` proves write-error cleanup but not APFS ENOSPC behavior.
Windows and Linux compile, but remain unqualified until native execution and
handle-relative containment evidence exist. See
[README.md](README.md#link-safety-boundary).
