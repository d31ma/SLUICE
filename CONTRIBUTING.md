# Contributing to Sluice

Thank you for improving Sluice. The project accepts documentation, design,
code, test, packaging, accessibility, security, and provider-qualification
contributions under the MIT License.

## Before contributing

Read:

1. `AGENTS.md` and the inherited Dropbox instructions;
2. `CONTEXT.md` and `CONTEXT-MAP.md`;
3. `docs/PROJECT_PLAN.md` and the active phase gate; and
4. relevant ADRs and public contracts.

Open a design discussion before changing persistent state, remote object
layout, process topology, security boundaries, public APIs, or support claims.
Accepted hard-to-reverse changes require an ADR.

## Development workflow

1. Create a focused branch from `main`.
2. Add a failing observable test for one behavior.
3. Implement the smallest production-quality vertical slice that passes it.
4. Run `make check`.
5. Update contracts, documentation, and the changelog when behavior changes.
6. Refresh Graphify project context.
7. Submit a pull request explaining behavior, evidence, risks, and rollback.

Do not commit generated binaries, local credentials, Fylo roots, S3 fixtures
containing secrets, or diagnostic bundles.

## Commit and review expectations

- Keep commits reviewable and behaviorally coherent.
- Preserve public compatibility unless the accepted change declares and
  migrates a breaking boundary.
- Use stable error codes at public boundaries.
- Never weaken conflict preservation, exclusive Fylo ownership, conditional
  remote writes, authentication, redaction, or path safety for convenience.
- Treat native support as an evidence claim, not a build claim.

## Certificates of origin

By contributing, you certify that you have the right to submit the work under
the project license. Sign commits with `git commit -s` to add the Developer
Certificate of Origin sign-off:

```text
Signed-off-by: Your Name <you@example.com>
```

The full certificate is available at <https://developercertificate.org/>.

## Reporting security issues

Do not open a public issue for a suspected vulnerability. Follow
`SECURITY.md`.
