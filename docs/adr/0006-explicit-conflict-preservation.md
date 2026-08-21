# ADR 0006: Preserve and Surface Every Conflict

- Status: Accepted
- Date: 2026-08-21

## Context

Local and remote revisions may both change after their last common revision.
Choosing by timestamp, upload order, or device priority can silently discard a
user version. A sync client must preserve recoverability even when it cannot
infer intent.

## Decision

Three-way reconciliation creates a conflict when local and remote revisions
both diverge and are not content-identical. Sluice preserves the local path,
materializes the remote version under a deterministic conflict filename,
records both revisions in Fylo, and offers keep-local, keep-remote, keep-both,
or manual resolution.

Unrelated paths continue synchronizing. Resolution is explicit, idempotent,
and itself represented as a durable operation.

## Consequences

- Conflicts consume additional local and remote storage until resolved.
- Users receive actionable notifications without global sync deadlock.
- Tests must cover edit/edit, delete/edit, rename/edit, case collisions, and
  offline tombstones.
- No timestamp-only code path is permitted.

## Rejected alternatives

- Last writer wins: loses data under clock skew and offline operation.
- Always keep local or remote: device identity does not express user intent.
- Block all synchronization: unrelated work should continue.
