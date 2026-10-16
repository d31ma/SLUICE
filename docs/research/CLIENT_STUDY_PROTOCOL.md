# Clean-Room Client Study Protocol

Status: active research procedure

Last reviewed: 2026-08-22

## Purpose

Use public evidence from mature cloud-drive clients to discover user needs and
failure modes without copying proprietary implementation, protected assets, or
distinctive product expression. ADR 0009 is authoritative if this procedure
and the decision record differ.

## Approved evidence

- Current official help, support, accessibility, privacy, and developer
  documentation.
- Official installers obtained from the vendor and used through their normal
  user interface in a disposable test environment.
- Visible window, tray/menu-bar, notification, and file-manager behavior.
- Operating-system accessibility output exposed to an ordinary user.
- Documented sync markers and ordinary filesystem events caused by synthetic
  test files.
- Public process lifecycle, startup registration, CPU, memory, network totals,
  and recovery timing. Do not publish comparative benchmark results where an
  applicable agreement forbids publication.
- Public source whose repository and per-file license have been verified.

## Prohibited evidence and actions

- Decompiling, disassembling, modifying, unpacking, or patching proprietary
  binaries or libraries.
- Attaching a debugger to discover proprietary logic or memory contents.
- Intercepting or defeating TLS, certificate pinning, authentication, access
  controls, encryption, or rate limits.
- Discovering, replaying, or depending on undocumented private protocols.
- Extracting credentials, tokens, strings, icons, fonts, layouts, source maps,
  or other vendor assets.
- Testing with another person's account, shared production data, or data that
  is private, regulated, copyrighted, or operationally important.
- Committing installers, binaries, vendor screenshots, raw traces, vendor
  assets, or secrets to Sluice.

Stop and obtain project counsel review before expanding beyond the approved
surface. Contract and copyright exceptions vary by jurisdiction; this protocol
does not decide that law.

## Environment

Use one disposable virtual machine or dedicated test account per
client/operating-system pair. Before each study:

1. Download the official client directly from the vendor.
2. Record download URL, product version, installer SHA-256, operating system,
   locale, architecture, filesystem, and observation date in a private lab
   record. A hash is evidence; the installer itself stays outside the repo.
3. Use a project-controlled synthetic account with multi-factor authentication
   and the minimum required plan.
4. Create a unique synthetic data set containing small and large files, nested
   directories, Unicode and case variants, a deliberate simultaneous edit, a
   delete-vs-edit case, and no sensitive content.
5. Take a clean VM snapshot. Keep each vendor client isolated from every other
   vendor's synchronized root.
6. Disable unrelated backup, indexing, and sync tools that would invalidate
   observations.

## Scenario catalogue

Run the same numbered scenarios wherever the client exposes the capability:

| ID | Scenario | Record |
| --- | --- | --- |
| S01 | First launch and account connection | Steps, consent, defaults, recovery paths |
| S02 | Tray/menu-bar open, hide, restore, and quit | Menu actions, window lifetime, background lifetime |
| S03 | Idle and up-to-date state | Visible status, timestamps, activity affordance |
| S04 | Active upload and download | Progress granularity, cancellation/pause, queue ordering |
| S05 | Offline then reconnect | Status, queued work, retry and recovery behavior |
| S06 | Global pause and resume | Persistence across restart, queued-change behavior |
| S07 | Concurrent edit conflict | Version preservation, names, explanation, resolution |
| S08 | Permission, disk-full, and unavailable-root errors | Error hierarchy, safe state, recovery action |
| S09 | Client crash and operating-system restart | Auto-start, state recovery, duplicate work |
| S10 | Selective/offline availability | User terminology, disk-state indicators, scope controls |
| S11 | Accessibility and keyboard pass | Labels, focus order, announcements, contrast, scaling |
| S12 | Sign out and uninstall | Local-file outcome, warnings, retained state |

Do not infer success from animation or icon changes alone. Verify the resulting
ordinary file state and repeat any ambiguous scenario.

## Observation record

Every finding promoted into the public behavior matrix must include:

- stable observation ID;
- vendor, client version, operating system version, architecture, and locale;
- scenario ID and exact synthetic preconditions;
- observable result stated without inferred implementation details;
- official documentation URL when one exists;
- private evidence reference and SHA-256 when the evidence cannot be published;
- confidence: documented, repeated observation, or single observation;
- observer and review date; and
- provider-neutral Sluice implication, or `none`.

Use neutral language such as "the tray menu offers Pause" rather than "the
client calls an internal pause endpoint." Record absences as "not observed in
this scenario," never as proof that a feature does not exist.

## Clean handoff

The observer updates the behavior matrix with facts, then the frontend state
machine or design system with a provider-neutral requirement. Implementation
must cite that reduced requirement, not depend on raw vendor artifacts. When
feasible, observer and implementer are different contributors. If the same
contributor performs both roles, a second reviewer must verify that the change
can be explained entirely by public documentation and the reduced record.

The reviewer checks:

- no vendor code, assets, strings, or distinctive layout were copied;
- the proposed behavior works for Sluice's S3/Fylo/Yon architecture;
- accessibility, privacy, and failure states are specified;
- conflict behavior preserves every divergent version; and
- the change does not claim support or bypass the active phase gate.

## Source and license register

Terms and licenses change. Re-check them before each new study round.

| Product | Source/terms checked | Engineering boundary |
| --- | --- | --- |
| Dropbox | [Dropbox Terms](https://www.dropbox.com/terms) | Behavioral observation only; no reverse engineering or asset extraction |
| OneDrive | [Microsoft Services Agreement](https://www.microsoft.com/en/servicesagreement/) | Behavioral observation only; no reverse engineering or protected-interface circumvention |
| Google Drive | [Google Terms](https://policies.google.com/terms) | Behavioral observation only; no copying or modifying client software |
| Proton Drive | [Verified source organization](https://github.com/ProtonDriveApps) and [Windows client license](https://github.com/ProtonDriveApps/windows-drive) | Public source study is license-specific; no GPL-derived code in MIT Sluice without a new decision |
