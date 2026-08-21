# ADR 0005: Store Credentials in Native Secret Facilities

- Status: Accepted
- Date: 2026-08-21

## Context

S3 credentials and local API authentication tokens grant access to user data.
Plain configuration files, Fylo documents, command lines, URLs, logs, and
diagnostic bundles are inappropriate storage or transport surfaces.

## Decision

Store desktop secrets in macOS Keychain, Windows Credential Manager, and Linux
Secret Service. Fylo profile records contain opaque secret references only.
Headless Linux may use an explicitly configured environment or permission-
checked file fallback; this is never selected silently.

The local Yon API uses a random token, binds only to loopback, accepts the token
in an authorization header, and compares it in constant time. Tokens never
appear in URLs or logs.

## Consequences

- Secret-store adapters require native integration tests.
- Profiles can be exported without credentials.
- Moving a Fylo root does not move account authority.
- Headless operation needs documented credential provisioning.
- Diagnostic redaction is enforced by construction and tests.

## Rejected alternatives

- Store credentials in Fylo: violates the intended data boundary.
- Plain `.env` as the desktop default: too easy to disclose or mis-permission.
- Unauthenticated loopback API: other local processes can issue commands.
