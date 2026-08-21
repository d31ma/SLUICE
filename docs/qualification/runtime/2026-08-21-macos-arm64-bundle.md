# Runtime Bundle Staging: macOS arm64, 2026-08-21

## Result

The Tachyon and Fylo runtime payload passed the isolated native staging test
on the named macOS arm64 host. This proves that the exact checksummed assets
can be downloaded, placed in the declared application-bundle layout, and
executed from an unprivileged temporary directory. It is not an installer,
signing, notarization, upgrade, rollback, Windows, Linux, Preview, or Supported
claim.

Evidence was captured at `2026-08-21T22:51:19Z`.

| Item | Evidence |
| --- | --- |
| Host | macOS 26.5.2, build 25F84, Darwin 25.5.0, arm64 kernel |
| Tachyon asset | `ty-macos-arm64` from release `v26.33.02` |
| Tachyon SHA-256 | `08b90a516ccb665664c3b986e71769cdcc738960e45a0bd8c73ab691a0cda1cd` |
| Fylo asset | `fylo-macos-arm64` from release `v26.33.02` |
| Fylo SHA-256 | `2c78f91130863ad797cf7a6062aa1245fa638857fee63b8671fa9e2748974fa3` |
| Fylo source commit | `e236162508f6789f980fa7ca136a6671938030cb` |
| Tachyon source commit | `f96c1589ee33b047044f220448268d32ad86e58f` |

## Command and observations

```sh
make runtime-stage-macos
# runtime bundle contract: 5 targets, 10 checksummed artifacts
# runtime staging: verified tachyon ty-macos-arm64
# runtime staging: verified fylo fylo-macos-arm64
# runtime staging: macos-arm64 payload passed in an isolated temporary root
# runtime staging test: passed without modifying system paths
```

The test created its root with `mktemp`, staged executables beneath
`Sluice.app/Contents/Resources/runtime/bin`, and copied the Tachyon and Fylo
license materials beneath `Sluice.app/Contents/Resources/licenses`. It verified
each download before installation, required Mach-O arm64 executables, observed
Tachyon version `26.33.02`, and required Fylo runtime version `26.33.02`, source
commit `e236162508f6789f980fa7ca136a6671938030cb`, and build target
`macos-arm64`. The temporary root was removed automatically. No system path was
modified.

## Remaining gates

- A real macOS application package must still be signed, notarized, installed,
  upgraded, rolled back, and run through the installed-runtime suite.
- Tachyon's CycloneDX SBOM and Fylo's SPDX SBOM must be included in final
  dependency-license and provenance review.
- Windows and Linux require independent native staging and installation runs;
  their manifest entries remain `not-native-tested`.
