# ADR 0007: Promote Targets Through Evidence-Based Support Tiers

- Status: Accepted
- Date: 2026-08-21

## Context

Sluice spans desktop, placeholder, and future mobile integrations whose build
and runtime evidence differ. A cross-compiled artifact does not prove native
filesystem, tray, accessibility, crash, upgrade, or lifecycle behavior.

## Decision

Use four published tiers: buildable, native-tested, preview, and supported.
Each operating system, placeholder adapter, provider profile, Android target,
and iOS target is promoted independently.

Supported requires clean install, upgrade, rollback, uninstall, crash recovery,
security, accessibility, performance, and soak evidence on named native
environments. Evidence is attached to a release and cannot be inferred from a
newer source build.

## Consequences

- Release notes state exactly what was executed and where.
- Mobile work does not block desktop 1.0.
- Placeholder integrations can remain preview while ordinary sync is
  supported.
- Native test infrastructure is a release dependency.

## Rejected alternatives

- Supported when it compiles: too weak for platform integration claims.
- One product-wide tier: hides different evidence across targets.
- Permanent beta label: avoids rather than defines release accountability.
