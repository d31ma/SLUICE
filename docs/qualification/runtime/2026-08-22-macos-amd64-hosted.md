# Runtime and Package Staging: macOS amd64, 2026-08-22

## Result

The pinned macOS amd64 Tachyon/Fylo payload, installed runtime contracts, and
unsigned `Sluice.app` runtime topology passed on GitHub's native Intel macOS
runner. The app started its bundle-resolved agent, Fylo, Tachyon, and Yon
processes twice against one isolated state root, preserved its API token, and
removed its descriptor, Unix socket, and child processes after shutdown.

This is native amd64 staging, ABI, runtime, and non-graphical package-lifecycle
evidence. It is not evidence of a rendered Fyne window, tray icon,
notification, login launch, accessibility behavior, signing, notarization, or
any macOS support tier.

| Item | Evidence |
| --- | --- |
| CI run | [GitHub Actions run 32545842856](https://github.com/d31ma/sluice/actions/runs/32545842856), job `Installed runtimes macos-15-intel` |
| Host | GitHub-hosted `macos-15-intel`; macOS 15.7.7 build 24G720; runner image `macos-15` version `20260727.0377.1` |
| Architecture and Go | `darwin/amd64`; Go 1.27.0 |
| Sluice source | `f2b888dde9d7f3ad264b0bea06e18d0c1fa07ec1` |
| Tachyon | 26.33.02 `ty-macos-x64`, checksum `b5fd0495a18a1923a132275d15358e170e66516e0f47b1f54f159031cec5d457` |
| Fylo | 26.33.02 `macos-x64`, commit `e236162508f6789f980fa7ca136a6671938030cb`, checksum `551e72291a7085536f5de360807dbe93a149826f2c0e6bbd7fe56b2e34abc545` |

## Executed contracts

The job downloaded both release artifacts over TLS, verified their SHA-256
digests and Mach-O x86_64 identities, required their exact versions and Fylo
build target, and staged the declared license/notice tree. It then built the
three Go executables as native amd64 Mach-O programs and validated the unsigned
application layout.

With required-runtime skips disabled, the following race-enabled suites passed:

- runtime executable validation;
- Fylo capability handshake, exclusive-root contention, durable idempotency,
  pending-reservation failure, queue recovery, close, and reopen;
- production Unix IPC, agent, authenticated local API, and Tachyon/Yon bridge;
  and
- two package-resolved four-process starts with stable token, native socket,
  authenticated health, restart, and coordinated cleanup.

The package smoke used `--packaged-runtime-smoke`. The hosted Intel runner did
not expose an NSGL pixel format usable by GLFW, so the test intentionally did
not initialize Fyne. The earlier native UI attempt failed with
`NSGL: Failed to find a suitable pixel format`; this is a runner limitation,
not visual qualification evidence.

## Remaining gates

- Run the real `--native-smoke` path in an interactive Intel macOS login
  session and record window, tray/menu, notification, keyboard, accessibility,
  real-login autostart, and shutdown behavior.
- Signing, notarization, installation, upgrade, rollback, SBOM, and provenance
  remain Phase 8 and Phase 9 work.
