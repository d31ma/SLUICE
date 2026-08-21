# Filesystem contract qualification

This directory records the Phase 1 contract for platform-neutral logical paths
and verified atomic file installation. It is evidence for the filesystem spike,
not a claim that Windows, Linux, or all macOS filesystems are qualified.

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
3. On macOS, the installer opens the root and each parent component with
   `openat`, `O_DIRECTORY`, and `O_NOFOLLOW`, then keeps the resulting directory
   handles through the commit. Unix symbolic links and Windows reparse points
   are rejected. A linked destination is rejected as an unsafe link.
4. Content is streamed into an exclusively created sibling temporary file, so
   the temporary file and destination are on the same filesystem.
5. SHA-256 is calculated while writing and must match `ExpectedSHA256`.
6. The requested permission bits are applied and the temporary file is flushed
   before installation. A platform may keep its handle open through commit.
7. Cancellation is checked before mutation, while streaming, and immediately
   before rename. Any handled pre-rename error removes the temporary file.
8. On macOS, a private extended-attribute marker, per-directory lock, and
   per-temporary-file lock distinguish Sluice-owned live installs from files
   orphaned by process termination. The next install in the directory removes
   only marked, unlocked orphan temporary files before writing. A user file that
   merely resembles the reserved temporary filename is left untouched.
9. A native atomic no-replace rename installs the file. Existing destinations
   are never overwritten, including under competing installers.
10. On POSIX, the destination directory is flushed after rename. On Windows,
    `MoveFileEx` uses `MOVEFILE_WRITE_THROUGH`.

No-replace is intentional. Replacing a cataloged file requires a future
compare-and-swap contract that proves the expected prior revision; it must not
be approximated with a timestamp or a check-then-overwrite sequence.

If the directory flush fails after rename, the call returns
`filesystem.install.io`, but the destination may already exist. The operation
ledger must reconcile destination content before deciding whether to retry.

## Link-safety boundary

The macOS implementation is directory-handle-relative. Immediately before its
`renameatx_np` commit, it reopens the configured root and logical parent through
no-follow handles and compares device and inode identities with the handles that
own the temporary file. A deterministic hostile-swap test renames the opened
parent and replaces its logical path with a symlink while installation is in
progress. The call fails closed, removes the temporary file through the original
directory handle, and writes no bytes through the replacement link.

The Linux and Windows implementations still use path-based component checks.
Their native concurrent-swap gates remain failed until equivalent handle-based
adapters and native evidence exist. Cross-compilation does not qualify those
implementations.

## Stable failures

Public errors expose a stable `ErrorCode`, support `errors.Is` for recovery
classes such as `ErrDestinationExists`, and omit sensitive absolute paths from
their message. Underlying I/O errors remain available through `errors.Is` and
`errors.As` without being formatted into the public error string.

## Evidence

See [QUALIFICATION.md](QUALIFICATION.md) for the exact environment, commands,
results, and remaining native gates.
