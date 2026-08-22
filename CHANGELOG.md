# Changelog

All notable Sluice changes are recorded here. Sluice follows Semantic
Versioning for application releases; protocol and schema versions are governed
independently by `docs/VERSIONING.md`.

## Unreleased

### Added

- Initial open-source project, architecture, governance, and delivery plan.
- Pinned Tachyon/Fylo runtime contracts, private redistribution manifest,
  focused exclusive-root Fylo supervisor, and durable idempotency/queue
  qualification.
- Native production IPC using Unix-domain sockets on macOS/Linux and a
  current-user-only named-pipe implementation on Windows; explicit loopback
  TCP remains diagnostic-only.
- Native macOS arm64 APFS qualification for real ENOSPC cleanup, case and
  normalization collisions, hostile parent replacement, and pre-rename crash
  recovery.
- Isolated unsigned macOS arm64 `Sluice.app` staging and maintained ten-minute
  qualification of the Fyne/Tachyon/Yon/agent/Fylo topology with authenticated
  health, bounded Tachyon/agent fault recovery, resource measurements,
  capability rotation, and coordinated shutdown.
- Digest-pinned Linux arm64 runtime and AppDir qualification with native Unix
  IPC, XDG state/autostart contracts, authenticated process topology, and
  explicit headless tray/notification limitations; shared handle-relative
  atomic installation qualified on an ext4-compatible volume and tmpfs.
