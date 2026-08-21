# Sluice Local API v1 Contracts

These Draft 2020-12 JSON Schemas are the normative JSON body contracts for the
loopback-only Yon API described in `docs/PROJECT_PLAN.md`. Every request is
authenticated, and every mutation is idempotent.

## HTTP contract

- Bind only to an operating-system loopback address on a random port.
- Send the per-install token only as `Authorization: Bearer <token>`. Tokens in
  URLs, logs, problem details, or diagnostics are invalid behavior.
- Send `Idempotency-Key` on every `POST`. A key identifies the semantic request,
  is scoped to the installation and endpoint, and is retained at least as long
  as the operation it created. Reusing a key with a different body returns
  `409` with problem code `idempotency_key_reused`.
- Return errors as `application/problem+json` conforming to
  `problem.schema.json`. Clients branch on `code` and `retryable`, never on
  human-readable text.
- Reject unknown properties and unsupported protocol versions. Collection
  responses are bounded and include page metadata.
- Use `Content-Type: application/json` for successful JSON bodies and reject
  request bodies larger than the endpoint's configured bound.

`request-metadata.schema.json` models the HTTP metadata asserted by black-box
contract tests. It is not an extra JSON envelope sent over the wire.

## Endpoint body schemas

| Endpoint | Schema |
| --- | --- |
| `GET /v1/health` | `health.schema.json` |
| `GET /v1/version` | `version.schema.json` |
| `GET /v1/status` | `status.schema.json` |
| `GET /v1/profiles` | `profiles.schema.json` |
| `POST /v1/profiles` | request: `profile-create.schema.json` |
| `POST /v1/profiles/{profileId}/probe` | response: `profile-probe.schema.json` |
| `GET /v1/roots` | `roots.schema.json` |
| `POST /v1/roots` | request: `root-create.schema.json` |
| root pause, resume, reconcile | response: `root-action.schema.json` |
| `GET /v1/operations` | `operations.schema.json` |
| `GET /v1/conflicts` | `conflicts.schema.json` |
| `POST /v1/conflicts/{conflictId}/resolve` | request: `conflict-resolution.schema.json` |
| `POST /v1/diagnostics/export` | response: `diagnostic-export.schema.json` |

POST responses that are not otherwise specified use the resource schema on a
synchronous `200`/`201`, or `root-action.schema.json` on an asynchronous `202`.
