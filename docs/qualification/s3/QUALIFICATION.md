# S3 Qualification Evidence - 2026-08-21

## Status

| Provider | Contract status | Capability evidence | Promotion status |
| --- | --- | --- | --- |
| MinIO | Pass on local isolated service | Conditional writes, ranges, multipart, versioning, native SHA-256 all true | Phase 1 compatibility evidence only |
| RustFS 1.0.0-beta.9 | Pass on local isolated service | Conditional writes, ranges, multipart, versioning, native SHA-256 all true | Independent third-provider evidence complete |
| Garage v2.3.0 | Fail on local isolated service | Conditional writes and versioning false | Incompatible with the Sluice remote protocol |
| AWS S3 | Not run: no authorized bucket or credentials were available | None | Not qualified |

The independent third-provider portion now passes. The Phase 1 three-provider
gate remains open only for an authorized AWS S3 run. No external credentials
were invented and no external buckets were provisioned.

## MinIO run

- Host: Darwin 25.5.0, arm64.
- Go toolchain: `go1.27.0 darwin/amd64`.
- Docker client/server: 29.7.2.
- Container platform: Linux arm64.
- MinIO release: `RELEASE.2025-09-07T16-13-09Z`.
- Image: `minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e`.
- Endpoint: loopback HTTP on an ephemeral test port.
- Bucket: isolated test bucket with versioning enabled.
- AWS SDK: v2 S3 `v1.107.3`; transfermanager `v0.3.15`.

Observed capability output:

```text
provider=minio capabilities={ConditionalWrites:true RangeDownloads:true MultipartUploads:true ObjectVersioning:true ChecksumSHA256:true}
```

Observed contract result:

```text
=== RUN   TestProviderQualification
--- PASS: TestProviderQualification (0.28s)
PASS
ok   github.com/d31ma/sluice/internal/objectstore/s3 0.975s
```

The provider version listing was empty after the run, and the named container
was stopped and removed. No persistent Docker volume was attached.

## Independent-provider runs

RustFS passed all five capability probes and the complete contract against a
pinned beta.9 image. Garage was also evaluated, but failed closed before the
main contract because it lacks required conditional writes. See the dated
[RustFS pass](2026-08-21-rustfs.md) and
[Garage incompatibility](2026-08-21-garage.md) records.

## Deterministic adapter checks

The local test suite additionally proves behavior that a cooperative provider
cannot reliably trigger on demand:

- conditional 412 and 409 map to `Applied=false`;
- HTTP 500 on a write is retryable and ambiguous;
- malformed or cross-prefix cursors fail before any provider request;
- a corrupt full download produces `integrity_failure`;
- version IDs are preserved through `Head` and `Delete`; and
- eight simultaneous callers produce at most the configured two in-flight
  provider requests.

Commands and results:

```text
go test ./internal/objectstore/s3
ok   github.com/d31ma/sluice/internal/objectstore/s3 1.007s

go test -race ./internal/objectstore/s3
ok   github.com/d31ma/sluice/internal/objectstore/s3 2.530s

go tool golangci-lint run ./internal/objectstore/s3/...
0 issues.
```

## Native and credential gaps

- AWS requires an explicitly authorized, isolated bucket. The evidence run
  should enable versioning and use least-privilege credentials limited to the
  qualification prefix and required bucket metadata calls.
- Native provider checksum absence is recorded by `ChecksumSHA256=false`; the
  adapter still stores a Sluice SHA-256 metadata value and verifies every full
  download. A profile must not claim native checksum support from that fallback.
- S3 credentials remain outside this package's persistence model. Production
  wiring must obtain them from the platform secret adapter and pass an in-memory
  SDK configuration.
