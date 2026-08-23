# Sluice Design System

Status: original frontend baseline

Last reviewed: 2026-08-22

## Product character

Sluice should feel calm, inspectable, and recoverable. It is a control channel
for user-owned storage, not a branded clone of another cloud drive. The design
uses familiar operating-system conventions while keeping Sluice's wording,
composition, iconography, and visual identity original.

Core promises:

- users always know whether work is active, paused, offline, blocked, or
  conflicted;
- the safest next action is clear;
- hiding the window never changes synchronization behavior;
- errors describe what remains safe before asking for action; and
- conflicts emphasize preserved versions rather than a presumed winner.

## Shared Fyne foundations

- Use Fyne theme tokens rather than hard-coded platform-specific colors.
- Respect system light/dark mode, scale, reduced-motion expectations, and
  platform font rendering.
- Prefer semantic text, standard Fyne controls, and layouts that reflow before
  custom drawing.
- Keep shared view and interaction code in `ui/app`; platform adapters own only
  tray lifecycle, notifications, secret storage, filesystem integration, and
  other unavoidable OS behavior.
- Never import vendor icons, screenshots, sounds, fonts, names, or copied
  layouts. New Sluice assets require source and license provenance.

Fyne draws a cross-platform widget set; "native" in Sluice means native
application packaging, lifecycle, tray integration, and platform adapters, not
that every visible control is an OS toolkit control.

## Information architecture

The desktop window has seven eventual destinations, delivered in project-plan
order:

1. Status: global state, bounded progress, and safest next action.
2. Activity: queued, active, completed, failed, and conflicted operations.
3. Folders: roots, modes, pause state, and availability policy.
4. Conflicts: preserved versions and explicit resolution.
5. Storage: S3 profile, endpoint validation, and secret reference workflow.
6. Settings: login startup, bandwidth, ignore, notification, and update policy.
7. Diagnostics: redacted health, export preview, repair, and support evidence.

Only destinations backed by a versioned API and tested behavior are enabled.
An inactive feature is omitted or honestly labeled unavailable; it is not
represented by a dead control.

## Status composition

The shared status surface contains:

- a short textual heading from the frontend state machine;
- one sentence explaining safety and expected behavior;
- optional bounded aggregate progress;
- at most one primary recovery/control action; and
- a path to detail when the state requires inspection.

Tray content is intentionally smaller: state, aggregate progress when useful,
Open Sluice, Pause/Resume, Sync now when valid, Open root when unambiguous,
Review conflicts when present, and Quit. Window close maps to hide; only Quit
ends the persistent desktop lifecycle.

## Language

- Use "sync," "local folder," "storage," "version," and "conflict."
- Reserve "uploaded" and "downloaded" for verified transfer outcomes.
- Do not say "backed up" merely because a sync queue is empty.
- Explain what remains safe first: "Changes remain safe locally."
- Prefer actions such as "Retry now" and "Review conflicts" over generic "OK."
- Never expose raw provider errors as the only explanation; pair a stable,
  redacted message with optional diagnostic detail.

## State semantics

Color and iconography reinforce text but never replace it. The eventual icon
family should be simple, original, and recognizable at tray size:

| Semantic role | Visual direction | Required text equivalent |
| --- | --- | --- |
| Neutral/ready | calm channel or check motif | Up to date |
| Active | restrained directional motion | Syncing plus aggregate progress |
| Paused | pause motif | Syncing paused |
| Offline | disconnected channel motif | Offline |
| Attention | warning motif | Needs attention |
| Conflict | branching channel motif | Conflicts need review |

Do not copy another client's badge shapes, exact hues, animation timing, or
icon silhouettes. Motion stops in reduced-motion mode and never becomes the
only activity signal.

## Accessibility and privacy

- Every control has a stable accessible name and keyboard path.
- Reading and focus order follow visual order.
- Status survives 200% scaling and narrow layouts without clipping.
- Contrast is verified on every supported native target.
- Notifications contain no file contents, credentials, signed URLs, or
  sensitive absolute paths; file names are omitted by default.
- Background updates do not steal focus. Screen-reader announcements are
  deduplicated and rate-limited.
- Destructive actions state scope and consequence, require confirmation when
  recovery is not immediate, and offer undo where the domain permits it.

## Acceptance discipline

Each new view needs unit-level presentation tests, Fyne accessibility coverage,
and, when its Yon contract exists, real-process acceptance on native targets.
Screenshots can assist private research but do not replace accessibility,
keyboard, state, lifecycle, or recovery evidence and are not copied into the
public repository from vendor applications.
