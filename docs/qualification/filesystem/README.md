# Filesystem contract qualification

This directory records the Phase 1 contract for platform-neutral logical paths
and verified atomic file installation. The named native APFS, NTFS,
ext4-compatible, and tmpfs environments have passed; that evidence is not a
claim about every filesystem available on Windows, Linux, or macOS.

## Logical-path contract

`filesystem.ParseLogicalPath` accepts one canonical wire form:

- the path is relative to an already configured synchronization root;
- `/` is the only separator and is preserved as written;
- every segment is non-empty and is neither `.` nor `..`;
- input is valid UTF-8 and contains no Unicode control character;
- Windows-forbidden characters `<`, `>`, `:`, `"`, `|`, `?`, and `*` are
  forbidden on every target;
- `\` is forbidden rather than interpreted as a separator;
- a segment may not end in a dot or space; and
- Windows device names are forbidden case-insensitively, including when an
  extension is present.

The common-denominator policy intentionally rejects names that Linux or macOS
could store but Windows could not represent safely. The parser does not silently
clean, normalize, decode, or change a path. Rejected input must remain inert and
surface its stable `filesystem.path.*` error code.

Unicode normalization and case folding are not performed by the parser because
they can merge distinct user names. A catalog or reconciliation layer must
detect target-specific case and normalization collisions and surface a conflict
before scheduling an installation.

`filesystem.PortableCollisionKey` supplies that conservative catalog identity.
It applies Unicode NFC followed by default Unicode case folding while retaining
the original `LogicalPath` unchanged. Distinct original names sharing one key
must be preserved as a conflict; the key is never a filename and never selects
a winner. The corpus covers ASCII case, NFC/NFD equivalence, German sharp-s,
Greek final sigma, and distinct non-colliding names.

## Physical-root contract

`filesystem.CanonicalizeRoot` accepts absolute or relative configuration input,
requires an existing directory, resolves the configured root once, and returns
an absolute clean path. Resolving a root symlink is a configuration-time action;
it does not authorize following links below that root.

Only a `LogicalPath` can be resolved beneath a canonical `Root`, which prevents
callers from passing an unchecked remote string to an installation operation.
Absolute logical paths, Windows drive paths, UNC-style paths, and traversal
segments never reach host path joining.

## Atomic-install contract

`AtomicInstaller.Install` has deliberately narrow semantics:

1. The destination parent must already exist.
2. Every existing component from the canonical root to that parent is opened or
   inspected without following links.
3. On macOS and Linux, the installer opens the root and each parent component
   with `openat`, `O_DIRECTORY`, and `O_NOFOLLOW`. On Windows it uses
   root-relative `NtCreateFile` calls with `OBJ_DONT_REPARSE` and
   `FILE_OPEN_REPARSE_POINT`. Every directory handle remains open through the
   commit, and Windows handles deny delete sharing so an ancestor cannot be
   renamed out from under the install. Unix links, Windows reparse points, and
   linked destinations are rejected.
4. Content is streamed into an exclusively created sibling temporary file, so
   the temporary file and destination are on the same filesystem.
5. SHA-256 is calculated while writing and must match `ExpectedSHA256`.
6. The requested permission bits are applied and the temporary file is flushed
   before installation. A platform may keep its handle open through commit.
7. Cancellation is checked before mutation, while streaming, and immediately
   before rename. Any handled pre-rename error removes the temporary file.
8. On macOS and Linux, a private extended-attribute marker, per-directory lock,
   and per-temporary-file lock distinguish Sluice-owned live installs from
   files orphaned by process termination. Windows uses a cryptographically
   named sidecar marker bound to the temporary file's volume and file identity;
   a live file remains protected by delete-sharing denial. The next install in
   the directory removes only marked, inactive orphan temporary files before
   writing. A user file that merely resembles the reserved temporary filename
   is left untouched.
9. A native atomic no-replace rename installs the file. Existing destinations
   are never overwritten, including under competing installers.
10. On POSIX, the destination directory is flushed after rename. On Windows,
    the already-open temporary handle is renamed relative to the held parent
    handle with `NtSetInformationFile(FileRenameInformation)` and
    `ReplaceIfExists` disabled, then flushed after commit.

No-replace is intentional. Replacing a cataloged file requires a future
compare-and-swap contract that proves the expected prior revision; it must not
be approximated with a timestamp or a check-then-overwrite sequence.

If the directory flush fails after rename, the call returns
`filesystem.install.io`, but the destination may already exist. The operation
ledger must reconcile destination content before deciding whether to retry.

## Link-safety boundary

The macOS and Linux implementations are directory-handle-relative. Immediately
before their `renameatx_np` or `renameat2(RENAME_NOREPLACE)` commit, they reopen
the configured root and logical parent through no-follow handles and compare
device and inode identities with the handles that own the temporary file. A
deterministic hostile-swap test renames the opened parent and replaces its
logical path with a symlink while installation is in progress. The call fails
closed, removes the marked temporary file through the original directory
handle, and writes no bytes through the replacement link.

The Windows adapter holds a no-reparse handle for every logical parent and
denies delete sharing until the handle-relative rename completes. Its native
NTFS ordinary/race, parent-swap, reparse and junction, interruption, ACL, and
disk-pressure suites passed on the recorded Windows Server 2025 amd64 host.

## Stable failures

Public errors expose a stable `ErrorCode`, support `errors.Is` for recovery
classes such as `ErrDestinationExists`, and omit sensitive absolute paths from
their message. Underlying I/O errors remain available through `errors.Is` and
`errors.As` without being formatted into the public error string.

## Evidence

The native macOS qualification builds and executes a true arm64 test binary,
runs the ordinary and race suites, and mounts a disposable 64 MiB APFS image
for the destructive disk-full fixture. The same run proves APFS case and
NFC/NFD collision behavior and kills a child after its complete temporary
payload exists but before rename, then verifies orphan cleanup and recovery.
It neither modifies the host filesystem layout nor promotes another target.

The Linux arm64 qualification runs the same shared secure installer as an
unprivileged user on an ext4-compatible volume and tmpfs. It proves hostile
parent-swap containment, real tmpfs ENOSPC cleanup, unsupported-rename error
classification, and pre-rename forced-termination recovery.

The Linux amd64 hosted qualification repeats the destructive corpus on a local
ext4 scratch filesystem and isolated tmpfs. The Windows amd64 hosted
qualification uses an isolated NTFS VHDX and additionally requires native
reparse-point, junction, ACL, disk-full, and process-termination fixtures.

See [QUALIFICATION.md](QUALIFICATION.md) for the exact environment, command,
observations, and scope. Platform-specific hosted records are
[Windows amd64 NTFS](2026-08-22-windows-amd64-ntfs.md) and
[Linux amd64 ext4/tmpfs](2026-08-22-linux-amd64-ext4-tmpfs.md).
