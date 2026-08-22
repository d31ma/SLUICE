# IPC Qualification: Linux arm64 Container, 2026-08-21

## Result

Production Unix-domain IPC and the real
`ty -> sluice-yon -> sluice-agent -> fylo` chain passed natively on Linux arm64
at Sluice revision `64ae711f9d6ec5b094044ac758ca6858addf92c9`.

`make linux-qualify-arm64` ran the race-enabled IPC, agent, local API, Yon, and
installed-runtime suites as an unprivileged user. It then started the packaged
AppDir topology twice. Both runs published a fresh private Unix descriptor,
reached authenticated health, and removed the descriptor and socket during
coordinated shutdown. The API token remained stable across the restart.

The environment and pinned image/runtime identities are recorded in
`../runtime/2026-08-21-linux-arm64-container.md`. This evidence applies to the
Linux arm64 ABI and kernel path in that environment. It does not qualify Linux
amd64, a graphical desktop distribution, or the Windows named-pipe path.
