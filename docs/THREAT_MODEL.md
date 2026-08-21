# Sluice Threat Model

Status: Phase 0 baseline

Last reviewed: 2026-08-21

## 1. Scope

This model covers the desktop process topology, local filesystem and Fylo
state, loopback Yon API, local IPC, S3-compatible provider boundary, native
secret storage, update artifacts, diagnostics, and placeholder adapters.

Mobile-specific extensions require a Phase 10 revision based on physical-
device topology.

## 2. Protected assets

- User file contents and filenames.
- S3 access keys, session tokens, signed URLs, and provider account metadata.
- Local API token and IPC authority.
- Fylo catalog, operation intent, conflicts, checkpoints, and tombstones.
- Remote manifests, tombstones, and immutable content references.
- Update signing keys and release provenance.
- User trust in conflict preservation, deletion, and support claims.

## 3. Trust boundaries

```text
User
  |
  v
Fyne desktop UI
  | Authorization token over loopback HTTP
  v
Tachyon/Yon handler process
  | bounded local IPC
  v
sluice-agent ------------------> Native secret facility
  |             |
  |             +-------------> S3-compatible endpoint
  v
Fylo machine protocol
  |
  v
Local Fylo root

sluice-agent <-----------------> synchronized filesystem root
```

Every arrow crossing a process, filesystem, network, or credential facility is
a validation boundary. Loopback and same-user processes are not implicitly
trusted.

## 4. Threat actors

- Malicious or compromised local unprivileged process.
- Malicious file or directory name inside a synchronized root.
- Compromised or misconfigured S3-compatible endpoint.
- Network attacker when TLS verification is weakened.
- Malicious remote manifest or object uploaded outside Sluice.
- Compromised dependency, installer, update channel, or build environment.
- Accidental operator action, stale device, or faulty filesystem/provider.

## 5. Threats and required controls

### 5.1 Local API command forgery

Threat: another local process sends pause, delete, profile, conflict, or export
commands.

Controls:

- random per-install token in native secret storage;
- loopback-only random port;
- authorization header, constant-time comparison, and uniform denial;
- request size, method, content type, and schema bounds;
- mutation idempotency keys and audit events;
- no secrets in URLs, process arguments, logs, or diagnostics.

### 5.2 IPC impersonation and stale process control

Threat: a stale or malicious process impersonates the agent or replays a
command.

Controls:

- user-private socket/pipe namespace and permissions;
- handshake with protocol version, agent instance ID, and challenge;
- request IDs, bounded frames, deadlines, and cancellation;
- agent generation fencing and exclusive Fylo root ownership;
- fail closed on identity or version mismatch.

### 5.3 Path traversal and link attacks

Threat: remote or local input escapes a root, overwrites a sensitive path, or
changes meaning between validation and use.

Controls:

- strict logical-path canonicalization independent of host paths;
- reject absolute paths, control characters, empty segments, `.` and `..`;
- platform collision checks for case, reserved names, separators, and Unicode;
- directory-handle-relative operations where available;
- do not follow symlinks, junctions, or reparse points by default;
- same-filesystem temporary download, verification, and atomic installation;
- destination non-replacement or compare-and-swap rules.

### 5.4 Remote overwrite and rollback

Threat: concurrent devices or stale clients overwrite a newer manifest or
resurrect deleted content.

Controls:

- immutable content objects;
- conditionally written manifests using ETag or version ID;
- retained versioned tombstones;
- three-way reconciliation and explicit conflicts;
- unknown protocol versions fail closed;
- listing is not the sole conflict clock;
- garbage collection proves lack of retained references.

### 5.5 Credential disclosure

Threat: credentials leak through state, exports, logs, crash reports, commands,
or environment inheritance.

Controls:

- native secret stores and opaque Fylo references;
- allowlisted child environment;
- no shell invocation;
- structured redaction types for headers, URLs, paths, and errors;
- diagnostic preview and redaction tests;
- least-privilege provider policies and short-lived credentials when possible.

### 5.6 Fylo corruption or split ownership

Threat: competing writers or corruption produces incorrect operation intent.

Controls:

- one warm child with `--exclusive-root`;
- root outside synchronized trees on a qualified local filesystem;
- version-matched binary and shim;
- handshake and capability checks;
- stable error-code handling, paging, and bounded frames;
- pause on ambiguous native I/O or recovery failure;
- preserve evidence and provide repair/rebuild runbooks.

### 5.7 Resource exhaustion

Threat: large trees, event storms, responses, queues, or objects exhaust
memory, file descriptors, disk, goroutines, or network capacity.

Controls:

- default ignore rules before watcher enqueue;
- bounded scans, pages, queues, worker pools, buffers, and retries;
- backpressure and per-root/global pause;
- multipart and range transfer limits;
- disk-space checks and retention policies;
- rate-limited notifications and logs;
- soak tests and resource metrics.

### 5.8 Supply-chain compromise

Threat: a compromised dependency or artifact executes with file and credential
authority.

Controls:

- pinned Go toolchain and dependency graph;
- locked Tachyon and Fylo artifacts with checksums;
- vulnerability, license, and dependency review;
- signed artifacts, SBOM, provenance, and reproducible-build evidence;
- verified updates before execution;
- no mutable production download URLs without integrity metadata.

### 5.9 Misleading support claim

Threat: users entrust data to an integration that only cross-compiled.

Controls:

- evidence-based support tiers;
- named native test environment and artifact digest;
- release-specific crash, recovery, accessibility, performance, and soak
  evidence;
- independent target promotion.

## 6. Security-sensitive invariants

- Authentication fails closed.
- Credentials and content never enter logs.
- Conflict resolution never silently discards a version.
- Download installation never follows an untrusted link.
- Remote deletion never precedes a durable tombstone and retention policy.
- Unknown versions and capabilities stop synchronization before side effects.
- Ambiguous I/O is reconciled before retry.

## 7. Residual risks

- Fyne and OS-specific accessibility behavior requires native qualification.
- S3-compatible providers may diverge despite sharing an API shape.
- Same-user malware may access files the user can access; process separation is
  not an OS sandbox.
- Endpoint administrators can delete or alter bucket data outside Sluice.
- Client-side content encryption is deferred and provider-side encryption does
  not hide content from the provider account.
- Mobile background execution cannot be guaranteed beyond OS policy.

## 8. Verification plan

- Unit and property tests for paths, manifests, reconciliation, and redaction.
- Real-process local API and IPC authentication tests.
- Native secret-store, filesystem, tray, and placeholder tests.
- Provider contract tests with malicious and ambiguous responses.
- Fuzzing of local API, IPC, Fylo, manifest, and path inputs.
- Fault injection for kill, disk full, permission loss, truncation, and
  corruption.
- Dependency, SBOM, provenance, and artifact verification in release gates.
