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
| Yon route to persistent agent | Real Tachyon 26.33.02 route and authenticated loopback IPC on macOS arm64; restart, auth, bounded-frame, saturation, and shutdown regressions | Partial | Complete real `ty -> sluice-yon -> sluice-agent -> fylo` mutation path with durable Fylo idempotency replay; native Windows and Linux execution |
| Fylo lifecycle and ownership | Native macOS arm64 version/capability handshake, effective exclusive-root contention, forced child termination, close, and reopen | Partial | Collection writes, durable queue claim/recovery, and full agent-path restart evidence; native Windows and Linux execution |
| S3 provider contract | AWS SDK v2 adapter, injected conditional-failure tests, and real versioned MinIO qualification | Partial | Authorized AWS S3 and one independent third-provider qualification |
| Atomic installation and logical paths | Corpus and native macOS APFS tests; Windows and Linux compile evidence | Partial | Native Windows and Linux tests plus disk-full, forced-kill, permission-loss, and hostile parent-component swap qualification on every claimed filesystem |
| Local API and remote JSON Schemas | Draft 2020-12 schemas, positive and negative fixtures, canonical remote-key and JSON implementation | Pass for contract scope | Keep schemas backward compatible while the missing runtime mutation path is implemented |
| Compiled Go interfaces | `contracts/v1` compiles with conformance tests | Pass | Implementations remain phase-specific |
| Performance baseline | macOS arm64 path, authenticated read, and 1 MiB install results; Phase 2 100,000-file budget frozen | Pass for baseline scope | Run the frozen scanner workload when Phase 2 becomes eligible |
| Runtime redistribution | License policy is machine checked and upstream projects are MIT-compatible | Partial | Record exact bundled artifacts, notices, checksums, packaging, and installation tests for each target |

## Current decision

Phase 1 is not green, and Sluice makes no Preview or Supported target claim.
Production Phase 2 sync-engine work must not begin until every named native
environment and external-provider gate above has recorded evidence.

The fail-closed agent behavior is intentional: mutation requests return a
stable state-unavailable response until a Fylo-backed idempotency store can
persist and replay the response across process restarts. This prevents the
prototype from presenting memory-only deduplication as durable coordination.

## Evidence index

- `desktop/` - Fyne lifecycle and autostart contracts.
- `filesystem/` - logical paths and atomic installation.
- `ipc/` - persistent agent and Yon bridge evidence.
- `runtime/` - installed Tachyon and Fylo evidence.
- `s3/` - provider contract and provider runs.
- `performance/` - reference hardware baselines and frozen budgets.
