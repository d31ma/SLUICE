# ADR 0001: Use Fyne for the Shared Client

- Status: Accepted
- Date: 2026-08-21

## Context

Sluice needs one frontend codebase for Windows, macOS, and Linux, with future
Android and iOS delivery. Desktop releases must provide system-tray lifecycle,
native packaging, notifications, and a responsive status and conflict UI. The
primary implementation language is Go.

## Decision

Use Fyne v2 for shared view and interaction code. Desktop-specific tray,
autostart, notification, and secret-store behavior remains behind platform
interfaces. Mobile storage and lifecycle behavior is explicitly platform
specific even when views are shared.

Phase 1 must prove tray lifecycle, accessibility, native packaging, and clean
shutdown on all three desktop targets before the choice is considered
qualified for production work.

Fyne 2.8 keeps its accessibility bridge behind the experimental
`accessibility` build tag. Every Sluice desktop package builder enables that
tag. Enabling the tag is build configuration, not qualification: screen-reader
and keyboard behavior must still pass on each named native target before the
Fyne decision clears Phase 1.

## Consequences

- The client and sync engine use one primary language.
- No browser runtime is required for the main application.
- Fyne draws a cross-platform widget set rather than using every platform's
  native controls.
- The current accessibility implementation is experimental upstream and may
  require an upgrade or a superseding ADR if native qualification fails.
- UI quality and accessibility require native evidence rather than assumptions.
- Replacing Fyne requires a superseding ADR and migration plan.

## Rejected alternatives

- Flutter: broad platform support, but introduces Dart and depends on a
  third-party desktop tray package.
- Compose Multiplatform: first-party tray support, but introduces Kotlin and a
  bundled desktop JVM runtime.
- Tauri: good tray support, but uses webview UI and a Rust shell.
- Tachyon Tac: strategically aligned but not yet a supported native release and
  lacks a qualified tray contract.
