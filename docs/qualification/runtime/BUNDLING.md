# Tachyon and Fylo Runtime Bundling Contract

## Scope

Sluice packages exact Tachyon `ty` and Fylo executables as private runtime
dependencies. They are not discovered through `PATH`, downloaded at
application startup, or replaced by an installer script from either upstream
project.

The authoritative metadata is
[`packaging/runtime/manifest.json`](../../../packaging/runtime/manifest.json).
It pins Tachyon and Fylo 26.33.02 release assets, SHA-256 digests, upstream
source commits, MIT license text, and Tachyon's upstream notice. Every URL
names a versioned release asset or full source commit. URLs using `latest`, a
branch name, a query, or a redirect service are prohibited by the contract
checker. A versioned release URL is never trusted by itself: the recorded
SHA-256 digest remains the identity of the bytes if an upstream asset is ever
deleted and re-uploaded.

## Payload layout

All manifest paths are relative to an empty package staging root. This keeps
verification separate from privileged installation.

| Target | Runtime paths beneath staging root | Notice directory | Status |
| --- | --- | --- | --- |
| macOS arm64 | `Sluice.app/Contents/Resources/runtime/bin/{ty,fylo}` | `Sluice.app/Contents/Resources/licenses` | Native staging passed on the named host |
| macOS amd64 | `Sluice.app/Contents/Resources/runtime/bin/{ty,fylo}` | `Sluice.app/Contents/Resources/licenses` | Native staging and headless package-runtime topology passed on the named host |
| Linux arm64 | `Sluice.AppDir/usr/lib/sluice/runtime/bin/{ty,fylo}` | `Sluice.AppDir/usr/share/doc/sluice/licenses` | Native staging passed in the named Linux arm64 container environment |
| Linux amd64 | `Sluice.AppDir/usr/lib/sluice/runtime/bin/{ty,fylo}` | `Sluice.AppDir/usr/share/doc/sluice/licenses` | Native staging and headless AppDir topology passed on the named host |
| Windows amd64 | `Sluice/runtime/bin/{ty.exe,fylo.exe}` | `Sluice/licenses` | Not native-tested |

The macOS application resolves executables from its resources directory. Both
Linux AppDir targets resolve them beneath `usr/lib/sluice` from their real
`AppRun` locations. The Linux manifest paths are relative to the AppDir root;
the table shows their complete path beneath the package staging root. A future
Windows installer must preserve the declared relative layout within its
package root and then pass native staging and installed-runtime suites before
its status changes. This document does not claim that final installers exist
or work.

## Verification order

Packaging must fail closed in this order:

1. Validate the manifest shape, exact upstream repository, versioned URLs,
   digest syntax, target matrix, safe relative paths, and bundled notice
   digests with `make runtime-contract`.
2. Download an exact release asset over HTTPS into an isolated staging
   directory. Never execute an upstream installer or fetch a `latest` URL.
3. Verify SHA-256 before copying or setting the executable bit.
4. Copy the exact bundled license and notice files into the target's declared
   notice directory.
5. Validate the native executable format and run `ty --version` plus
   `fylo version --output json` directly, without a shell wrapper. Require the
   exact version, Fylo source commit, and Fylo build target.
6. Before support promotion, run the installed-runtime contract suite against
   the final installed package on that same target. Staging success alone is
   not installer or support evidence; final installation belongs to Phases 8
   and 9 rather than the Phase 1 unsigned-topology gate.

`make runtime-stage-macos` performs steps 1 through 5 on a native Apple
Silicon host. It creates its root with `mktemp`, removes it on exit, and never
touches system application or binary directories.

`make app-package-macos` extends that isolated staging contract into an
unsigned native arm64 `Sluice.app`. It builds the three Sluice executables,
adds the minimal Tachyon route and `Info.plist`, starts the full packaged
topology twice with a restricted `PATH`, proves authenticated health, and
requires clean descriptor/socket/process shutdown. The native result is
recorded in
`docs/qualification/runtime/2026-08-21-macos-arm64-app.md`.

`make linux-qualify-arm64` performs the equivalent Linux arm64 artifact and
AppDir checks in a digest-pinned native arm64 container, then executes the
installed-runtime, production Unix IPC, XDG, filesystem, and headless Fyne
lifecycle suites. The Tachyon 26.33.02 Linux artifact requires glibc 2.39 or
newer; the accepted environment uses glibc 2.41. The result and graphical
desktop limitations are recorded in
`docs/qualification/runtime/2026-08-21-linux-arm64-container.md`.

The native hosted amd64 runs use the same manifest and staging contracts. The
Linux run exercises the AppDir twice under isolated Xvfb and D-Bus; the macOS
Intel run exercises the packaged four-process runtime twice without
initializing Fyne because the hosted runner has no usable NSGL pixel format.
Their evidence and limits are recorded in
`docs/qualification/runtime/2026-08-22-linux-amd64-hosted.md` and
`docs/qualification/runtime/2026-08-22-macos-amd64-hosted.md`.

## Legal materials

The redistributed copies are:

- Tachyon MIT license and NOTICE from commit
  `f96c1589ee33b047044f220448268d32ad86e58f`, stored under
  `third_party/tachyon/`;
- Fylo MIT license from commit
  `e236162508f6789f980fa7ca136a6671938030cb`, stored under
  `third_party/fylo/`.

The source URLs and file digests are part of the manifest and are validated
offline. Tachyon publishes a release SBOM, while Fylo publishes an SPDX JSON
SBOM; collecting and verifying those dependency notices in a final installer
remains a release-engineering gate beyond this two-runtime license contract.

## Promotion rule

Adding an artifact or cross-compiling a package does not qualify a target.
Change `qualification` to `native-staging-passed` only in the same change that
adds a native evidence report naming the host, artifact digests, command, and
results. macOS amd64 and Linux amd64 meet that narrow staging rule through the
2026-08-22 hosted reports. Windows remains `not-native-tested`: its native
portable layout stages successfully, but the pinned Fylo artifact prevents the
required installed-runtime and full package-topology run.
