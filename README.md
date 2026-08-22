# Sluice

Sluice is a planned open-source file synchronization client backed by
user-controlled S3-compatible object storage.

The initial release targets Windows, macOS, and Linux as a native desktop
application that runs continuously from the operating-system system tray.
Android and iOS are future targets.

A sluice is a controlled channel between two bodies of water. The product uses
the same model: a controlled, observable, and recoverable channel between local
files and an object-storage bucket.

## Why

Existing clients commonly fail at one or more of these boundaries:

- Unfiltered filesystem watchers allow `.git`, `node_modules`, and build output
  to create indexing storms.
- Per-object request latency makes large trees unusably slow without bounded
  concurrency.
- Placeholder files can make reported disk use differ from resident disk use.
- Conflict policies can silently choose a winner and discard a user version.
- Storage provider, location, and pricing are often controlled by the product
  vendor rather than the user.

## Product principles

1. Users control the S3-compatible endpoint, bucket, region, credentials, and
   cost model.
2. Regenerable directories are ignored by default.
3. Every external operation is resumable from durable local state.
4. Conflicts preserve both versions and require an explicit resolution.
5. Transfers use bounded concurrency and backpressure.
6. One Go sync engine owns synchronization behavior across targets.
7. Frontend view and interaction code is shared; unavoidable OS filesystem and
   lifecycle integrations remain thin platform adapters.
8. Local control state is stored in Fylo rather than SQLite.

## Architecture

```text
Fyne desktop UI and system tray
             |
      Tachyon/Yon REST API
             |
   persistent Go sync agent
       /             \
Fylo local state    S3-compatible storage
```

- **Language:** Go.
- **Backend framework:** [Tachyon Yon](https://tachyon.del.ma), using registered
  Go executable handlers as a versioned REST boundary.
- **Frontend:** [Fyne](https://fyne.io) for one Go UI codebase and native
  desktop system-tray integration.
- **Local state:** [Fylo](https://fylo.del.ma), accessed through the ADR 0008
  focused Go supervisor and versioned machine protocol by one long-running
  agent.
- **Remote content:** a user-selected S3-compatible bucket using a versioned
  Sluice object and manifest protocol.

Fylo stores the local catalog, checkpoints, operation ledger, durable work
queue, tombstones, and conflicts. User file bytes remain in their ordinary
local paths and the configured S3 bucket; Fylo is not a duplicate content
store. Credentials are kept in the operating system's native secret store.

## Platform integration

The shared Go engine and Fyne views do not remove platform differences:

| Target | Integration direction |
| --- | --- |
| Windows | Native watcher, Credential Manager, and Cloud Files API for future placeholders |
| macOS | Native watcher, Keychain, and File Provider for future placeholders |
| Linux | Native watcher, Secret Service, and optional FUSE for future placeholders |
| Android | Future SAF/DocumentsProvider and foreground/background work adapters |
| iOS | Future File Provider, Keychain, and background-task adapters |

Desktop ordinary-folder synchronization ships before placeholder adapters.
Each placeholder and mobile target is qualified and promoted independently.

## Status

Phase 1 feasibility and executable-contract work is underway. Native macOS
arm64 evidence now covers the pinned Tachyon/Fylo runtime boundary, private
Unix-socket Yon-to-agent bridge, APFS atomic-install failure corpus, and an
unsigned four-process `Sluice.app` topology. The packaged app also passes a
maintained ten-minute resource baseline, bounded Tachyon/agent recovery, and
clean shutdown. Native Linux arm64 evidence covers the pinned runtimes,
production Unix IPC, and a headless AppDir lifecycle; its real desktop-shell
gate remains open. Native Linux amd64 now passes the same pinned-runtime and
headless AppDir topology, while native macOS amd64 passes pinned runtimes and
the non-graphical packaged topology. Windows portable-package staging and its
ordinary Go suites run natively, but the pinned Fylo artifact advertises an
insufficient machine-access capability and blocks the full chain. MinIO and
RustFS pass the current S3 provider contract. Destructive Linux amd64 and
Windows filesystem qualification, interactive desktop execution, authorized
AWS S3, and visual/accessibility and real-login checks remain open, so no
operating system, storage provider, installer, or release tier is supported
yet.

Canonical development commands:

```sh
make bootstrap
make test
make check
make build
# Native Linux arm64 Docker engine only:
make linux-qualify-arm64
```

Bootstrap downloads the pinned Go modules plus the checksum-locked Tachyon and
Fylo tools for the current supported host into the ignored `tools/bin`
directory. The complete quality gate uses those native runtimes for executable
contract coverage.

The authoritative Phase 1 evidence and remaining gates are in
[the qualification matrix](docs/qualification/README.md). Cross-compilation
and unsigned staging are deliberately not presented as support claims.

The durable product, architecture, security, test, release, and phased delivery
baseline is in [the project plan](docs/PROJECT_PLAN.md).

## Prior art

- `rclone bisync` for bidirectional object-storage synchronization.
- `syncthing` for continuous reconciliation and conflict behavior.
- Dropbox, OneDrive, and Google Drive for tray lifecycle and placeholder UX.
