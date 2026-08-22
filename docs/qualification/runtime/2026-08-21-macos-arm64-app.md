# Unsigned Sluice.app Topology: macOS arm64, 2026-08-21

## Result

The isolated unsigned macOS arm64 application-bundle qualification passed on
the named native host. The staged app contained `sluice-desktop`,
`sluice-agent`, `sluice-yon`, the minimal Tachyon health route, pinned
Tachyon/Fylo 26.33.02 executables, `Info.plist`, the Sluice MIT license, and
the pinned upstream license and notice files.

This is native Phase 1 technical-bundling evidence. It is not signing,
notarization, installation, upgrade, rollback, or a macOS support-tier claim.

Evidence was captured on 2026-08-21. The maintained lifecycle measurements
below supersede the earlier bounded-smoke lifecycle result.

| Item | Evidence |
| --- | --- |
| Host | macOS 26.5.2, build 25F84, Darwin 25.5.0 arm64 kernel |
| Build shell | Rosetta-translated amd64 Go host; target was explicitly `GOOS=darwin GOARCH=arm64` |
| Go toolchain | Go 1.27.0 |
| Sluice source | `8f7da959bc5a6945cab698ddb8f7af8c907f9abe` |
| Application | Unsigned arm64 `Sluice.app` in a private temporary directory |
| Runtime discovery | Every executable resolved relative to `Contents/MacOS/sluice-desktop`; qualification `PATH` contained only `/usr/bin:/bin` |
| Local IPC | Private Unix socket descriptor; descriptor and socket absent after shutdown |
| Restart | Two complete starts used the same isolated Fylo root and stable private API token |

## Command and observed topology

```sh
make app-package-macos
# app bundle contract: Info.plist and Tachyon health route passed
# runtime bundle contract: 5 targets, 10 checksummed artifacts
# runtime staging: verified tachyon ty-macos-arm64
# runtime staging: verified fylo fylo-macos-arm64
# app bundle staging: unsigned macOS arm64 Sluice.app passed
# app bundle test: packaged health, native IPC, restart, and clean shutdown passed
```

On each start, the desktop coordinator:

1. created or securely reopened the private bearer token;
2. launched the bundled agent without the diagnostic `--listen` override;
3. waited for the agent's new private IPC descriptor after Fylo became ready;
4. materialized an ephemeral Tachyon project whose `.tachyonrc` named the
   absolute, bundle-resolved `sluice-yon` helper;
5. launched the bundled Tachyon binary on a random loopback port;
6. required an authenticated `GET /v1/health` response with status `ok` before
   showing the Fyne shell; and
7. stopped Tachyon first, then the agent and its warm Fylo child.

The qualification invoked the arm64 desktop executable twice with
`--native-smoke` and a temporary `--qualification-state-root`. Each bounded
run exercised show, hide, notification request, and Quit. The temporary app
was not registered with macOS notification services, so the OS denied the
notification permission request; delivery and visual appearance remain manual
registered-build evidence rather than a claim of this test.

## Maintained lifecycle and recovery

The same source revision passed the real maintained-background qualification:

```sh
make app-runtime-qualify-macos
# app runtime qualification: PASS
# native target: macOS arm64
# authenticated startup: 869 ms
# idle warmup: 30 s
# recovery during warmup: no
# idle duration: 600 s
# idle samples: 61
# aggregate CPU from process time: 0.855%
# agent CPU from process time: 0.002%
# agent+Fylo CPU from process time: 0.002%
# aggregate sampled CPU mean/max: 0.577% / 1.500%
# all-process RSS mean/max: 106599 KiB / 186256 KiB
# desktop+agent RSS mean/max: 95751 KiB / 172528 KiB
# authenticated recovery after Tachyon termination: 480 ms
# authenticated recovery after agent termination: 372 ms
# clean shutdown: 59 ms
# desktop survived both faults: yes
# stable token and Fylo root: yes
# descriptor capability rotated safely: yes
```

One unchanged desktop/Tachyon/agent/Fylo generation remained healthy through
the 30-second warmup and 600-second post-warmup sample; no process was replaced
during that sustained interval. After sampling, the qualification separately
sent `TERM` to Tachyon and `KILL` to the agent. The desktop survived each fault,
retired the old agent/Tachyon/Fylo generation, restored authenticated health,
and rotated the private IPC capability while preserving the API token and Fylo
root. Clean desktop shutdown then removed the descriptor and socket and left no
packaged child running. Detailed measurement interpretation is recorded in
`../performance/2026-08-21-macos.md`.

Earlier repeated 60-second development observations found that Tachyon could
intermittently exit after receiving `SIGTERM` without an actionable preceding
diagnostic. That discovery produced the generation supervisor qualified above
and was reported upstream as
[Tachyon issue 169](https://github.com/d31ma/Tachyon/issues/169). The stable
ten-minute sample is recorded separately from that development observation and
from the deliberately forced recovery checks.

## Remaining Phase 1 gates

- Record visual tray/menu/notification behavior and accessibility on a
  registered build, and prove start-at-login during a real login.
- Repeat package and lifecycle qualification natively on Windows, Linux amd64,
  and macOS amd64. Linux arm64 has separate headless package evidence but still
  needs a real graphical desktop run.

## Later release gates

- Sign, notarize, install, upgrade, and roll back the final macOS package in
  Phases 8 and 9.
- Include and audit final dependency SBOMs and provenance before release
  promotion.
