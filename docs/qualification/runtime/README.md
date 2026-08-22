# Installed Runtime Qualification

This directory records native, black-box evidence for Sluice's Tachyon/Yon and
Fylo process boundaries. A report qualifies only the named artifacts and host.
It is not evidence for another operating system, architecture, release, or
Phase 1 spike.

## Required command

Run the installed-runtime suite with missing binaries treated as failures:

```sh
SLUICE_REQUIRE_INSTALLED_RUNTIMES=1 \
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CC='clang -arch arm64' \
  go test -race -count=1 \
  ./internal/runtime ./internal/state/fylo ./internal/transport/yon
```

The explicit target settings matter when the host Go tool itself runs under
translation: qualification requires the resulting test executables to match
the named native architecture. Substitute the matching native toolchain and
target values in Windows and Linux reports.

Without `SLUICE_REQUIRE_INSTALLED_RUNTIMES=1`, these packages remain usable in
ordinary source checks and skip only the tests whose external artifact is not
installed. A skip is never qualification evidence.

## Reports

- [macOS arm64, 2026-08-21](2026-08-21-macos-arm64.md)
- [macOS arm64 runtime bundle staging, 2026-08-21](2026-08-21-macos-arm64-bundle.md)
- [Unsigned macOS arm64 application topology, 2026-08-21](2026-08-21-macos-arm64-app.md)
- [Linux arm64 runtime and AppDir, 2026-08-21](2026-08-21-linux-arm64-container.md)
- [Runtime bundling contract](BUNDLING.md)

## Promotion rule

The runtime slice is eligible for a target only when that target has a report
showing all of the following against checksummed, version-pinned release
artifacts:

- direct argument-vector execution with no shell;
- bounded, allowlisted child environments;
- Tachyon version rejection before server startup;
- a registered Go executable reached through a real Yon route;
- clean Tachyon stop and same-port restart;
- Fylo version and live capability handshake agreement;
- effective kernel-backed exclusive-root rejection;
- mismatch rejection before the Fylo state root is opened; and
- clean Fylo close, lease release, and restart.

The macOS arm64 report also records the accepted ADR 0008 focused supervisor;
the released Go shim's missing exclusive-root flag is no longer an open Phase
1 dependency. Linux arm64 now has native container and headless AppDir
evidence, but its real desktop gate remains open. Windows, Linux amd64, and
macOS amd64 have no native report. Every support tier remains unqualified.
