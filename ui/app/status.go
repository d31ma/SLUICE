package appui

import (
	"errors"
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SyncState is the frontend's vendor-neutral projection of agent status. It is
// not a durable domain state or a Yon wire value.
type SyncState string

const (
	SyncStateSetup      SyncState = "setup"
	SyncStateConnecting SyncState = "connecting"
	SyncStateUpToDate   SyncState = "up_to_date"
	SyncStateSyncing    SyncState = "syncing"
	SyncStatePaused     SyncState = "paused"
	SyncStateOffline    SyncState = "offline"
	SyncStateAttention  SyncState = "attention"
	SyncStateConflicts  SyncState = "conflicts"
)

// StatusAction identifies an intent that the frontend may send through Yon.
// Wiring these intents to the local API remains a later vertical slice.
type StatusAction string

const (
	StatusActionNone            StatusAction = ""
	StatusActionConfigure       StatusAction = "configure"
	StatusActionPause           StatusAction = "pause"
	StatusActionResume          StatusAction = "resume"
	StatusActionRetry           StatusAction = "retry"
	StatusActionViewDetails     StatusAction = "view_details"
	StatusActionReviewConflicts StatusAction = "review_conflicts"
)

// StatusSnapshot contains the bounded, non-sensitive facts needed to render
// global status. It deliberately carries no file names or absolute paths.
type StatusSnapshot struct {
	State         SyncState
	PendingItems  int
	ConflictCount int
}

// StatusPresentation is shared copy and action metadata for the window, tray,
// and future mobile views.
type StatusPresentation struct {
	Heading            string
	Detail             string
	PrimaryAction      StatusAction
	PrimaryActionLabel string
	TrayStatus         string
}

// PresentStatus validates an agent snapshot and translates it into the shared
// frontend presentation contract.
func PresentStatus(snapshot StatusSnapshot) (StatusPresentation, error) {
	if snapshot.PendingItems < 0 {
		return StatusPresentation{}, errors.New("pending item count must not be negative")
	}
	if snapshot.ConflictCount < 0 {
		return StatusPresentation{}, errors.New("conflict count must not be negative")
	}

	var presentation StatusPresentation
	switch snapshot.State {
	case SyncStateSetup:
		presentation = StatusPresentation{
			Heading:            "Ready for configuration",
			Detail:             "Connect an S3-compatible profile and choose a local folder to begin syncing.",
			PrimaryAction:      StatusActionConfigure,
			PrimaryActionLabel: "Configure storage",
		}
	case SyncStateConnecting:
		presentation = StatusPresentation{
			Heading: "Connecting",
			Detail:  "Starting the local service and checking storage.",
		}
	case SyncStateUpToDate:
		presentation = StatusPresentation{
			Heading: "Up to date",
			Detail:  "All selected files are synchronized.",
		}
	case SyncStateSyncing:
		presentation = StatusPresentation{
			Heading:            "Syncing",
			Detail:             syncingDetail(snapshot.PendingItems),
			PrimaryAction:      StatusActionPause,
			PrimaryActionLabel: "Pause syncing",
		}
	case SyncStatePaused:
		presentation = StatusPresentation{
			Heading:            "Syncing paused",
			Detail:             "Changes remain queued until syncing resumes.",
			PrimaryAction:      StatusActionResume,
			PrimaryActionLabel: "Resume syncing",
		}
	case SyncStateOffline:
		presentation = StatusPresentation{
			Heading:            "Offline",
			Detail:             "Changes remain safe locally; Sluice will retry when connectivity returns.",
			PrimaryAction:      StatusActionRetry,
			PrimaryActionLabel: "Retry now",
		}
	case SyncStateAttention:
		presentation = StatusPresentation{
			Heading:            "Needs attention",
			Detail:             "A synchronization problem needs review before Sluice can continue.",
			PrimaryAction:      StatusActionViewDetails,
			PrimaryActionLabel: "View details",
		}
	case SyncStateConflicts:
		presentation = StatusPresentation{
			Heading:            "Conflicts need review",
			Detail:             conflictDetail(snapshot.ConflictCount),
			PrimaryAction:      StatusActionReviewConflicts,
			PrimaryActionLabel: "Review conflicts",
		}
	default:
		return StatusPresentation{}, fmt.Errorf("unknown sync state %q", snapshot.State)
	}
	presentation.TrayStatus = presentation.Heading
	return presentation, nil
}

// CanTransitionStatus reports whether two adjacent frontend projections are
// coherent. The agent remains authoritative and may emit the same state again.
func CanTransitionStatus(from, to SyncState) bool {
	if !knownSyncState(from) || !knownSyncState(to) {
		return false
	}
	if from == to {
		return true
	}
	return slices.Contains(allowedTransitionTargets(from), to)
}

func allowedTransitionTargets(from SyncState) []SyncState {
	switch from {
	case SyncStateSetup:
		return []SyncState{SyncStateConnecting}
	case SyncStateConnecting:
		return []SyncState{SyncStateSyncing, SyncStateUpToDate, SyncStateOffline, SyncStateAttention}
	case SyncStateUpToDate:
		return []SyncState{
			SyncStateSyncing,
			SyncStatePaused,
			SyncStateOffline,
			SyncStateAttention,
			SyncStateConflicts,
		}
	case SyncStateSyncing:
		return []SyncState{
			SyncStateUpToDate,
			SyncStatePaused,
			SyncStateOffline,
			SyncStateAttention,
			SyncStateConflicts,
		}
	case SyncStatePaused:
		return []SyncState{SyncStateConnecting, SyncStateOffline}
	case SyncStateOffline:
		return []SyncState{SyncStateConnecting, SyncStatePaused}
	case SyncStateAttention:
		return []SyncState{SyncStateConnecting, SyncStatePaused, SyncStateConflicts}
	case SyncStateConflicts:
		return []SyncState{SyncStateSyncing, SyncStateUpToDate, SyncStatePaused}
	default:
		return nil
	}
}

// NewStatusPanel renders the read-only Fyne status surface. Action dispatch is
// intentionally kept out until the versioned Yon command contract exists.
func NewStatusPanel(snapshot StatusSnapshot) (fyne.CanvasObject, error) {
	presentation, err := PresentStatus(snapshot)
	if err != nil {
		return nil, err
	}
	return newStatusPanel(presentation), nil
}

func newStatusPanel(presentation StatusPresentation) fyne.CanvasObject {
	status := widget.NewLabelWithStyle(
		presentation.Heading,
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	description := widget.NewLabel(presentation.Detail)
	description.Wrapping = fyne.TextWrapWord
	return newAccessibleCard("Status", "Local agent", container.NewVBox(status, description))
}

func knownSyncState(state SyncState) bool {
	switch state {
	case SyncStateSetup, SyncStateConnecting, SyncStateUpToDate, SyncStateSyncing,
		SyncStatePaused, SyncStateOffline, SyncStateAttention, SyncStateConflicts:
		return true
	default:
		return false
	}
}

func syncingDetail(count int) string {
	switch count {
	case 0:
		return "Synchronizing selected files."
	case 1:
		return "Synchronizing 1 queued item."
	default:
		return fmt.Sprintf("Synchronizing %d queued items.", count)
	}
}

func conflictDetail(count int) string {
	switch count {
	case 0:
		return "Divergent versions are preserved for explicit review."
	case 1:
		return "1 divergent version is preserved for explicit review."
	default:
		return fmt.Sprintf("%d divergent versions are preserved for explicit review.", count)
	}
}
