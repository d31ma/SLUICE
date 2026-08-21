# Support

## Current tier

Sluice is in Phase 1 feasibility and executable-contract development. There
are no preview or supported releases yet.

## Evidence vocabulary

- **Buildable**: an artifact compiles for a target.
- **Native-tested**: the artifact executes and passes its named suite on that
  target.
- **Preview**: the artifact is distributed for evaluation with documented
  limitations and recovery guidance.
- **Supported**: installation, upgrade, rollback, crash recovery, security,
  accessibility, performance, and soak gates pass.

Cross-compilation is never evidence of native support. Each operating system,
placeholder adapter, and mobile target is promoted independently.

## Getting help

Use the canonical GitHub repository for questions, reproducible bug reports,
and feature requests. Before filing a bug, run the documented doctor command
when it becomes available and remove secrets and sensitive paths from output.

Security issues must follow `SECURITY.md` rather than public support channels.
