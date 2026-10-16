# Cloud-Drive Client Behavior Matrix

Status: public-documentation baseline; controlled observations pending

Last reviewed: 2026-08-22

This matrix records user-visible behavior, not implementation internals. A
documentation link establishes that a vendor describes the behavior; it does
not prove current native behavior on every platform. Native observations use
the protocol and scenario IDs in `CLIENT_STUDY_PROTOCOL.md` before they are
promoted from pending.

| Concern | Dropbox | OneDrive | Google Drive | Proton Drive | Sluice requirement |
| --- | --- | --- | --- | --- | --- |
| Persistent entry point | Desktop icon exposes sync status and menu actions ([official help](https://help.dropbox.com/installs/dropbox-icon-in-taskbar)) | Notification-area/menu-bar icon communicates sync state ([icon reference](https://support.microsoft.com/en-gb/office/what-do-the-onedrive-icons-mean-11143026-8000-44f8-aaa9-67c985aa49b3)) | Menu-bar/system-tray app exposes status and settings ([official help](https://support.google.com/drive/answer/10838124)) | Tray icon exposes state and open/pause/exit actions ([Windows guide](https://proton.me/support/proton-drive-windows-app)) | The agent outlives the window; the tray always exposes global status, open, pause/resume, and explicit quit |
| Idle and progress | Icon and menu report current sync condition ([status help](https://help.dropbox.com/installs/dropbox-icon-in-taskbar)) | Icons distinguish processing, paused, error, and availability states ([icon reference](https://support.microsoft.com/en-gb/office/what-do-the-onedrive-icons-mean-11143026-8000-44f8-aaa9-67c985aa49b3)) | Desktop app reports sync activity and errors ([troubleshooting](https://support.google.com/drive/answer/2565956?hl=en-GB)) | Activity view shows real-time progress and errors; hover text reports state ([Windows guide](https://proton.me/support/proton-drive-windows-app)) | Use one bounded state vocabulary across tray/window/mobile; show useful aggregate progress without leaking file paths |
| Pause and resume | Global pause/resume is a documented tray action ([official help](https://help.dropbox.com/sync/pause-resume)) | Pause/resume is exposed for synchronization ([official help](https://support.microsoft.com/en-us/onedrive/stop-syncing-a-library-with-the-onedrive-for-work-or-school-app)) | Controlled observation pending | Tray menu exposes Pause sync ([Windows guide](https://proton.me/support/proton-drive-windows-app)) | Pause is explicit, durable, and describes that changes remain queued; resume never implies conflict-free completion |
| Offline/on-demand availability | Selective sync removes selected local folders while retaining cloud content ([official help](https://help.dropbox.com/sync/selective-sync-overview)) | Files On-Demand distinguishes online-only, locally available, and always available ([official help](https://support.microsoft.com/en-us/office/save-disk-space-with-onedrive-files-on-demand-for-windows-0e6860d3-d9f3-4971-b321-7092438fb38e)) | Stream and mirror modes expose different local-availability behavior ([official help](https://support.google.com/drive/answer/13401938)) | macOS exposes syncing, error, paused, and offline states ([official help](https://proton.me/support/proton-drive-macos-sync-status)) | Ordinary folders ship first; future placeholder adapters use explicit availability terminology and never confuse logical with resident bytes |
| Conflict outcome | Conflict guidance makes alternate versions visible rather than silently replacing them ([example](https://help.dropbox.com/sync/encrypted-file-folder-conflict)) | Controlled observation pending | Mirror-mode conflicts retain both versions ([official help](https://support.google.com/drive/answer/13401938)) | Controlled observation pending | Preserve every divergent version, show a conflict count and cause, and require explicit resolution; timestamps never select a winner |
| Error and recovery | Preferences and status surfaces centralize configuration and diagnosis ([preferences](https://help.dropbox.com/account-settings/view-preferences)) | Error overlays and account states are differentiated ([icon reference](https://support.microsoft.com/en-gb/office/what-do-the-onedrive-icons-mean-11143026-8000-44f8-aaa9-67c985aa49b3)) | Troubleshooting identifies sync errors and restart/reconnect paths ([official help](https://support.google.com/drive/answer/2565956?hl=en-GB)) | Activity view exposes errors and recovery guidance; common issues document local recovery ([guide](https://proton.me/support/proton-drive-windows-common-issues)) | Separate offline, recoverable attention, and conflict states; give one safe primary action and retain detailed redacted diagnostics |
| Startup and background lifetime | Controlled observation pending | Controlled observation pending | Controlled observation pending | Starts at boot by default and remains tray-controlled ([Windows guide](https://proton.me/support/proton-drive-windows-app)) | Start-at-login is explicit and reversible; hiding/closing the window never stops active synchronization; Quit is unambiguous |

## Derived priorities

1. Make current state and the safest next action understandable from the tray
   without opening the main window.
2. Treat offline, paused, attention, and conflict as distinct conditions.
3. Preserve work across window close, app restart, network loss, and machine
   restart.
4. Keep file-availability language separate from sync-completion language.
5. Make conflict preservation a visible product promise, not a diagnostic
   afterthought.

These priorities are requirements candidates. Phase 5 acceptance still needs
real Yon-driven UI tests and native evidence; this matrix cannot satisfy that
gate.
