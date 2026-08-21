# Architecture and Data Flow

## Components

| Component | Owns | Must not own |
| --- | --- | --- |
| `sluice-desktop` | Fyne windows, tray, notifications, desktop lifecycle | S3 transfers, Fylo root, reconciliation |
| Tachyon/Yon | Loopback REST routing, request bounds, response mapping | Watchers, transfers, Fylo ownership |
| `sluice-agent` | Reconciliation, durable operations, workers, filesystem and S3 adapters | UI widgets, credential values in state |
| `fylo` | Local control documents, indexes, transactions, queue | User content copy, credentials, remote protocol |
| S3 provider | Immutable content objects, manifests, tombstones | Local operation intent |

## Startup

1. The desktop application acquires the single-desktop-instance boundary.
2. It resolves the native secret reference for the local API token.
3. It starts or connects to `sluice-agent` through a private local endpoint.
4. The agent canonicalizes and validates its Fylo root.
5. The agent starts the version-pinned Fylo child with exclusive-root mode.
6. The agent performs a side-effect-free handshake and validates capabilities.
7. The desktop starts Tachyon/Yon on a random loopback port with bounded,
   allowlisted child environment.
8. The UI authenticates, reads `/v1/version` and `/v1/status`, and becomes
   interactive.
9. The agent resumes durable operations and schedules reconciliation.

Any failure before step 8 produces a recoverable UI error without modifying
user files or remote objects.

## Local observation to upload

1. A watcher hint or scan observes a logical path.
2. The filesystem adapter reads metadata without following unsafe links.
3. The agent compares the observation with the Fylo entry and checkpoint.
4. The agent hashes changed regular-file bytes under a bounded policy.
5. Reconciliation creates a durable upload operation and idempotency key.
6. Fylo records the operation; its queue receives the operation ID.
7. A worker claims the operation with a fencing receipt.
8. The S3 adapter uploads or reuses the immutable content object.
9. The S3 adapter conditionally writes the path manifest.
10. The agent verifies the remote postcondition and completes the operation.

## Remote observation to download

1. Reconciliation pages manifests and tombstones from the last safe cursor.
2. It validates protocol version, schema, logical path, hashes, and revision.
3. It compares remote, last-synced, and local revisions.
4. It records a download operation or explicit conflict in Fylo.
5. A worker downloads to a same-filesystem temporary path.
6. The worker verifies length and SHA-256.
7. The platform adapter atomically installs bytes without following links.
8. The agent verifies local state and completes the operation.

## Conflict flow

1. Three-way reconciliation detects divergent non-identical revisions.
2. Fylo records both revision references before materialization.
3. Remote bytes are verified and installed under a deterministic conflict name.
4. The UI receives a conflict event and offers explicit resolutions.
5. Resolution becomes a durable idempotent operation.

## Shutdown

1. Desktop Quit or OS shutdown stops accepting new UI mutations.
2. The agent stops claiming new operations and cancels bounded external work.
3. In-flight operations retain a recoverable durable state.
4. The agent closes the Fylo input stream and waits for clean child exit.
5. The local API and IPC endpoints are removed.

Forced termination at any step is handled by the next startup's recovery and
reconciliation.
