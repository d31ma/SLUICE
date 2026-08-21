# ADR 0002: Separate UI, Yon API, Sync Agent, and Fylo

- Status: Accepted
- Date: 2026-08-21

## Context

Yon handlers are bounded request processes. Continuous filesystem observation,
transfer scheduling, and single-writer Fylo state require a persistent owner.
Running the sync engine inside UI or request-handler lifecycles would couple
durability to short-lived processes and invite multiple Fylo writers.

## Decision

Use four explicit desktop lifecycle boundaries:

1. `sluice-desktop` owns Fyne windows and tray lifecycle.
2. Tachyon Yon owns the authenticated loopback REST boundary.
3. `sluice-agent` is the persistent sync engine and sole state owner.
4. One warm `fylo` child is supervised by the agent.

Yon handlers bridge low-frequency UI commands to the agent through a local
Unix domain socket on macOS/Linux or named pipe on Windows. Transfers and
watchers do not traverse Yon.

## Consequences

- Each process has one auditable responsibility.
- UI closure does not stop synchronization.
- Yon process isolation does not interrupt durable sync state.
- Installers must ship and supervise multiple version-pinned executables.
- Phase 1 must measure startup, shutdown, crash, and upgrade behavior.

## Rejected alternatives

- Sync in the Fyne process: window lifecycle can terminate active work.
- Sync in Yon handlers: request processes are not durable daemon state.
- Direct UI-to-agent only: violates the selected Yon backend boundary.
- Multiple Fylo clients: violates exclusive root ownership.
