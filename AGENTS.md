# AGENTS.md - Sluice

## Inheritance

This repository inherits all shared workspace instructions from:

`/Users/iyor/Library/CloudStorage/Dropbox/INSTRUCTIONS.md`

Read and follow that file before starting any task in this repository. Its
workflow, engineering standards, validation phases, Graphify requirements, and
safety rules apply here. This file adds Sluice-specific direction and overrides
the shared file only where it says so explicitly.

## Start here

Read these sources before planning or changing Sluice:

1. `README.md`
2. `docs/PROJECT_PLAN.md`
3. Relevant ADRs and API contracts once they exist
4. `graphify-out/GRAPH_REPORT.md` for generated project context

The project plan is the durable product and architecture baseline. Work must
stay within the active phase and satisfy that phase's acceptance gate.

## Current status

Sluice is in Phase 1 feasibility and executable-contract work. The repository
foundation and initial contract packages exist, but no target, integration, or
release tier is supported until the native evidence required by the project
plan is recorded.

## Non-negotiable architecture

- Go is the primary implementation language.
- Tachyon Yon is the versioned REST and supervised-handler boundary.
- Fyne is the shared desktop UI and system-tray framework unless an accepted
  ADR replaces it.
- Fylo is the authoritative local control-state store; SQLite is not part of
  the architecture.
- One persistent `sluice-agent` owns synchronization and the Fylo root.
- Fylo stores catalogs, checkpoints, operations, queues, tombstones, conflicts,
  and redacted events. It does not duplicate every synchronized file's bytes.
- User content remains in ordinary local paths and the configured
  S3-compatible object store.
- The Yon API remains thin. Request handlers do not own watcher state, perform
  long-running transfers, or open competing Fylo writers.
- UI and domain logic are shared. OS filesystem, secret-store, tray lifecycle,
  placeholder, and mobile background behavior live behind explicit platform
  adapters.
- Conflicts preserve every divergent version and require an explicit
  resolution. Never select a winner using timestamps alone.

## Fylo rules

- Use the version-matched Fylo binary and the ADR 0008 focused protocol
  supervisor until a matching released Go shim passes the same contract.
- Require a successful handshake and known capability versions before opening
  production state.
- Run one warm Fylo process with exclusive-root ownership. The focused
  supervisor must pass `--exclusive-root`; never weaken the guarantee because
  the released Go shim lacks the option.
- Keep the Fylo root outside every synchronized root and on a supported local
  filesystem.
- Keep Fylo's optional remote sync hooks disabled for the Sluice state root;
  Sluice owns its remote protocol.
- Use paged queries and bounded machine-protocol frames.
- Respect Fylo's document model: related records belong in separate
  collections rather than arrays of objects.
- Store credentials in native OS secret facilities. Fylo stores only opaque
  secret references.

## Delivery rules

- Implement vertical slices in the order defined by `docs/PROJECT_PLAN.md`.
- Phase 1 feasibility contracts must pass before production sync-engine work.
- Ordinary-folder synchronization ships before placeholder adapters.
- Desktop 1.0 is not blocked by future Android or iOS work.
- Cross-compilation proves buildability only; it does not prove native support.
- Windows, macOS, Linux, individual placeholder adapters, Android, and iOS are
  promoted independently through the documented support tiers.

## Testing and safety

- Test observable CLI, HTTP, IPC, filesystem, Fylo, and S3 behavior.
- Use real `ty`, Yon handler, `sluice-agent`, and `fylo` processes in final
  contract suites.
- Include crash, restart, disk pressure, permission loss, watcher overflow,
  ambiguous S3 responses, path traversal, symlink/reparse, and conflict cases.
- Treat file paths, object keys, manifests, endpoint URLs, Yon requests, Fylo
  responses, and filesystem events as untrusted input.
- Never invoke project processes through a shell.
- Never log credentials, authorization headers, signed URLs, file contents, or
  unredacted sensitive paths.

## Documentation and context

- Keep `README.md`, `docs/PROJECT_PLAN.md`, ADRs, schemas, runbooks, and support
  claims aligned with implemented behavior.
- Record hard-to-reverse decisions as ADRs before implementation.
- Update Graphify after code or documentation changes as required by the shared
  Dropbox instructions.
- Use `make bootstrap` to download the pinned toolchain and dependencies.
- Use `make fmt` to format Go source.
- Use `make test` for the fast test suite and `make check` for the complete
  local quality gate.
- Use `make build` to compile all Go packages.
