# Security Policy

## Current status

Sluice is pre-release and has no supported versions. Security reports are still
welcome because architectural and dependency issues are cheapest to correct
before release.

## Reporting a vulnerability

Use GitHub private vulnerability reporting on the canonical Sluice repository.
Do not include credentials, private user files, production bucket contents, or
unredacted diagnostics unless a maintainer explicitly requests a secure
transfer method.

Include:

- affected version or commit;
- affected operating system and provider;
- reproduction steps or proof of concept;
- expected and observed impact;
- whether credentials or user content may have been exposed; and
- any known mitigation.

If private vulnerability reporting is unavailable, contact the maintainers
through the private channel published by the DELMA organization. Do not open a
public issue containing exploit details.

## Response targets

- Acknowledge a complete report within 3 business days.
- Provide an initial severity assessment within 7 business days.
- Coordinate disclosure timing with the reporter based on affected users and
  remediation readiness.

These are response targets, not guarantees. Active exploitation or credible
data-loss risk takes priority over scheduled feature work.

## Supported versions

No version is currently supported. Before desktop 1.0, this section will list
supported release lines and end-of-support dates.

## Security boundaries

The current threat model is `docs/THREAT_MODEL.md`. Sluice treats paths, object
keys, manifests, endpoint responses, local API requests, Fylo frames, and
filesystem events as untrusted. Credentials belong in native secret storage
and must never appear in logs or diagnostic bundles.
