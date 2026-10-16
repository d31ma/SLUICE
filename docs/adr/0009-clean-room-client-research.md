# ADR 0009: Use Clean-Room Behavioral Research for Client UX

- Status: Accepted
- Date: 2026-08-22

## Context

Dropbox, OneDrive, Google Drive, and Proton Drive have mature desktop clients.
Their public behavior is useful product evidence for Sluice's tray lifecycle,
status model, offline behavior, conflict presentation, and filesystem
integration. Blind imitation would nevertheless create legal, licensing,
security, and product risks.

The proprietary client agreements impose material restrictions. Dropbox's
[terms](https://www.dropbox.com/terms) and the
[Microsoft Services Agreement](https://www.microsoft.com/en/servicesagreement/)
restrict reverse engineering except where applicable law overrides those
terms. Google's [terms](https://policies.google.com/terms) restrict copying,
modifying, distributing, selling, or leasing its software. Proton publishes
client source through the verified
[Proton Drive Apps organization](https://github.com/ProtonDriveApps), but the
[Windows client](https://github.com/ProtonDriveApps/windows-drive) is primarily
GPL-3.0 rather than MIT.

Sluice is MIT-licensed and needs an original, provider-neutral experience.
Behavioral research must therefore be distinguishable from decompilation,
private-protocol discovery, asset extraction, or source-code transplantation.

## Decision

Sluice adopts the procedure in
[`docs/research/CLIENT_STUDY_PROTOCOL.md`](../research/CLIENT_STUDY_PROTOCOL.md)
for competitive client research.

The permitted research surface is observable, user-facing behavior obtained
through official documentation and normally operated, official client builds
in isolated test environments. Researchers may record visible UI state,
accessibility output, documented filesystem state, public process lifecycle,
resource use, and behavior under synthetic offline, restart, and conflict
scenarios.

The following are outside the approved workstream:

- decompilation, disassembly, binary patching, or debugger-assisted discovery
  of proprietary implementation logic;
- bypassing TLS, certificate pinning, authentication, access controls, or
  other technical safeguards;
- extracting credentials, proprietary assets, strings, private APIs, or wire
  protocols;
- placing vendor installers, binaries, screenshots, icons, fonts, or other
  proprietary artifacts in the public repository; and
- copying vendor source, layouts, or distinctive trade dress into Sluice.

Each finding must identify its source, client version, operating system,
observation date, synthetic scenario, and confidence. Observations are reduced
to provider-neutral requirements in the behavior matrix and state-machine
specification before implementation. When feasible, someone other than the
observer implements from that reduced specification. Reviewers reject changes
whose provenance cannot be reconstructed.

Published open-source client code is not automatically reusable. Proton Drive
source may be studied under the license attached to each file, but GPL-derived
code may not enter Sluice's MIT codebase without a separate licensing decision
and superseding ADR. Reuse of permissively licensed components requires a
normal license and attribution review.

Any proposal to inspect client internals, undocumented protocols, or protected
artifacts requires prior project counsel review and a new ADR. This decision is
an engineering policy, not legal advice.

## Consequences

- Sluice can learn from mature interaction patterns while retaining an
  original design and auditable provenance.
- Behavioral findings inform requirements; they do not establish platform or
  provider support.
- Research takes more discipline and may omit useful implementation details
  that cannot be learned from the approved surface.
- Public repository history remains free of downloaded proprietary clients and
  vendor visual assets.
- GPL contamination and contractual risk become explicit review gates rather
  than implicit contributor judgment.

## Rejected alternatives

- Decompile all four clients: exceeds the approved research surface and creates
  avoidable contractual, security, and provenance risk.
- Copy Proton Drive implementation because its source is public: its principal
  client license is incompatible with unreviewed inclusion in an MIT project.
- Rely only on memory and visual resemblance: produces unverifiable
  requirements and encourages accidental copying.
- Avoid competitor research entirely: discards valuable public evidence about
  user expectations and failure modes.
