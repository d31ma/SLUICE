# macOS arm64 Interactive Desktop Evidence: 2026-08-22

## Result

Partial. The accessibility-enabled unsigned application rendered its real
window and a visibly and programmatically named system-tray item in a native
graphical login session. An authorized native AX audit found all eight visible
status strings. Real close-to-tray, tray restore, application-menu Quit, and the
packaged native smoke passed. Tray-menu interaction, notification delivery,
VoiceOver announcement, keyboard access, logout/shutdown, and a real-login
LaunchAgent start remain open.

## Environment

| Item | Value |
| --- | --- |
| Host | Mac mini, Mac16,10, Apple M4 |
| OS | macOS 26.5.2, build 25F84 |
| Architecture | Native arm64 |
| Desktop session | Logged-in Aqua session with Dock and menu bar |
| Sluice source | `5e2b2a924cf0` |
| Fyne | 2.8.0, built with the opt-in `accessibility` tag |
| Application | Isolated unsigned arm64 `Sluice.app` |

No device serial number, hardware UUID, account path, credential, or private
application-state path is retained in this record.

## Passing observations

The staged application started its bundled Tachyon, Yon, agent, and Fylo
processes from package-relative paths. A window-scoped capture showed the
`Sluice` window, status panels, ready state, and onboarding text rendered
without overlap or clipping after the accessibility composition change. A
separate 77 by 24 pixel menu-bar capture showed only the folder icon and its
visible `Sluice` title; it contained no neighboring account or desktop content.
The transient capture hashes were:

```text
dfe32536ffe39018a46b1883b3a342270173f3fc009727a48c29769ef6862b32  qualified-window.png
5941b4e74eacad2cb84b399fabd667c7e52bbd1dcbc6ef3ec6ed665776c2f7f9  qualified-tray.png
```

Closing the native red window control removed the main window from the
on-screen Quartz list while the agent descriptor, private Unix socket, desktop,
agent, Fylo, and Tachyon processes remained alive. Clicking the app-scoped tray
item restored the same window. Selecting the standard `Quit Sluice` application
menu action then removed the desktop, every packaged child, descriptor, and
socket. The tray-menu `Quit` item remains a separate unobserved action.

The maintained package test then ran the accessibility-enabled executable
twice against one isolated state root. Window show, hide, notification request,
authenticated four-process health, restart, and coordinated shutdown passed.
macOS returned `UNErrorDomain Code=1` for both notification-permission requests,
so delivery is not claimed.

The package builders for macOS, Windows, and Linux now require the same Fyne
accessibility build profile. Hosted CI run
[32558865448](https://github.com/d31ma/sluice/actions/runs/32558865448)
proved native tagged package staging on all three builders. The Windows job
then reached its independently tracked Fylo capability blocker.

## Accessibility observation and workarounds

Before the build correction, macOS exposed only the native window chrome and
title through the accessibility hierarchy; Fyne canvas labels were absent.
Fyne 2.8.0 implements its experimental bridge only when built with
`-tags accessibility`, so the prior package could not satisfy the gate.

The first accessibility-enabled package still exposed only `Sluice`: Fyne's
2.8 walker did not descend through `widget.Card`, so every status string inside
the two cards was absent. The black-box UI contract now rejects that structure,
and Sluice composes each panel from accessible containers over an empty visual
Card. The corrected package exposed exactly these eight user-visible strings to
an assistive-access-authorized, bounded, cycle-aware AX traversal:

```text
Connect an S3-compatible profile and choose a local folder to begin syncing.
Get started
Local agent
Profile and folder setup becomes available after the local service is connected.
Ready for configuration
Sluice
Status
Your storage remains under your control
```

Fyne also left the `AXMenuBarItem` title, description, and help empty. Sluice
now applies the pinned `fyne.io/systray` title and tooltip after the Fyne
lifecycle starts. The exact package exposed `AXTitle=Sluice` and
`AXHelp=Sluice`, and the visible tray label matched. The upstream defects and
workarounds are tracked in
[Fyne issue 6491](https://github.com/fyne-io/fyne/issues/6491) and
[Fyne issue 6492](https://github.com/fyne-io/fyne/issues/6492).

This native API audit proves presence and naming, not what VoiceOver announces
or whether a keyboard-only user can reach and invoke every action. A real
VoiceOver/keyboard session must still verify announcement, focus order, tray
menu access, and each tray command.

## Cleanup and remaining gate

The qualification removed the exact temporary application state it created;
no synchronized user content existed. The generated package and captures lived
only in isolated temporary directories.

The macOS Phase 1 desktop row still requires:

- opening and exercising every tray-menu item;
- notification delivery or a user-observed permission decision;
- VoiceOver announcement, focus order, and keyboard-only actions;
- tray-menu Quit, logout, and shutdown observation; and
- a real LaunchAgent start after logout/login, followed by disable and cleanup.

No support-tier promotion follows from this partial evidence.
