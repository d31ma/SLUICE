# Phase 1 Qualification Matrix

Status: gate open

Evidence date: 2026-08-21

This page is the authoritative roll-up for the feasibility gate in
`docs/PROJECT_PLAN.md`. A passing unit test or cross-build is not promoted to
native qualification. Each row names the evidence that exists and the work
still required before Phase 2 may begin.

| Spike or deliverable | Evidence | Result | Gate still required |
| --- | --- | --- | --- |
| Fyne desktop lifecycle and autostart | Native macOS arm64 Fyne smoke, isolated LaunchAgent lifecycle, and maintained unsigned `Sluice.app`; one stable ten-minute generation, forced Tachyon/agent recovery, authenticated health, and coordinated shutdown | Partial | Native Windows and Linux execution; visual tray and notification observation, real-login launch, and accessibility on every claimed desktop target |
| Yon route to persistent agent | Real `ty -> sluice-yon -> sluice-agent -> fylo` path over a private macOS Unix socket; durable replay survives agent restart and changed payload conflicts; auth, bounded-frame, saturation, and shutdown regressions | Partial | Native Windows and Linux execution of the production local transports and full process chain |
| Fylo lifecycle and ownership | Native macOS arm64 version/capability handshake, effective exclusive-root contention, collection writes, durable idempotency replay, queue lease recovery after owner restart, close/reopen, and packaged supervisor recovery after agent `KILL`; ADR 0008 accepts the focused supervisor | Partial | Native Windows and Linux execution |
| S3 provider contract | AWS SDK v2 adapter, injected conditional-failure tests, and real versioned MinIO and RustFS qualification; Garage rejected as incompatible | Partial | Authorized AWS S3 qualification |
| Atomic installation and logical paths | Native macOS arm64 APFS corpus; handle-relative hostile parent-swap containment; real partial-write ENOSPC cleanup; case and NFC/NFD collision handling; cancellation, permission loss, and pre-rename forced-kill recovery; Windows and Linux compile evidence | Partial | Native Windows and Linux qualification on the named filesystems and containment mechanisms |
| Local API and remote JSON Schemas | Draft 2020-12 schemas, positive and negative fixtures, canonical remote-key and JSON implementation | Pass for contract scope | None for Phase 1; future endpoints must preserve the versioning contract |
| Compiled Go interfaces | `contracts/v1` compiles with conformance tests | Pass | None for Phase 1; implementations remain phase-specific |
| Performance baseline | macOS logical-path, authenticated-read, and 1 MiB APFS install microbenchmarks; maintained native package: 869 ms startup, 600-second idle with 0.855% aggregate process-time CPU and 106,599 KiB mean all-process RSS, bounded recovery, and 59 ms shutdown; Phase 2 100,000-file budget frozen | Pass for Phase 1 reference scope | None for Phase 1; target-specific baselines remain required before support promotion |
| Runtime redistribution | Exact 26.33.02 artifact/checksum matrix, commit-pinned MIT notices, offline negative contracts, native arm64 installed-runtime execution, and an isolated unsigned macOS app bundle running the four-process topology twice | Partial | Native Windows and Linux runtime staging and unsigned package-topology execution |

## Current decision

Phase 1 is not green, and Sluice makes no Preview or Supported target claim.
Production Phase 2 sync-engine work must not begin until every remaining Phase
1 native-environment, external-provider, and UX gate above has
recorded evidence.

The production agent now stores mutation reservations and responses in Fylo.
Completed responses replay across process restarts, while changed payloads
conflict. A claim left pending by an uncertain crash still fails closed. A
production domain mutation route and operator recovery for uncertain pending
claims are deliberately not Phase 1 gates: they belong to the later
sync-engine, operation-ledger, and UI vertical slices.

The frozen 100,000-file scanner budget is executed only after Phase 2 becomes
eligible and the scanner exists; it is not retroactively treated as a Phase 1
implementation gate.

Likewise, final signing, notarization, SBOM/provenance, installation, upgrade,
and rollback qualification remain Phase 8 and Phase 9 release work. Phase 1
proves that the pinned private runtimes can legally and technically be staged
inside an unsigned native package topology; it does not claim a releasable
installer.

## Evidence index

- `desktop/` - Fyne lifecycle and autostart contracts.
- `filesystem/` - logical paths and atomic installation.
- `ipc/` - persistent agent and Yon bridge evidence.
- `runtime/` - installed Tachyon and Fylo evidence.
- `s3/` - provider contract and provider runs.
- `performance/` - reference hardware baselines and frozen budgets.
