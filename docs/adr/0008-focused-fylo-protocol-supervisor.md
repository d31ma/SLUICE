# ADR 0008: Use the Focused Fylo Protocol Supervisor

- Status: Accepted; supersedes ADR 0003 only for the Go-shim startup choice
- Date: 2026-08-21

## Context

ADR 0003 requires the agent to own one warm, version-pinned Fylo process with
effective `--exclusive-root` protection. It also required Phase 1 to add or
consume that option through Fylo's released Go shim.

Fylo 26.33.02 implements and advertises exclusive-root ownership, but its
version-matched vendorable Go shim does not expose the option and is not a
standalone versioned Go module. Sluice's focused supervisor now executes the
same bounded machine protocol directly, passes the mandatory flag, verifies
the exact runtime and capabilities before opening state, completes a live
handshake, bounds both frame directions, and owns shutdown and restart.

Vendoring the upstream shim without using it would create a second, unqualified
startup path and would not improve the ownership guarantee.

## Decision

Sluice uses `internal/state/fylo.Supervisor` as its production Fylo integration
for the pinned 26.33.02 runtime. The supervisor must:

- execute the pinned absolute binary directly, never through a shell;
- preflight `fylo version --output json` and require the exact runtime,
  protocol, frame limits, build identity, and capabilities;
- start exactly one warm `fylo exec --loop` process with `--exclusive-root`;
- complete the live handshake before publishing agent availability;
- enforce bounded NDJSON requests and responses;
- prove a second owner receives the stable root-locked failure while the first
  remains responsive; and
- close, release the root lease, and reopen successfully.

The upstream Go shim may replace this supervisor only after a matching released
version exposes the exclusive-root option and passes the same black-box suite.
The change requires an ADR because it replaces a security-sensitive process
boundary; API resemblance alone is insufficient.

## Consequences

- Effective exclusive ownership remains a release-blocking invariant.
- The missing shim option is no longer a Phase 1 dependency gate.
- Sluice owns a small protocol adapter that must track the pinned Fylo release.
- Runtime upgrades must rerun mismatch, ownership, crash, close, and reopen
  contracts before changing the pin.
- There is one production integration path rather than an unused shim beside a
  tested supervisor.

## Rejected alternatives

- Omit `--exclusive-root`: permits competing writers and violates ADR 0003.
- Vendor the current shim but keep the supervisor for real startup: creates
  dead code and a misleading compliance artifact.
- Patch an unversioned local shim copy: obscures provenance and forks the
  runtime contract without an upstream release.
- Wait indefinitely for an upstream API: blocks a behavior Fylo already
  implements and Sluice already qualifies directly.
