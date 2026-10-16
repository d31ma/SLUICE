# Frontend Status State Machine

Status: executable presentation contract; agent/Yon mapping pending

Last reviewed: 2026-08-22

This is the shared, vendor-neutral projection used by the Fyne window and
future tray/mobile views. It is not a Fylo collection state, durable operation
state, or Yon schema. The agent remains authoritative; a later versioned Yon
contract will map domain facts into this bounded snapshot.

## Snapshot

The frontend accepts only:

- `state`: one state from the table below;
- `pendingItems`: a non-negative aggregate queue count; and
- `conflictCount`: a non-negative aggregate conflict count.

It does not accept file names, object keys, credentials, endpoints, signed
URLs, or absolute paths. Detailed activity belongs in an explicitly opened,
redacted view.

## States and actions

| State | Meaning | Primary action intent | Window/tray promise |
| --- | --- | --- | --- |
| `setup` | No usable profile/root is configured | `configure` | Explain that user-controlled S3 configuration is required |
| `connecting` | Local service or configured storage is being checked | none | Never imply files are synchronized yet |
| `up_to_date` | The agent reports no outstanding selected-file work | none | Say "Up to date," not "backed up" |
| `syncing` | Selected-file work is active or queued | `pause` | Show bounded aggregate progress |
| `paused` | The user or policy has stopped new work | `resume` | Explain that changes remain queued |
| `offline` | Required connectivity is unavailable | `retry` | Explain that local changes remain safe and retry is expected |
| `attention` | A recoverable problem needs inspection | `view_details` | Do not collapse the problem into "offline" or "conflict" |
| `conflicts` | One or more divergent versions need a decision | `review_conflicts` | State that divergent versions are preserved |

Action intents are presentation metadata only. The UI must not enable a button
until the corresponding authenticated Yon command exists and its failure
behavior is tested.

## Allowed adjacent projections

The executable contract permits repeated states and these directed changes:

```text
setup       -> connecting
connecting  -> syncing | up_to_date | offline | attention
up_to_date  -> syncing | paused | offline | attention | conflicts
syncing     -> up_to_date | paused | offline | attention | conflicts
paused      -> connecting | offline
offline     -> connecting | paused
attention   -> connecting | paused | conflicts
conflicts   -> syncing | up_to_date | paused
```

The transition check detects incoherent frontend fixtures; it must not reject
or mutate authoritative agent state. A new real-world transition first changes
this document and its tests, then the projection code.

## Precedence for aggregate state

The future Yon mapper should use this safety-first precedence when several
facts coexist:

1. `setup` when configuration is unusable;
2. `conflicts` when unresolved divergent versions exist;
3. `attention` for a non-conflict error requiring user action;
4. `paused` when work is intentionally suspended;
5. `offline` when connectivity blocks progress;
6. `connecting` while establishing the local/runtime/provider boundary;
7. `syncing` while work is active or queued; and
8. `up_to_date` only when none of the above applies.

This precedence is a design input, not authorization to implement the Yon
mapping before its Phase 5 vertical slice.

## Accessibility contract

- State is always expressed as text, never color or animation alone.
- Status heading and explanation are exposed to Fyne accessibility output.
- Counts use readable singular/plural copy and never expose sensitive paths.
- Focus must not move merely because a background state changes.
- Announcements are rate-limited and reserved for meaningful state changes.
- Conflict and destructive-resolution actions require explicit labels and
  never preselect a timestamp-based winner.

`ui/app/status_test.go` is the executable acceptance surface for the current
pure presentation slice. Phase 5 adds live Yon-driven acceptance tests rather
than replacing them with in-process mocks.
