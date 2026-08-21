# Runtime Bundle Manifest

`manifest.json` is the machine-checked source of truth for the Tachyon `ty` and
Fylo executables shipped with Sluice. It records exact release assets, SHA-256
digests, commit-pinned source and license URLs, the payload-relative install
layout, and the qualification state of every published desktop target.

The matrix records artifact availability, not installation support. A target
with `not-native-tested` has not been installed or executed on that operating
system. Cross-building or successfully downloading its assets must not change
that value.

Run the offline contract checks with:

```sh
make runtime-contract
```

On a native Apple Silicon host, download, verify, stage, and execute the macOS
arm64 pair inside an isolated temporary directory with:

```sh
make runtime-stage-macos
```

The staging test never writes to `/Applications`, `/Library`, `/usr`, `/opt`,
or another installation path. A future installer consumes the same relative
layout beneath its own package root only after target-specific native evidence
exists.
