# Governance

## Model

Sluice uses a maintainer-led, evidence-driven governance model. Maintainers are
responsible for product coherence, data safety, security response, releases,
and the accuracy of support claims.

## Roles

### Contributors

Anyone who participates constructively through issues, design, documentation,
code, testing, review, or operations evidence.

### Maintainers

Contributors with sustained project judgment and responsibility. Maintainers
may merge changes, accept ADRs, qualify releases, and handle private security
reports.

The DELMA GitHub organization (`@d31ma`) is the initial maintainer and code
owner. Delegated ownership is recorded in `.github/CODEOWNERS` and reviewed as
part of every maintainer change.

### Release managers

Maintainers explicitly assigned to a release. They verify the complete release
evidence, publish artifacts, monitor rollout, and own rollback decisions.

## Decisions

- Reversible implementation choices are decided through ordinary review.
- Persistent formats, public protocols, process topology, security boundaries,
  and support policies require an ADR.
- Maintainers seek rough consensus. When consensus is not possible, the
  designated maintainer records the decision and dissent in the ADR.
- No decision may waive conflict preservation, durability, credential safety,
  or evidence-based support claims.

## Maintainer changes

Existing maintainers appoint new maintainers based on sustained contributions,
review quality, operational judgment, and conduct. Maintainer removal requires
documented cause or prolonged inactivity and must preserve security-reporting
continuity.

## Project contacts

Until project-specific addresses are published, use the private security and
maintainer contact mechanisms on the canonical GitHub repository. Phase 0 is
not complete for public release until `SECURITY.md` names a tested private
reporting channel.
