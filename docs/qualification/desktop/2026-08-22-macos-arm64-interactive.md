# macOS arm64 Interactive Desktop Evidence: 2026-08-22

## Result

Partial. The accessibility-enabled unsigned application rendered its real
window and system-tray icon in a native graphical login session, and the
packaged native smoke completed twice. Tray-menu interaction, notification
delivery, VoiceOver navigation, keyboard access, and a real-login LaunchAgent
start remain open.

## Environment

| Item | Value |
| --- | --- |
| Host | Mac mini, Mac16,10, Apple M4 |
| OS | macOS 26.5.2, build 25F84 |
| Architecture | Native arm64 |
| Desktop session | Logged-in Aqua session with Dock and menu bar |
| Sluice source | `03e0ca2efb5a` |
| Fyne | 2.8.0, built with the opt-in `accessibility` tag |
| Application | Isolated unsigned arm64 `Sluice.app` |

No device serial number, hardware UUID, account path, credential, or private
application-state path is retained in this record.

## Passing observations

The staged application started its bundled Tachyon, Yon, agent, and Fylo
processes from package-relative paths. A window-scoped capture showed the
`Sluice` window, status card, ready state, and onboarding text rendered without
overlap or clipping. A separate 52 by 32 pixel menu-bar capture showed the
Sluice folder tray icon; it contained no neighboring account or desktop
content. The transient capture hashes were:

```text
dffee16a87c62e0438dbc91758637c648c476153363c90c3e82d113dadef09eb  tagged-window.png
b49f20cb7190dfc57cafe94b1e42f040a480867e4e752714367f0ab1aa312095  tagged-tray.png
```

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

## Accessibility observation

Before the build correction, macOS exposed only the native window chrome and
title through the accessibility hierarchy; Fyne canvas labels were absent.
Fyne 2.8.0 implements its experimental bridge only when built with
`-tags accessibility`, so the prior package could not satisfy the gate.

The corrected direct-binary run created the visible window, but System Events
reported zero accessible windows for that process. A LaunchServices run then
required assistive-access authorization that was not granted to the
qualification process. This record therefore does not claim VoiceOver or
keyboard accessibility. The next run must use a user-authorized Accessibility
Inspector or VoiceOver session and verify every label, focus order, action, and
tray command.

## Cleanup and remaining gate

The qualification removed the exact temporary application state it created;
no synchronized user content existed. The generated package and captures lived
only in isolated temporary directories.

The macOS Phase 1 desktop row still requires:

- opening and exercising every tray-menu item;
- notification delivery or a user-observed permission decision;
- VoiceOver labels, focus order, and keyboard-only actions;
- close-to-tray, explicit Quit, logout, and shutdown observation; and
- a real LaunchAgent start after logout/login, followed by disable and cleanup.

No support-tier promotion follows from this partial evidence.
