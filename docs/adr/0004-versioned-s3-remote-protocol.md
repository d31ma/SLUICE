# ADR 0004: Use a Versioned Content-Addressed S3 Protocol

- Status: Accepted
- Date: 2026-08-21

## Context

Object listings, mutable path keys, timestamps, and last-writer-wins updates do
not form a safe bidirectional synchronization protocol. Sluice must tolerate
offline devices, ambiguous responses, provider differences, renames, deletes,
and concurrent edits without silent loss.

## Decision

Use an independently versioned `.sluice/v1` namespace with:

- immutable content objects addressed by SHA-256;
- conditionally written path manifests referencing content hashes;
- retained, versioned tombstones;
- root descriptors and device/revision identity; and
- garbage collection that is dry-run-first and proves no retained reference.

Unknown protocol versions fail closed. Listing is discovery, not the sole
conflict clock. Manifest writes use the last observed ETag or provider version
ID. Canonical JSON schemas and path rules are public contracts.

## Consequences

- Unchanged content can be deduplicated across paths and devices.
- Logical rename changes manifests without uploading content again.
- Orphaned immutable objects require verified garbage collection.
- Provider conditional-write behavior must pass a qualification suite.
- Remote format migration is separate from application release versioning.

## Rejected alternatives

- Mirror local paths directly to mutable keys: concurrent changes overwrite.
- Timestamp winner: clocks are not a safe causal order.
- Provider versioning alone: not all compatible providers expose equivalent
  behavior.
- Listing-only reconciliation: listings do not encode last common revision.
