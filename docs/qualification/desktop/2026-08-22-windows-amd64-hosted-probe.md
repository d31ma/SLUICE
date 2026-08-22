# Windows amd64 Hosted Desktop Probe - 2026-08-22

## Result

Rejected as native desktop qualification. The accessibility-enabled Windows
binary executed on a native Windows Server 2025 amd64 GitHub-hosted runner, but
Fyne could not create a window because the runner exposed no usable WGL/OpenGL
driver. The command initially returned zero despite that logged initialization
failure. Sluice now treats Fyne's `window creation error` as a failed
`--native-smoke` contract, and CI records this hosted limitation without
promoting the runner as graphical evidence.

## Environment

| Item | Value |
| --- | --- |
| OS | Microsoft Windows Server 2025 Datacenter, 10.0.26100 |
| Architecture | Native amd64 |
| Runner image | `windows-2025-vs2026`, image `20260818.207.1` |
| Runner image provisioner | `20260729.566` |
| Sluice source | `df1090730cf9` |
| Fyne | 2.8.0, built with the opt-in `accessibility` tag |
| Hosted run | [32561676680](https://github.com/d31ma/sluice/actions/runs/32561676680) |

No private runner identifier, worker identifier, token, credential, or
qualification-state path is retained in this record.

## Probe and observed failure

The native runner executed:

```text
go run -tags accessibility ./cmd/sluice-desktop --native-smoke
```

The real GLFW/Fyne driver reported:

```text
Fyne error:  window creation error
Cause: APIUnavailable: WGL: The driver does not appear to support OpenGL
```

The system-tray implementation also reported that the tray was not ready. No
window, tray icon, menu, notification, accessibility tree, or user interaction
is claimed. The ordinary Windows race suite, destructive NTFS qualification,
vet, build, and portable-package staging remain valid independent evidence.

## Fail-closed correction

`--native-smoke` previously scheduled hide, show, notification, and quit calls
but did not surface Fyne's logged window-initialization failure. The smoke now
captures Fyne's native lifecycle log and returns a stable error when the driver
reports `window creation error`. Notification permission or delivery errors
remain observable outcomes and do not masquerade as window failures.

The hosted workflow treats this specific rejected-environment result as an
informational probe. Any other nonzero result still fails the job. A future
hosted image that returns zero would prove only that the automated native
lifecycle completed; it would not replace the manual observations below.

## Remaining native gate

Windows Phase 1 still requires a native amd64 graphical login session with a
usable GPU or software OpenGL driver, the corrected Fylo artifact, and the full
record in `README.md`: real login/autostart, visible window and tray, every tray
action, notification outcome, screen-reader and keyboard behavior, explicit
quit, logout, and shutdown. This rejected hosted probe promotes no support
tier.
