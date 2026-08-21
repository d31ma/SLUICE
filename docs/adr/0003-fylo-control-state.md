# ADR 0003: Use Fylo for Authoritative Local Control State

- Status: Accepted
- Date: 2026-08-21

## Context

Sluice needs a recoverable catalog, checkpoints, operation ledger, retry queue,
tombstones, conflicts, and redacted events. The project requires Fylo instead
of SQLite. Fylo provides a Go shim, versioned bounded machine protocol,
filesystem-first records, rebuildable indexes, durable queues, and
exclusive-root ownership.

## Decision

The persistent agent owns one warm, version-pinned Fylo process and production
root. It requires a successful handshake, known capabilities, bounded paging,
and `--exclusive-root`. Sluice stores control documents in named collections
and queue payloads containing only operation IDs and fencing data.

User file bytes remain in their ordinary local paths and remote content
objects. Fylo's optional S3/GCS hooks remain disabled for the Sluice state root.
Credentials remain in OS secret storage; Fylo holds opaque references.

Phase 1 must add or consume a Fylo Go-shim option for `--exclusive-root`.

## Consequences

- Fylo format and machine compatibility become release dependencies.
- The agent is the only permitted state writer.
- Collection design avoids arrays of objects and uses IDs for relationships.
- Queries and exports must be paged within negotiated frame limits.
- The state root must live on a qualified local filesystem outside sync roots.

## Rejected alternatives

- SQLite: superseded by product direction.
- Storing state in S3: remote availability cannot gate local recovery.
- Reconstructing everything from S3 and filesystem on every launch: too slow
  and cannot preserve durable operation intent.
- Storing all content in Fylo raw-file buckets: duplicates user bytes and
  changes the ordinary-folder product model.
