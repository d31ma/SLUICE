# Desktop Native Qualification

This directory records evidence for Sluice's desktop lifecycle and
start-at-login adapters. It uses the support vocabulary in
`docs/PROJECT_PLAN.md`: a cross-build is buildability evidence only, while a
native-tested claim requires the named suite to execute on the target OS.

## Current evidence

| Target | Build evidence | Native evidence | Current conclusion |
| --- | --- | --- | --- |
| macOS 26.5.2 arm64 | Native arm64 Fyne binary built | Automated window, tray API, hide/show, notification API, clean quit, and isolated LaunchAgent lifecycle passed on 2026-08-21 | Phase 1 partial; visual tray/notification, real-login launch, accessibility, installer, and agent shutdown remain |
| Windows amd64 | Full Fyne binary cross-built as PE32+ with Zig; autostart test binary cross-built | None | Buildable only; registry and desktop lifecycle are not native-tested |
| Linux amd64 | Autostart test binary cross-built as a static ELF | None; full Fyne cross-build lacks local Wayland/OpenGL development files | Autostart package buildable only; desktop target is not native-tested |

No desktop target is currently Preview or Supported. Windows and Linux must
not inherit the macOS result.

## Executable interface

The desktop binary provides these observable commands:

```text
sluice-desktop
sluice-desktop --background
sluice-desktop --native-smoke
sluice-desktop autostart enable
sluice-desktop autostart disable
sluice-desktop autostart status
```

Normal launch opens the shared Fyne window and installs the system-tray menu.
`--background` starts with the window hidden, as used by every autostart
registration. The tray can show or hide the window and is the only UI action
that quits the application.

`--native-smoke` opens the real Fyne application, then exercises hide, show,
notification dispatch, and clean quit after a bounded delay. A zero exit code
proves that the native code path completed without a panic or lifecycle error;
it cannot prove that a desktop shell rendered a tray icon or displayed a
notification. Those observations remain manual native gates.

The current command owns only the Fyne lifecycle. It does not invent a fake
sync agent. Coordinated shutdown with the real `sluice-agent` remains part of
the separate Phase 1 process-topology qualification.

## Native test requirements

Each native run must record:

1. OS version, architecture, desktop environment, and physical or virtual
   hardware.
2. Exact application artifact and source revision.
3. Autostart disabled, enabled, repair, and disabled-again results.
4. A logout/login or reboot proving the exact installed binary starts hidden
   and presents a working tray icon.
5. Window close behavior, tray show/hide, notification display or documented
   permission denial, explicit quit, and OS shutdown behavior.
6. Any accessibility, packaging, signing, or desktop-environment limitation.

Platform mechanisms and safety properties are documented in
`AUTOSTART_CONTRACT.md`. The first macOS execution record is in
`MACOS-2026-08-21.md`.
