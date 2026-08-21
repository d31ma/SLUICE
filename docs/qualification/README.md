# Phase 1 Qualification Matrix

Status: gate open

Evidence date: 2026-08-21

This page is the authoritative roll-up for the feasibility gate in
`docs/PROJECT_PLAN.md`. A passing unit test or cross-build is not promoted to
native qualification. Each row names the evidence that exists and the work
still required before Phase 2 may begin.

| Spike or deliverable | Evidence | Result | Gate still required |
| --- | --- | --- | --- |
| Fyne desktop lifecycle and autostart | Native macOS arm64 smoke and LaunchAgent lifecycle; Windows desktop PE cross-build; Linux autostart cross-build | Partial | Native Windows and Linux execution; visual tray and notification observation, real-login launch, accessibility, and coordinated agent shutdown on every claimed desktop target |
| Yon route to persistent agent | Real `ty -> sluice-yon -> sluice-agent -> fylo` path on macOS arm64; durable replay survives agent restart and changed payload conflicts; auth, bounded-frame, saturation, and shutdown regressions | Partial | A production domain mutation route and native Windows and Linux execution |
| Fylo lifecycle and ownership | Native macOS arm64 version/capability handshake, effective exclusive-root contention, collection writes, durable idempotency replay, queue lease recovery after owner restart, close, and reopen | Partial | Operation-ledger recovery for uncertain pending claims; native Windows and Linux execution |
| S3 provider contract | AWS SDK v2 adapter, injected conditional-failure tests, and real versioned MinIO and RustFS qualification; Garage rejected as incompatible | Partial | Authorized AWS S3 qualification |
| Atomic installation and logical paths | Corpus; Darwin handle-relative hostile parent-swap containment, permission-loss, kernel write-failure, cancellation, forced-kill orphan recovery; Windows and Linux compile evidence | Partial | Real APFS ENOSPC, native arm64 and rename-boundary termination runs, normalization/case collision policy, and native Windows and Linux qualification |
| Local API and remote JSON Schemas | Draft 2020-12 schemas, positive and negative fixtures, canonical remote-key and JSON implementation | Pass for contract scope | Keep schemas backward compatible while the missing runtime mutation path is implemented |
| Compiled Go interfaces | `contracts/v1` compiles with conformance tests | Pass | Implementations remain phase-specific |
| Performance baseline | macOS arm64 path, authenticated read, and 1 MiB install results; Phase 2 100,000-file budget frozen | Pass for baseline scope | Run the frozen scanner workload when Phase 2 becomes eligible |
| Runtime redistribution | Exact 26.33.02 artifact/checksum matrix, commit-pinned MIT notices, offline negative contract tests, and isolated macOS arm64 staging/execution | Partial | Native Windows/Linux staging, final installer/signing/notarization, upgrade/rollback, SBOM/provenance, and dependency-notice validation |

## Current decision

Phase 1 is not green, and Sluice makes no Preview or Supported target claim.
Production Phase 2 sync-engine work must not begin until every named native
environment and external-provider gate above has recorded evidence.

The production agent now stores mutation reservations and responses in Fylo.
Completed responses replay across process restarts, while changed payloads
conflict. A claim left pending by an uncertain crash still fails closed; Phase
2 must connect that state to the operation ledger so an operator can inspect
and recover it without repeating a possible side effect.

## Evidence index

- `desktop/` - Fyne lifecycle and autostart contracts.
- `filesystem/` - logical paths and atomic installation.
- `ipc/` - persistent agent and Yon bridge evidence.
- `runtime/` - installed Tachyon and Fylo evidence.
- `s3/` - provider contract and provider runs.
- `performance/` - reference hardware baselines and frozen budgets.
