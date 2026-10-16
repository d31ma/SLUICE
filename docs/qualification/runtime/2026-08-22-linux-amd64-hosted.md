# Linux amd64 Runtime and AppDir: 2026-08-22

## Result

The pinned Linux amd64 Tachyon/Fylo payload, installed runtime contracts, and
unsigned `Sluice.AppDir` topology passed on a native GitHub-hosted Ubuntu amd64
runner. The AppDir started the real desktop, Tachyon, agent, and Fylo processes
twice against one isolated state root, reached authenticated health, preserved
its API token, and left no packaged child or IPC resource after shutdown.

This is native Linux amd64 ABI, runtime, staging, and headless application
lifecycle evidence. It is not evidence that a graphical desktop rendered a
tray or notification, and it is not a general Linux support claim.

| Item | Evidence |
| --- | --- |
| CI run | [GitHub Actions run 32545842856](https://github.com/d31ma/sluice/actions/runs/32545842856), job `Installed runtimes ubuntu-latest` |
| Host | GitHub-hosted Ubuntu 24.04.4 LTS; runner image `ubuntu-24.04` version `20260816.277.1` |
| Architecture and Go | `linux/amd64`; Go 1.27.0 |
| Sluice source | `f2b888dde9d7f3ad264b0bea06e18d0c1fa07ec1` |
| Tachyon | 26.33.02 `ty-linux-x64`, checksum `c662da31be584364b1d0554df232336a047b90ba66ab9bde9c87e248bb184616` |
| Fylo | 26.33.02 `linux-x64`, commit `e236162508f6789f980fa7ca136a6671938030cb`, checksum `8d60479cc189afb991d969917158d93d78897dcd0ca3aa7e062be186982644e8` |

## Executed contracts

The job downloaded both release artifacts over TLS, verified their SHA-256
digests and ELF x86-64 identities, required their exact versions and Fylo build
target, and staged the declared license/notice tree. It built the desktop,
agent, and Yon helper for native amd64, validated the desktop entry and dynamic
library closure, and exercised the AppDir under isolated Xvfb and D-Bus.

With required-runtime skips disabled, the following race-enabled suites passed:

- runtime executable validation;
- Fylo capability handshake, exclusive-root contention, durable idempotency,
  pending-reservation failure, queue recovery, close, and reopen;
- production Unix IPC, agent, authenticated local API, and Tachyon/Yon bridge;
  and
- two package-resolved four-process starts with stable token, native socket,
  authenticated health, restart, and coordinated cleanup.

## Honest limits

Xvfb and the isolated D-Bus session do not prove tray/menu rendering,
notification delivery, keyboard behavior, accessibility, or a real graphical
login/autostart session. This runtime run does not itself prove destructive
filesystem behavior; that separate gate later passed in
`../filesystem/2026-08-22-linux-amd64-ext4-tmpfs.md`.
Final AppImage construction, distro integration, installation, signing,
upgrade/rollback, SBOM, and provenance remain Phase 8 and Phase 9 work.
