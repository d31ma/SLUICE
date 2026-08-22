# Versioning and Compatibility

## Application version

Sluice application releases use Semantic Versioning:

- major: incompatible user-facing or application API change;
- minor: backward-compatible feature addition; and
- patch: backward-compatible fix or hardening.

Pre-1.0 releases may change incomplete contracts, but every persistent or
remote change still requires an explicit migration and rollback boundary.

`VERSION` contains the source release version. Release tags use `v<version>`.

## Independently versioned contracts

| Contract | Initial version | Compatibility rule |
| --- | --- | --- |
| Local Yon HTTP API | `1` | Additive fields/endpoints within v1; incompatible behavior uses v2. |
| Agent IPC | `1` | Handshake negotiation; unknown major fails closed. |
| Fylo state schema | `1` | Forward migration with tested backup and rollback boundary. |
| Remote S3 protocol | `1` | Unknown version fails closed; migration never rewrites the only content copy. |
| Diagnostic bundle | `1` | Additive redacted records; readers ignore unknown safe fields. |

Application versions do not imply these contract versions. Artifacts and
diagnostics report all of them explicitly.

## Dependency compatibility

- Go toolchain is pinned by `go.mod` and CI.
- Tachyon and Fylo releases are pinned in the release manifest and verified by
  checksum/provenance.
- The Fylo binary and production protocol integration must match the qualified
  release identity. ADR 0008 selects the focused supervisor until a matching
  released Go shim exposes and passes the exclusive-root contract.
- S3 provider profiles record qualification capability and date.
- Unknown Tachyon, Fylo, or provider capability versions fail closed before
  external side effects.

## State migration

Every Fylo schema migration must provide:

1. source and destination versions;
2. preflight and free-space requirements;
3. durable progress or atomic replacement;
4. crash recovery at every phase;
5. post-migration verification;
6. backup and rollback behavior; and
7. release evidence from every supported prior version.

## Remote migration

Remote protocol migration is copy/verify/switch before delete. A client never
mutates an unknown namespace, silently downgrades a manifest, or garbage-
collects the old protocol until retained clients and tombstones no longer
reference it.

## Support policy

Support attaches to an exact artifact digest, target, and evidence report.
Cross-compilation is buildability only. Upgrade and rollback are required from
every supported release line before a new version is promoted.
