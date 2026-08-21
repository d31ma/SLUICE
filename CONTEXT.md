# Sluice Domain Context

## Purpose

Sluice is a local-first file synchronization client for user-controlled
S3-compatible storage. It provides continuous desktop synchronization, visible
conflict handling, crash recovery, and later platform-native placeholder
files, without making a hosted Sluice account mandatory.

## Domain terms

- **Profile**: one S3-compatible endpoint, region, bucket, object prefix,
  behavior policy, and opaque credential reference.
- **Root**: one canonical local filesystem boundary synchronized through a
  profile.
- **Logical path**: a normalized root-relative path used by the Sluice remote
  protocol. It is not an untrusted absolute filesystem path.
- **Entry**: the current observed state of one local logical path.
- **Revision**: a versioned observation of local or remote content and metadata.
- **Content object**: immutable bytes stored remotely by SHA-256 digest.
- **Path manifest**: the conditionally written remote mapping from a logical
  path to an immutable content object and revision.
- **Tombstone**: a retained, versioned deletion record.
- **Operation**: a durable, idempotent upload, download, delete, reconcile, or
  materialization state machine.
- **Conflict**: local and remote revisions that both diverged from their last
  common revision.
- **Reconciliation**: deterministic comparison of local, last-synced, and
  remote state that produces durable operations or conflicts.
- **Hydration**: materializing remote content into a platform placeholder.
- **Eviction**: removing resident bytes while retaining a recoverable
  placeholder and remote content reference.
- **Fylo root**: Sluice's local control-state store, owned by one persistent
  agent and kept outside every synchronized root.
- **Support tier**: buildable, native-tested, preview, or supported evidence
  for one target or integration.

## Relationships

1. A profile owns zero or more roots.
2. A root owns entries, checkpoints, operations, conflicts, and tombstones.
3. An entry refers to local, last-synced, and remote revisions.
4. A path manifest refers to one immutable content object.
5. A queue claim refers to one operation ledger record and carries a fencing
   receipt.
6. The Fyne application calls the loopback Yon API.
7. Yon handlers bridge low-frequency commands to the persistent Go agent.
8. The agent is the sole Fylo writer and performs filesystem and S3 work.

## Non-negotiable invariants

- Conflicts preserve both divergent versions.
- Timestamps alone never choose a conflict winner.
- An external side effect is never considered scheduled before its operation
  is durable in Fylo.
- An operation is never considered complete before its local or remote
  postcondition is verified.
- Content objects are immutable and content-addressed.
- Path manifests use conditional writes.
- Unknown local, API, Fylo, or remote protocol versions fail closed.
- Watcher events are hints; an authoritative scan repairs missed events.
- One persistent agent owns one Fylo root with exclusive-root fencing.
- Credentials are stored by native secret facilities; Fylo stores references.
- User content is not duplicated into Fylo.
- Cross-compilation is not evidence of native support.

## Resolved architecture decisions

- Go is the primary language.
- Tachyon Yon is the REST and supervised-handler boundary.
- Fyne is the shared desktop UI and tray framework.
- Fylo is the local control-state store.
- S3-compatible storage is the remote content medium.
- Ordinary-folder sync precedes placeholder integration.

## Explicit ambiguities

- Provider qualification beyond AWS S3, MinIO, and the first additional
  Phase 1 provider.
- The production client-side encryption design.
- Exact mobile process topology pending physical-device evidence.
- Linux placeholder support outside qualified FUSE environments.
