# Sluice Project Plan

Status: Proposed implementation baseline

Last updated: 2026-08-21

Initial release scope: Windows, macOS, and Linux desktop
Future scope: Android and iOS

## 1. Product definition

Sluice is an open-source, local-first file synchronization client for
S3-compatible object storage. It gives a user a Dropbox-like desktop
experience while allowing the user or operator to choose the storage provider,
bucket, region, credentials, and cost model.

The desktop application runs continuously, appears in the operating-system
system tray, synchronizes configured local roots, reports progress and
conflicts, and recovers safely after interruption. Placeholder files and
on-demand hydration are platform integrations layered on top of the same sync
engine; they are not separate implementations of synchronization.

### 1.1 Product principles

1. The user controls the storage account and bucket.
2. User files remain ordinary files whenever the platform permits it.
3. Conflicts are visible and recoverable; Sluice never silently discards a
   version.
4. Every acknowledged state transition is recoverable after process or machine
   failure.
5. Per-object latency is addressed with bounded concurrency and backpressure.
6. Regenerable trees such as `.git`, `node_modules`, `dist`, `target`, `build`,
   and `coverage` are ignored by default.
7. The Go sync engine is shared across supported targets.
8. Frontend view and interaction code is shared across targets; unavoidable OS
   integrations live behind explicit platform interfaces.
9. Local state is stored in Fylo, not SQLite.
10. The source, protocols, release process, and governance are open.

### 1.2 Users

- An individual who wants cloud-drive behavior with a personally controlled
  S3-compatible bucket.
- A developer synchronizing large source trees without event storms from
  regenerable directories.
- A small organization self-hosting both storage and the optional Yon control
  plane.
- An operator who needs deterministic diagnostics, repair, export, and
  recovery rather than an opaque sync database.

### 1.3 Desktop v1 outcomes

A desktop v1 user can:

- add and validate an S3-compatible endpoint and bucket;
- select one or more local synchronization roots;
- perform initial upload, download, and bidirectional reconciliation;
- pause and resume globally or by root;
- see queued, active, completed, failed, and conflicted work;
- recover automatically after a crash or forced termination;
- inspect and resolve conflicts without losing either version;
- start Sluice at login and control it from the system tray;
- export a redacted diagnostic bundle; and
- upgrade or roll back without corrupting local state or remote data.

### 1.4 Explicitly deferred from desktop v1

- Android and iOS distribution.
- Multi-user collaboration and shared-folder ACLs.
- A hosted Sluice account service.
- Peer-to-peer synchronization.
- Arbitrary plugin execution inside the sync engine.
- Client-side content encryption using a new Sluice cryptosystem.
- Guaranteed placeholder support on Linux environments without FUSE or an
  equivalent desktop integration.

S3 server-side encryption and provider-managed keys may be selected in v1.
Client-side encryption requires a separate threat model, key-recovery design,
format version, and compatibility ADR.

## 2. Technology decisions

| Concern | Decision | Rationale |
| --- | --- | --- |
| Primary language | Go | Required project direction; strong concurrency and cross-compilation support. |
| Backend framework | Tachyon Yon | Required project direction; versioned REST boundary and supervised executable handlers. |
| Desktop frontend | Fyne | One Go UI codebase, native application packaging, and built-in system-tray support on Windows, macOS, and Linux. |
| Local state | Fylo | Filesystem-first recovery, focused Go supervisor, bounded machine protocol, rebuildable indexes, exclusive-root ownership, and durable queues. |
| Remote storage | S3-compatible API | User-controlled storage and broad provider compatibility. |
| Desktop process model | Fyne shell + Yon server + Go agent + Fylo child | Keeps UI, API supervision, long-running sync, and durable local storage in explicit lifecycle boundaries. |
| Mobile direction | Shared Fyne views plus mobile platform adapters | Preserves shared UI and Go domain logic while accepting mobile storage and background-execution constraints. |
| License | MIT, to be committed in Phase 0 | Matches Tachyon and Fylo and permits broad use and contribution. |

Fyne produces native executables and native tray integration but draws its own
cross-platform widget set. If the project later requires each visible control
to be an OS toolkit control, that is a product-level change. Tachyon Tac may be
reconsidered after it has a qualified system-tray API and supported native
release tier.

## 3. System architecture

```text
                         S3-compatible storage
                         objects / manifests / tombstones
                                  ^
                                  |
                           bounded transfers
                                  |
+------------------+      +-------+--------+      +------------------+
| Fyne desktop app | HTTP | Sluice Yon API | IPC  | sluice-agent     |
| window / tray    +----->+ loopback only  +----->+ Go sync engine   |
| notifications    |      | Go handlers    |      | watchers/workers |
+------------------+      +----------------+      +---------+--------+
                                                            |
                                              one warm focused supervisor
                                                            |
                                                   +--------v--------+
                                                   | Fylo executable |
                                                   | catalog / queue |
                                                   | checkpoints     |
                                                   | conflicts       |
                                                   +-----------------+
```

### 3.1 Process responsibilities

#### `sluice-desktop`

- Owns the Fyne application, main window, system tray, notifications, and
  desktop lifecycle.
- Starts or connects to the local Yon API and `sluice-agent`.
- Contains no S3 synchronization or persistence logic.
- Hides the main window when it is closed and exits only through an explicit
  Quit action or system shutdown.

#### Tachyon/Yon local API

- Binds to a random loopback port only.
- Exposes versioned JSON endpoints beneath `/v1`.
- Runs registered Go executable handlers.
- Validates request shapes and translates domain errors to stable HTTP problem
  responses.
- Bridges low-frequency UI commands to `sluice-agent` over a Unix domain socket
  on macOS/Linux or named pipe on Windows.
- Does not perform transfers, hold watcher state, or own the Fylo root.

Yon is a REST boundary. Its request handlers are not the long-running sync
engine and must not open competing Fylo writers.

#### `sluice-agent`

- Is the only process allowed to mutate Sluice's Fylo root.
- Owns filesystem observation, periodic reconciliation, hashing, operation
  planning, S3 clients, transfer workers, retries, and conflict detection.
- Claims Fylo's exclusive-root lease for its full lifetime.
- Continues synchronizing when the desktop window is hidden.
- Provides a local IPC contract for Yon handlers and the CLI.
- Drains safely on shutdown and resumes incomplete work on restart.

#### `fylo`

- Runs as one warm, version-pinned child of `sluice-agent` through Fylo's Go
  shim after the shim exposes exclusive-root startup, or through a focused
  Sluice supervisor that preserves the same machine protocol until that Fylo
  release exists.
- Negotiates its runtime, protocol, frame limits, and capabilities before the
  agent opens production state.
- Stores Sluice control state and durable work records.
- Never stores S3 credentials or operating-system secret values.

### 3.2 Content and state boundaries

User content has exactly two intended locations: its ordinary local path and
the configured S3-compatible bucket. Fylo does not become a second copy of all
user content.

Fylo stores:

- endpoint and root configuration without secrets;
- the file/object catalog;
- scans and watcher checkpoints;
- planned and completed operations;
- retry and dead-letter state;
- local and remote version observations;
- tombstones and retention deadlines;
- conflicts and resolution decisions; and
- redacted operational events.

Secrets are stored through the native credential facility:

- macOS Keychain;
- Windows Credential Manager; and
- Linux Secret Service, with an explicitly configured environment/file fallback
  for headless systems.

Fylo stores only an opaque secret reference.

### 3.3 Fylo root and ownership

The default Fylo root is beneath the per-user application data directory and
outside every synchronized root:

```text
macOS:   ~/Library/Application Support/Sluice/fylo
Windows: %LOCALAPPDATA%\Sluice\fylo
Linux:   ${XDG_STATE_HOME:-~/.local/state}/sluice/fylo
```

Rules:

- The path is canonicalized before use.
- The agent starts Fylo with `--exclusive-root`.
- Phase 1 must use a version-matched integration that passes
  `--exclusive-root`. ADR 0008 selects the focused Sluice protocol supervisor
  until a matching released Go shim exposes and qualifies the same option; the
  requirement may never be weakened to a best-effort lock.
- A second owner fails closed and presents an actionable diagnostic.
- A network drive, object-backed mount, or independently synchronized
  directory is not a valid Fylo root.
- Fylo and its production integration are pinned to the same qualified release
  identity and shipped together. ADR 0008 currently selects the focused
  supervisor rather than the released shim.
- A capability or protocol mismatch prevents synchronization and leaves user
  files untouched.
- Fylo's optional S3/GCS sync hooks remain disabled for the Sluice state root.
  Sluice owns its remote protocol, and the state root must never recursively
  enter a synchronized root or share the user's content prefix.

### 3.4 Fylo collections

Fylo rejects arrays of objects, so relationships use IDs and related records
use separate collections. Collection names and minimal fields are part of the
Sluice state contract.

| Collection | Purpose | Required indexed fields |
| --- | --- | --- |
| `profiles` | S3 endpoint and behavior profiles | `profileId`, `endpoint`, `region`, `bucket`, `secretRef`, `enabled` |
| `roots` | Configured local sync roots | `rootId`, `profileId`, `canonicalPath`, `mode`, `state`, `scanGeneration` |
| `entries` | One current logical path record | `entryId`, `rootId`, `logicalPath`, `kind`, `size`, `mtime`, `contentHash`, `localState`, `remoteState` |
| `objects` | Observed remote objects and immutable content references | `objectId`, `profileId`, `objectKey`, `etag`, `versionId`, `contentHash`, `size` |
| `operations` | Durable sync state machine | `operationId`, `rootId`, `entryId`, `type`, `state`, `attempt`, `nextAttemptAt`, `idempotencyKey` |
| `checkpoints` | Scan, watcher, and remote-list cursors | `checkpointId`, `rootId`, `kind`, `generation`, `cursor`, `updatedAt` |
| `conflicts` | Both sides of unresolved divergence | `conflictId`, `rootId`, `entryId`, `localRevision`, `remoteRevision`, `state` |
| `tombstones` | Deletion propagation and retention | `tombstoneId`, `rootId`, `logicalPath`, `revision`, `expiresAt` |
| `events` | Redacted operator-visible history | `eventId`, `rootId`, `severity`, `code`, `occurredAt` |

The durable Fylo queue is used for operation claims and retry scheduling. The
`operations` collection remains the queryable ledger and source of user-facing
status; queue messages contain only operation IDs and fencing data.

Large directory traversals use paged Fylo queries. Sluice never assumes an
unbounded response fits inside the machine protocol frame.

### 3.5 Go module boundaries

```text
cmd/
  sluice/             CLI
  sluice-agent/       long-running sync agent
  sluice-desktop/     Fyne desktop application
  sluice-yon/         registered Yon executable adapter
internal/
  domain/             entries, revisions, operations, conflicts, errors
  syncengine/         reconciliation and state machines
  state/fylo/         Fylo repositories and migrations
  objectstore/s3/     S3-compatible implementation
  filesystem/         scanner, watcher, ignore matcher, hashing
  platform/           interfaces and target adapters
  transport/ipc/      local agent IPC
  transport/yon/      request/response mapping
  secrets/            credential-store interface and adapters
  observability/      redacted logs, metrics, diagnostic bundle
ui/
  app/                shared Fyne views, state, and navigation
  tray/               shared tray model and actions
server/
  routes/v1/          Tachyon/Yon route sources
api/
  local/v1/           JSON schemas and examples
  remote/v1/          S3 object-layout and manifest contracts
```

Business logic must not import Fyne, Tachyon, Fylo client types, AWS SDK types,
or OS-specific packages.

### 3.6 Core Go interfaces

The following interfaces define the initial architecture. Concrete signatures
may be extended additively, but replacements require an ADR and contract-test
updates.

```go
type StateStore interface {
    Open(ctx context.Context) error
    Close(ctx context.Context) error
    LoadRoot(ctx context.Context, rootID string) (Root, error)
    UpsertEntries(ctx context.Context, entries []Entry) error
    PageEntries(ctx context.Context, query EntryQuery, page Page) (EntryPage, error)
    Enqueue(ctx context.Context, operation Operation) error
    Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]Operation, error)
    Complete(ctx context.Context, operationID, receipt string, result OperationResult) error
    Retry(ctx context.Context, operationID, receipt string, failure Failure, at time.Time) error
    RecordConflict(ctx context.Context, conflict Conflict) error
}

type ObjectStore interface {
    Probe(ctx context.Context) (Capabilities, error)
    List(ctx context.Context, prefix, cursor string, limit int) (ObjectPage, error)
    Head(ctx context.Context, key string) (RemoteObject, error)
    Get(ctx context.Context, request DownloadRequest) (DownloadResult, error)
    Put(ctx context.Context, request UploadRequest) (UploadResult, error)
    Delete(ctx context.Context, request DeleteRequest) (DeleteResult, error)
    CompareAndSwap(ctx context.Context, request CASRequest) (CASResult, error)
}

type PlatformAdapter interface {
    CanonicalizeRoot(ctx context.Context, path string) (CanonicalRoot, error)
    Scan(ctx context.Context, root CanonicalRoot, rules IgnoreRules) (<-chan ScanEvent, <-chan error)
    Watch(ctx context.Context, root CanonicalRoot) (Watcher, error)
    Materialize(ctx context.Context, request MaterializeRequest) error
    Evict(ctx context.Context, request EvictRequest) error
    SetStartAtLogin(ctx context.Context, enabled bool) error
}

type SyncEngine interface {
    Reconcile(ctx context.Context, rootID string, reason ReconcileReason) (RunID, error)
    Pause(ctx context.Context, rootID string) error
    Resume(ctx context.Context, rootID string) error
    ResolveConflict(ctx context.Context, conflictID string, resolution Resolution) error
    Status(ctx context.Context, rootID string) (RootStatus, error)
}
```

Domain errors expose a stable code and retry classification. UI, Yon, CLI, and
logs branch on codes, never message text.

### 3.7 Local Yon API v1

The desktop API is loopback-only and authenticated with a random per-install
token stored in the native credential store. The token is never accepted in a
URL, written to logs, or included in diagnostic bundles.

Initial endpoints:

```text
GET    /v1/health
GET    /v1/version
GET    /v1/status
GET    /v1/profiles
POST   /v1/profiles
POST   /v1/profiles/{profileId}/probe
GET    /v1/roots
POST   /v1/roots
POST   /v1/roots/{rootId}/pause
POST   /v1/roots/{rootId}/resume
POST   /v1/roots/{rootId}/reconcile
GET    /v1/operations
GET    /v1/conflicts
POST   /v1/conflicts/{conflictId}/resolve
POST   /v1/diagnostics/export
```

Mutation endpoints require an idempotency key. Errors use
`application/problem+json` with `code`, `title`, `status`, `retryable`, and a
redacted `detail`.

### 3.8 Remote S3 protocol

Sluice must not treat arbitrary object keys and timestamps as a safe
bidirectional protocol. Phase 1 defines and versions a dedicated namespace:

```text
<prefix>/.sluice/v1/
  roots/<root-id>/descriptor.json
  manifests/<root-id>/<logical-path-hash>.json
  objects/sha256/<first-two>/<content-hash>
  tombstones/<root-id>/<logical-path-hash>.json
  leases/                  optional provider-qualified coordination only
```

Required properties:

- Content objects are immutable and addressed by SHA-256.
- Path manifests are mutable only through conditional writes using the last
  observed ETag or version ID.
- Tombstones are versioned and retained long enough for offline devices.
- A manifest references content by hash and contains the logical path, size,
  media type, executable bit where portable, revision, and device identity.
- Remote format versions are independently versioned from the Sluice release.
- Unknown format versions fail closed.
- Provider-specific checksum, multipart, and conditional-write behavior is
  discovered by a qualification probe and recorded in the profile.
- Listing is never used as the sole conflict clock.
- Sluice never deletes an immutable content object until a verified garbage
  collection proves no retained manifest or tombstone references it.

The exact JSON schemas and canonicalization rules are Phase 1 deliverables and
must exist before bidirectional implementation begins.

## 4. Sync behavior

### 4.1 Entry state machine

```text
unknown -> observed -> indexed -> compared
                               |-> upload_queued -> uploading -> synced
                               |-> download_queued -> downloading -> synced
                               |-> delete_queued -> deleting -> tombstoned
                               |-> conflict -> resolved -> compared
                               `-> failed_retryable -> queued
                               `-> failed_terminal
```

Every transition is written to Fylo before the corresponding external side
effect is considered scheduled. Completion is written only after local and
remote postconditions are verified.

### 4.2 Reconciliation

1. Load the root and last stable checkpoints.
2. Enumerate local changes using watcher hints plus a periodic authoritative
   scan.
3. Page remote manifests and tombstones from the last safe cursor.
4. Normalize logical paths and reject paths unsafe for the target platform.
5. Compare local, last-synced, and remote revisions.
6. Write a deterministic operation plan and idempotency keys to Fylo.
7. Claim work through the durable queue.
8. Execute transfers through a bounded worker pool.
9. Verify results, commit state, and advance checkpoints.
10. Surface divergence as a conflict with both versions preserved.

Watcher events are hints, not truth. Overflow, dropped events, sleep/wake, and
network recovery schedule a full or bounded reconciliation.

### 4.3 Conflict policy

A conflict exists when both local and remote revisions changed since the last
common revision and neither side is a content-identical change.

Default handling:

- preserve the local path;
- download the remote version to a deterministic conflict filename;
- create a Fylo conflict record referencing both revisions;
- notify the user without blocking unrelated files; and
- require an explicit keep-local, keep-remote, keep-both, or manual resolution.

Sluice never resolves by latest timestamp alone.

### 4.4 Ignore behavior

Default rules are versioned, visible, and overridable. User rules follow a
documented gitignore-compatible subset. Sluice explains which rule excluded a
path. Symlinks, junctions, reparse points, sockets, devices, and filesystem
cycles are handled by an explicit platform policy and never followed by
accident.

## 5. Quality, security, and operational requirements

### 5.1 Performance budgets

Initial budgets, measured on named reference hardware and revisable only by an
ADR before beta:

- Tray becomes interactive within 3 seconds of launch when no recovery is
  required.
- Idle agent CPU remains below 1% averaged over 10 minutes after a stable scan.
- Idle desktop plus agent memory remains below 250 MiB.
- A metadata-only scan of 100,000 files completes within 10 minutes on the
  reference SSD, excluding content hashing and network transfer.
- Transfer concurrency is configurable from 1 to 64 with a default of 8.
- Queue depth does not create unbounded goroutines or memory growth.
- UI list endpoints are paginated and return the first page within 500 ms for a
  100,000-entry root under the reference profile.

### 5.2 Durability gates

- Forced termination at every operation transition recovers to one documented
  valid state.
- Repeated recovery is idempotent.
- An acknowledged upload has a verified remote object and manifest.
- An acknowledged download has durable local bytes and metadata.
- Ambiguous S3 failures are reconciled before retry rather than blindly
  duplicated.
- Fylo corruption or capability failure pauses synchronization and preserves
  evidence for repair.

### 5.3 Security baseline

- TLS verification is enabled by default; insecure endpoints require explicit
  per-profile development configuration and a persistent warning.
- S3 credentials use least privilege and native secret storage.
- Local API authentication is mandatory even on loopback.
- File paths, object keys, endpoint URLs, manifest JSON, Fylo responses, and
  watcher events are untrusted inputs.
- No shell is used to invoke `ty`, Yon handlers, `sluice-agent`, or `fylo`.
- Downloads are written to a same-filesystem temporary path, verified, and
  atomically installed without following links.
- Logs redact credentials, authorization headers, signed URLs, document
  contents, and sensitive absolute paths.
- Update artifacts are signed and verified before installation.
- Release output includes checksums, SBOMs, provenance, and dependency-license
  reports.

### 5.4 Observability

Structured events include stable code, severity, component, root ID, operation
ID, attempt, duration, and redacted cause. Required metrics include queue depth,
active transfers, throughput, retry counts, conflict counts, reconciliation
duration, watcher overflow, Fylo recovery, and S3 request classes.

The diagnostic export is locally generated, previewable, redacted, and never
uploaded without a separate explicit user action.

## 6. Delivery phases

Each phase ends in an executable vertical slice. A phase is complete only when
its acceptance gate passes on every target claimed by that phase.

### Phase 0 - Open-source foundation and decision record

Goal: establish a trustworthy repository and freeze the initial contracts.

Deliverables:

- Go workspace, pinned toolchain, formatting, linting, unit-test, race-test,
  vulnerability, and license gates.
- MIT `LICENSE`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `SECURITY.md`,
  `SUPPORT.md`, governance, changelog, and maintainer ownership.
- ADRs for Fyne, Yon process topology, Fylo ownership, S3 namespace, secret
  storage, conflict policy, and support vocabulary.
- CI on Windows, macOS, and Linux.
- Versioning policy for application, local API, Fylo schema, and remote format.
- Threat model and data-flow diagram.

Acceptance gate:

- A clean checkout runs every documented gate.
- License and dependency policy are machine checked.
- No architectural ambiguity remains that would change process topology,
  persistent data shape, or remote object layout.

Out of scope: synchronization code.

### Phase 1 - Feasibility spikes and executable contracts

Goal: retire the highest-risk framework and integration assumptions before
building the engine.

Vertical spikes:

1. Fyne application with shared window, tray menu, hide/show, notification,
   start-at-login adapter, and clean shutdown on all three desktop OSs.
2. Tachyon/Yon route invoking a registered Go executable and bridging to a
   long-running local Go agent.
3. Go agent launching a pinned Fylo binary, completing the handshake, claiming
   exclusive ownership, writing collections, claiming a queue item, surviving
   forced termination, and reopening successfully.
4. S3 qualification against AWS S3, MinIO, and one additional provider using
   list, conditional put, multipart upload, range download, delete, and error
   injection.
5. Platform-safe atomic file installation and path normalization corpus.

Deliverables:

- JSON Schemas for local API v1 and remote protocol v1.
- A version-matched Fylo integration with an explicit exclusive-root option and
  a black-box contract proving the flag is effective. ADR 0008 qualifies the
  focused Sluice protocol supervisor for Fylo 26.33.02.
- Black-box contract tests for process restart, API authentication, Fylo
  mismatch, duplicate requests, and S3 conditional failures.
- Recorded reference-hardware performance baselines.
- Go interfaces from Section 3 compiled in a contract-only module.

Acceptance gate:

- All five spikes pass on their named native environments.
- The unsigned target package topologies can legally and technically bundle
  the pinned `ty` and `fylo` runtimes and required notices.
- No production phase depends on an unimplemented Tachyon or Fylo behavior.

### Phase 2 - Local catalog and deterministic scanner

Goal: turn a local directory into durable, queryable Sluice state without
network access.

Deliverables:

- Fylo migrations and repositories for profiles, roots, entries, operations,
  checkpoints, conflicts, tombstones, and events.
- Canonical path and logical-path rules for Windows, macOS, and Linux.
- Versioned default ignores and user ignore parser.
- Deterministic directory scanner with bounded memory and cancellation.
- Hash service with content-change detection and configurable hashing policy.
- Scan checkpointing, pagination, progress reporting, and restart recovery.
- CLI commands: `root add`, `root scan`, `status`, `doctor`, and `state export`.

Acceptance gate:

- Repeated unchanged scans produce no operations.
- Renames, case-only changes, permission failures, symlinks, cycles, and watcher
  overflow fixtures have defined results.
- A forced termination during every scan stage resumes without duplicated or
  lost catalog records.
- A 100,000-file metadata scan meets the Phase 1 budget.

### Phase 3 - One-way upload alpha

Goal: deliver the first useful end-to-end path: local files to S3.

Deliverables:

- Provider profile creation, secret references, and capability probe.
- Versioned immutable content-object upload.
- Path-manifest conditional writes.
- Fylo-backed operation ledger and durable queue.
- Bounded upload pool with backpressure, multipart resume, retry budgets,
  cancellation, and bandwidth limits.
- Idempotent CLI command `sync push` and progress inspection.
- Remote verification and redacted diagnostics.

Acceptance gate:

- Uploading the same unchanged tree twice transfers no content bytes the second
  time.
- Kill/restart tests during hashing, multipart upload, manifest publication,
  and completion recover correctly.
- Conditional-write conflicts never overwrite an unknown remote revision.
- AWS S3 and MinIO pass the full provider contract.

Release label: developer alpha; CLI only; no bidirectional safety claim.

### Phase 4 - Download and bidirectional reconciliation

Goal: complete a safe three-way synchronization engine.

Deliverables:

- Paged remote discovery and stable checkpoints.
- Verified download to temporary file and atomic installation.
- Tombstone creation, propagation, retention, and restore window.
- Three-way comparison using local, last-synced, and remote revisions.
- Rename and move representation.
- Conflict creation and keep-local, keep-remote, and keep-both resolutions.
- Remote garbage-collection dry run; destructive collection remains disabled
  until beta evidence exists.

Acceptance gate:

- A two-device deterministic simulation converges for non-conflicting changes.
- Concurrent modification, delete-vs-edit, rename-vs-edit, offline deletion,
  case collision, and provider clock-skew scenarios preserve all user content.
- Property tests prove reconciliation is idempotent and reaches a fixed point.
- Network interruption and ambiguous responses do not create silent loss.

Release label: headless alpha.

### Phase 5 - Desktop application and system tray

Goal: make the headless engine usable as a daily desktop application.

Deliverables:

- Shared Fyne onboarding, profile, root, activity, conflict, settings, and
  diagnostics views.
- Tray states for idle, syncing, paused, offline, conflict, and error.
- Tray actions: open, pause/resume, sync now, open root, view conflicts, quit.
- Native notifications with rate limiting and recovery actions.
- Start-at-login adapters and safe upgrade restart.
- Accessible keyboard navigation, focus, labels, contrast, scaling, and reduced
  motion.
- Loading, empty, offline, error, and recovery states.

Acceptance gate:

- UI acceptance tests exercise every state through the Yon API, not in-process
  mocks.
- Tray lifecycle and autostart pass on native Windows, macOS, and Linux hosts.
- Closing the window never terminates active synchronization.
- Screen-reader and keyboard audits pass the documented accessibility target.

Release label: desktop preview.

### Phase 6 - Continuous observation and desktop lifecycle

Goal: synchronize continuously without relying on perfect filesystem events.

Deliverables:

- Native watcher adapters with overflow detection.
- Debounce/coalescing that preserves causality and bounded memory.
- Periodic authoritative reconciliation.
- Sleep, wake, network-change, removable-volume, and shutdown handling.
- Battery, metered-network, schedule, and bandwidth policies.
- Per-root pause and global pause with durable state.

Acceptance gate:

- Event storms from ignored trees do not saturate the agent or Fylo.
- Dropped watcher events are repaired by reconciliation.
- Sleep/wake and offline/online transitions converge without manual restart.
- A 72-hour native soak has bounded memory, goroutine count, queue depth, and
  file handles.

### Phase 7 - Placeholder and on-demand hydration adapters

Goal: add cloud-drive semantics without changing the sync engine.

Target slices:

- Windows Cloud Files API placeholder, hydration, dehydration, and pin state.
- macOS File Provider domain, enumerator, item lifecycle, and materialization.
- Linux optional FUSE filesystem with explicit fallback to ordinary sync.

Deliverables:

- Platform capability negotiation and per-root mode.
- Shared materialization contracts and platform-specific adapters.
- Offline-open, cancellation, disk-pressure, and eviction behavior.
- Accurate physical-versus-logical disk usage reporting.
- Native install/uninstall cleanup and rollback runbooks.

Acceptance gate:

- Placeholder operations never report success before verified local bytes are
  available.
- Eviction never removes the only recoverable content revision.
- Each platform slice passes crash, reboot, upgrade, uninstall, and low-disk
  tests on native hosts.
- Ordinary-sync mode remains fully supported when placeholder capability is
  unavailable.

Release label: platform previews promoted independently.

### Phase 8 - Security, recovery, and operations hardening

Goal: qualify the system for beta exposure.

Deliverables:

- Completed threat model and independent security review.
- Fuzzing for manifests, paths, Yon requests, Fylo responses, and S3 error
  shapes.
- Fault injection for disk full, permission loss, corrupted Fylo state,
  missing secrets, partial local writes, and provider outages.
- State backup, repair, rebuild, migration, rollback, and reset runbooks.
- Signed update channel, SBOM, provenance, and reproducible-build evidence.
- Provider support matrix and qualification tool.
- Diagnostic bundle redaction tests.

Acceptance gate:

- No open critical or high security findings.
- Recovery exercises restore service without modifying user files unexpectedly.
- Upgrade from every preview schema and rollback across the documented boundary
  pass.
- Seven-day mixed-workload soak passes on all supported desktop targets.

Release label: public beta.

### Phase 9 - Desktop 1.0 qualification

Goal: publish a supportable open-source desktop release.

Deliverables:

- Signed Windows installer, notarized macOS package, and documented Linux
  packages.
- Automated update and rollback behavior.
- User guide, provider setup, troubleshooting, conflict recovery, privacy,
  security, operations, and contribution documentation.
- Compatibility and support matrices with named OS and provider versions.
- Release evidence bundle and incident-response ownership.

Acceptance gate:

- Clean-install, upgrade, rollback, uninstall, and state-preservation tests pass
  on every supported OS.
- Acceptance, integration, race, fuzz, crash, recovery, security, performance,
  accessibility, and soak gates are green.
- No known path can silently overwrite two divergent user versions.
- Support and security-response processes are staffed and documented.

Release label: desktop 1.0.

### Phase 10 - Android and iOS feasibility and delivery

Goal: extend Sluice without pretending desktop lifecycle assumptions transfer
to mobile.

Android slice:

- Shared Fyne views and Go domain logic.
- Storage Access Framework/DocumentsProvider adapter.
- Foreground-service notification for user-visible long work.
- WorkManager scheduling with battery and metered-network constraints.
- Fylo's supported on-device client rather than spawning the desktop binary.

iOS slice:

- Shared Fyne views and Go domain logic where toolchain support is qualified.
- File Provider extension and background-task integration.
- Keychain credentials and OS-managed transfer constraints.
- Fylo's supported on-device client rather than spawning a process.

Acceptance gate:

- A Phase 10 ADR proves the supported Fyne, Go, Fylo, and OS-extension topology
  on physical devices before production implementation.
- Mobile UI source remains shared; platform storage and lifecycle adapters are
  explicit and minimal.
- Neither platform claims continuous background behavior the OS cannot grant.

Mobile targets receive independent preview and supported promotions; they do
not delay desktop 1.0.

## 7. Test strategy

| Layer | Required evidence |
| --- | --- |
| Unit | Domain state machines, path rules, ignore rules, retry classification, manifest canonicalization. |
| Property/model | Reconciliation convergence, idempotency, operation fencing, tombstone retention, conflict preservation. |
| Contract | Fylo handshake/client, Yon API schemas, agent IPC, S3 provider profile, remote manifest format. |
| Integration | Real `ty`, Go Yon handler, `sluice-agent`, `fylo`, filesystem, and S3-compatible service. |
| Native | Tray, autostart, watcher, secret store, installer, filesystem semantics, placeholder API. |
| Fault injection | Kill points, disk full, corrupt frames, permission loss, timeouts, truncated downloads, ambiguous S3 responses. |
| Security | Path traversal, symlink/reparse attacks, SSRF, credential leakage, local API abuse, update tampering. |
| Performance | Large trees, small-file latency, multipart files, bounded concurrency, idle resource use. |
| Soak | Multi-day continuous changes, network transitions, sleep/wake, retries, and upgrade. |

Tests observe public CLI, HTTP, IPC, filesystem, and S3 behavior. In-process
mocks are limited to true external boundaries and never replace the final
real-process contract suite.

## 8. Release and support vocabulary

| Tier | Meaning |
| --- | --- |
| Buildable | An artifact compiles for the target; it has not executed there. |
| Native-tested | The artifact passes the named suite on the target OS/device. |
| Preview | Distributed for evaluation with documented limitations and recovery path. |
| Supported | Install, upgrade, rollback, security, crash, accessibility, performance, and soak gates pass. |

Cross-compilation is never presented as native support. Windows, macOS, Linux,
Android, iOS, and individual placeholder adapters are promoted independently.

## 9. Primary risks and mitigations

| Risk | Impact | Mitigation and decision point |
| --- | --- | --- |
| Yon executable lifecycle conflicts with a long-running sync engine | Lost state or repeated process startup | Keep the agent persistent and make Yon a thin IPC bridge; prove in Phase 1. |
| Fylo protocol/client mismatch | State unavailable or unsafe writes | Pin binary and production integration, require handshake/capabilities, fail closed. |
| Multiple writers open one Fylo root | Catalog corruption or inconsistent work claims | One agent, exclusive-root lease, native single-instance tests. |
| Fylo Go shim cannot currently request exclusive-root mode | Single-owner guarantee could be accidentally omitted | ADR 0008 selects the qualified focused protocol supervisor until a matching released shim passes the same suite. |
| Fyne does not meet desired native appearance/accessibility | Poor UX or product rejection | Native Phase 1 prototype and accessibility gate before engine investment. |
| S3-compatible providers differ materially | Data loss or stuck synchronization | Provider qualification suite and explicit supported profiles. |
| Remote manifest protocol is underspecified | Silent overwrite or non-convergence | Freeze schemas, conditional-write rules, and model tests in Phase 1. |
| Platform file APIs dominate schedule | Desktop release delay | Ship ordinary sync first; promote placeholder adapters independently. |
| Mobile cannot reuse desktop process topology | Rewrite pressure | Share views/domain only; decide mobile topology on physical-device evidence. |
| Fylo metadata grows with very large trees | Slow UI or reconciliation | Batch writes, paged queries, retention, compaction/rebuild monitoring, large-tree benchmarks. |
| Content-addressed objects accumulate | Storage cost | Verified, dry-run-first garbage collection after retention and reference proof. |
| Credentials leak through logs or diagnostics | Account compromise | Native secret storage, redaction-by-construction, security tests. |

## 10. Definition of done for every phase

A phase is done only when:

- acceptance criteria pass on the claimed native environments;
- new behavior has observable black-box coverage;
- errors are stable, actionable, and documented;
- migrations, recovery, and rollback are tested;
- secrets and untrusted inputs have been reviewed;
- performance and resource impact are measured;
- user, operator, and contributor documentation is current;
- no dead code, temporary artifacts, debug output, or unowned dependency ships;
- Graphify context is refreshed; and
- the release/support label accurately reflects the evidence.

## 11. Immediate next actions

1. Accept this plan as the working product and architecture baseline.
2. Initialize the Git repository and complete Phase 0 governance.
3. Record the seven Phase 0 ADRs.
4. Build the five Phase 1 feasibility spikes before implementing the sync
   engine.
5. Freeze local API v1, Fylo schema v1, and remote protocol v1 from the spike
   evidence.
6. Begin Phase 2 only after the Phase 1 gate is green.

## 12. Dependency references

- Tachyon/Yon: <https://tachyon.del.ma>
- Fylo: <https://fylo.del.ma>
- Fyne: <https://fyne.io>
- S3 API: provider-specific, qualified through the Sluice provider contract
