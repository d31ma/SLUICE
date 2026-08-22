# Linux arm64 Runtime and AppDir: 2026-08-21

## Result

The pinned Linux arm64 Tachyon/Fylo payload, production Unix IPC chain, and
unsigned `Sluice.AppDir` topology passed in a native Linux arm64 Docker engine.
The AppDir started the real desktop, Tachyon, agent, and Fylo processes twice
against one isolated state root, reached authenticated health, preserved its
API token, and left no packaged child or IPC resource after shutdown.

This is native Linux ABI, kernel, runtime, and headless application-lifecycle
evidence. It is not evidence that a Linux desktop shell rendered the tray or a
notification, and it is not an amd64 or general Linux support claim.

| Item | Evidence |
| --- | --- |
| Sluice source | `64ae711f9d6ec5b094044ac758ca6858addf92c9` |
| Engine | Docker 29.7.2 server, Linux arm64 |
| Kernel | LinuxKit 7.0.12, aarch64 |
| Qualification image | `golang@sha256:6237d172b66951ce3ebfe0156587ad41a9773772c061fb9a5e3d612f5fd22614` |
| Userspace | Debian Trixie; glibc 2.41 |
| Go | 1.27.0 linux/arm64 |
| Filesystems | Anonymous Docker volume reported as `ext2/ext3`, plus isolated 64 MiB tmpfs |
| Tachyon | 26.33.02 `ty-linux-arm64`, checksum `2e78dd4b46a300196e64d6563cbf53b77aa4fc407e51834568f7dccc6167e293` |
| Fylo | 26.33.02 `linux-arm64`, commit `e236162508f6789f980fa7ca136a6671938030cb`, checksum `ec161767b411b6a81d59771f43a094d4da22796c3c71301f2cc2a8f0a4db9d17` |

## Command

```sh
make linux-qualify-arm64
# Linux qualification: native arm64 runtime, IPC, filesystem, autostart,
# and AppDir smoke passed
```

The maintained harness uses a digest-pinned arm64 image, verifies that the
Docker server and guest are both arm64, installs the native Fyne build
dependencies, and runs the product tests as an unprivileged user. It verifies
the runtime artifact checksums and identities before executing them.

The following native suites passed with required-runtime skips disabled:

- runtime executable validation;
- Fylo handshake, exact version/capability matching, exclusive-root ownership,
  durable idempotency, queue lease recovery, close, and reopen;
- production Unix-domain socket permissions, stale/live endpoint handling,
  descriptor capability, bounded admission, and cleanup;
- the real `ty -> sluice-yon -> sluice-agent -> fylo` chain;
- the shared handle-relative Unix installer on both filesystems, including
  hostile parent replacement, permission denial, no-replace races, real
  partial-write ENOSPC cleanup, unsupported-rename error mapping, and forced
  pre-rename recovery;
- the XDG autostart contract; and
- two AppDir starts under isolated X11 and D-Bus, including authenticated
  health, stable token, native IPC, and coordinated process shutdown.

## Compatibility discovery

The same payload was first executed in the arm64 Debian Bookworm Go image. The
Tachyon executable failed before startup because it requires `GLIBC_2.39`,
while Bookworm provides glibc 2.36. Trixie glibc 2.41 satisfied the binary. The
Linux package baseline must therefore declare glibc 2.39 or newer unless a
future Tachyon artifact lowers that requirement.

## Honest limits

The isolated D-Bus session did not include
`org.kde.StatusNotifierWatcher` or `org.freedesktop.Notifications`. Fyne reached
those APIs but reported that the services were unavailable. Visual tray/menu
behavior, notification delivery, accessibility, keyboard behavior, and a real
desktop login/autostart session remain native manual gates.

Linux amd64 remains unqualified. Final AppImage construction, distro-specific
integration, installation, upgrade/rollback, signing, SBOM, and provenance
remain Phases 8 and 9 work.
