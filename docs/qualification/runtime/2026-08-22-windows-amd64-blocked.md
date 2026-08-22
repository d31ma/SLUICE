# Windows amd64 Runtime Qualification Blocker: 2026-08-22

## Result

Blocked by the pinned upstream Fylo `26.33.02` Windows artifact. Native portable
staging, checksum verification, executable-format checks, and the direct
Tachyon runtime contract pass, but the required Fylo capability handshake
fails before state is opened. Windows runtime and package-topology
qualification therefore remain unpromoted.

## Environment and artifacts

| Item | Value |
| --- | --- |
| CI run | [32558865448](https://github.com/d31ma/sluice/actions/runs/32558865448), job `96997210746` (`Installed runtimes windows-latest`) |
| Sluice source | `03e0ca2efb5a2407343dde55c61b0c83370181a8` |
| Host | Microsoft Windows Server 2025 Datacenter 10.0.26100, native amd64 |
| Runner image | `windows-2025-vs2026` version `20260818.207.1` |
| Go toolchain | Go 1.27.0, `windows/amd64` |
| Tachyon artifact | `ty-windows-x64.exe`, SHA-256 `00198a58534faff29f2373a2649c58f176e3305aa77242f59cee6530c7c371d5` |
| Fylo artifact | `fylo-windows-x64.exe`, SHA-256 `d57a612d5abb770bc3e228a08b6a628341364b19fe8c62a6418bbb178ef41f34` |
| Upstream tracking | [FYLO issue 105](https://github.com/d31ma/FYLO/issues/105) |

## Passing evidence

The native staging step downloaded the versioned artifacts, verified both
digests and executable identities, copied the pinned notices, and produced the
declared `Sluice/runtime/bin` portable layout. The desktop executable was
compiled natively with Fyne's opt-in accessibility bridge, so the packaging
change reached the independent runtime gate. The installed-runtime command
then passed `github.com/d31ma/sluice/internal/runtime`, including real Tachyon
startup, Yon execution, graceful Windows control-event shutdown, and restart.
The ordinary native Windows race suite, Windows filesystem gate, CRAP ceiling,
vet, and build passed in the companion `Go windows-latest` job.

## Blocking observation

With `SLUICE_REQUIRE_INSTALLED_RUNTIMES=1`, each native Fylo supervisor contract
failed closed with the same handshake result:

```text
fylo runtime is incompatible: machine access is 0, require 1
```

That rejected:

- completed idempotency replay and changed-payload conflict;
- pending idempotency fail-closed behavior;
- durable queue claim recovery after owner restart; and
- exclusive root ownership, clean close, and restart.

The downstream four-process chain then timed out waiting for the agent
descriptor because the agent correctly refused to open production state with
an incompatible Fylo process. The Windows package-topology step was skipped by
the failed prerequisite; it is not evidence of an independent package defect.

## Unblock and promotion rule

Retry only after Fylo publishes a version-matched Windows amd64 release whose
live machine handshake advertises `machineAccess >= 1`. Update the pinned
artifact URL and digest through the runtime manifest review, rerun the required
native installed-runtime and two-start package-topology suites, and promote the
manifest only if every contract passes. Sluice must not lower the capability
requirement or bypass the handshake to make this job green.
