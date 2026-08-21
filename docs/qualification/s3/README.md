# S3 Provider Qualification

The executable provider contract is `TestProviderQualification` in
`internal/objectstore/s3/qualification_test.go`. It uses only the exported
Sluice object-store API and creates a random namespace beneath
`.sluice/v1/qualification/`. The test removes every object version it creates.

The contract covers:

- behavioral capability probing;
- paged `ListObjectsV2` with opaque, prefix-bound cursors;
- create-only and ETag compare-and-swap, including rejected stale writes;
- single and automatic multipart upload through AWS SDK v2 transfermanager;
- `HeadObject`, full download, and byte-range download;
- local SHA-256 calculation, provider checksum negotiation, metadata checksum
  round-trip, and full-download integrity verification;
- exact version-ID deletion when the bucket exposes versions; and
- aggregate and per-transfer concurrency bounds.

Unit-level HTTP black-box tests inject 409, 412, and 500 responses. A 409 or
412 conditional failure returns `Applied=false`; it is not silently retried.
A timeout, connection loss, or 5xx write result is marked ambiguous so the sync
engine can read back the object before deciding whether to retry.

## Provider variables

Do not put real credentials in a shell history, repository file, or diagnostic
bundle. Load them from the native secret adapter in the application. The test
harness accepts environment variables solely for an isolated qualification
run:

```text
SLUICE_S3_QUALIFY=1
SLUICE_S3_PROVIDER=<evidence label>
SLUICE_S3_ENDPOINT=<empty for AWS, URL for compatible provider>
SLUICE_S3_REGION=<region; defaults to us-east-1>
SLUICE_S3_BUCKET=<pre-created isolated bucket>
SLUICE_S3_ACCESS_KEY_ID=<optional; default AWS chain is used when empty>
SLUICE_S3_SECRET_ACCESS_KEY=<optional>
SLUICE_S3_SESSION_TOKEN=<optional>
SLUICE_S3_FORCE_PATH_STYLE=<true or false>
SLUICE_S3_REQUIRE_VERSIONING=<1 when this evidence requires versioning>
```

Run only the provider contract:

```sh
go test -v ./internal/objectstore/s3 -run '^TestProviderQualification$'
```

## Reproducible MinIO service

The local evidence pins the multi-platform image digest, not a mutable tag:

```sh
docker run -d --name sluice-s3-qualification \
  -p 127.0.0.1:19000:9000 \
  -e MINIO_ROOT_USER=<temporary-access-key> \
  -e MINIO_ROOT_PASSWORD=<temporary-secret-key> \
  minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e \
  server /data
```

Create a dedicated bucket and enable versioning with a trusted S3 client, run
the contract with endpoint `http://127.0.0.1:19000` and path-style addressing,
then stop and remove the named container. Bind the service to loopback only.

This image contains MinIO `RELEASE.2025-09-07T16-13-09Z`. It is suitable only
for the isolated compatibility fixture recorded here. The open-source MinIO
repository was archived in 2026 and its later security release did not publish
this container tag; do not infer production security approval from this test.

## Promotion rule

A provider is supported only when a dated evidence file records the endpoint
class, bucket settings, exact adapter and provider versions, capability output,
all contract results, cleanup result, and any deviation. Cross-provider claims
cannot be inferred from MinIO. The independent RustFS run is recorded in
`2026-08-21-rustfs.md`; the separate Garage attempt is recorded as incompatible
in `2026-08-21-garage.md`. AWS S3 still needs its own authorized run and
evidence.
