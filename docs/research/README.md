# Client Research

This directory turns public documentation and controlled observations of
Dropbox, OneDrive, Google Drive, and Proton Drive into original,
provider-neutral Sluice frontend requirements.

Start with:

1. [Client study protocol](CLIENT_STUDY_PROTOCOL.md) for the permitted research
   surface, environment, and evidence schema.
2. [Behavior matrix](CLIENT_BEHAVIOR_MATRIX.md) for sourced findings and Sluice
   decisions.
3. [Frontend state machine](FRONTEND_STATE_MACHINE.md) for shared UI states,
   actions, and transitions.
4. [Sluice design system](SLUICE_DESIGN_SYSTEM.md) for an original Fyne visual
   and interaction language.

ADR 0009 governs this work. Research artifacts are product input, not evidence
that an operating system, storage provider, or release tier is supported.
Downloaded clients, vendor assets, raw screenshots, credentials, packet
captures, and production user data do not belong in this repository.

The first executable projection of the state-machine contract lives in
`ui/app/status.go`; its tests verify copy, action intents, validation,
transitions, conflict preservation, and accessible Fyne rendering.
