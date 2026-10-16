//go:build !android && !ios

package appui_test

import (
	"testing"

	appui "github.com/d31ma/sluice/ui/app"
)

func TestPresentStatusDefinesEveryFrontendState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		heading     string
		detail      string
		action      appui.StatusAction
		actionLabel string
		state       appui.SyncState
		pending     int
		conflicts   int
	}{
		{
			name:        "setup",
			heading:     "Ready for configuration",
			detail:      "Connect an S3-compatible profile and choose a local folder to begin syncing.",
			action:      appui.StatusActionConfigure,
			actionLabel: "Configure storage",
			state:       appui.SyncStateSetup,
		},
		{
			name:    "connecting",
			heading: "Connecting",
			detail:  "Starting the local service and checking storage.",
			state:   appui.SyncStateConnecting,
		},
		{
			name:    "up to date",
			heading: "Up to date",
			detail:  "All selected files are synchronized.",
			state:   appui.SyncStateUpToDate,
		},
		{
			name:        "syncing with queue depth",
			heading:     "Syncing",
			detail:      "Synchronizing 3 queued items.",
			action:      appui.StatusActionPause,
			actionLabel: "Pause syncing",
			state:       appui.SyncStateSyncing,
			pending:     3,
		},
		{
			name:        "paused",
			heading:     "Syncing paused",
			detail:      "Changes remain queued until syncing resumes.",
			action:      appui.StatusActionResume,
			actionLabel: "Resume syncing",
			state:       appui.SyncStatePaused,
		},
		{
			name:        "offline",
			heading:     "Offline",
			detail:      "Changes remain safe locally; Sluice will retry when connectivity returns.",
			action:      appui.StatusActionRetry,
			actionLabel: "Retry now",
			state:       appui.SyncStateOffline,
		},
		{
			name:        "attention",
			heading:     "Needs attention",
			detail:      "A synchronization problem needs review before Sluice can continue.",
			action:      appui.StatusActionViewDetails,
			actionLabel: "View details",
			state:       appui.SyncStateAttention,
		},
		{
			name:        "conflicts preserve versions",
			heading:     "Conflicts need review",
			detail:      "2 divergent versions are preserved for explicit review.",
			action:      appui.StatusActionReviewConflicts,
			actionLabel: "Review conflicts",
			state:       appui.SyncStateConflicts,
			conflicts:   2,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			presentation, err := appui.PresentStatus(appui.StatusSnapshot{
				State:         testCase.state,
				PendingItems:  testCase.pending,
				ConflictCount: testCase.conflicts,
			})
			if err != nil {
				t.Fatalf("PresentStatus() error = %v", err)
			}
			if presentation.Heading != testCase.heading {
				t.Errorf("heading = %q, want %q", presentation.Heading, testCase.heading)
			}
			if presentation.Detail != testCase.detail {
				t.Errorf("detail = %q, want %q", presentation.Detail, testCase.detail)
			}
			if presentation.PrimaryAction != testCase.action {
				t.Errorf("primary action = %q, want %q", presentation.PrimaryAction, testCase.action)
			}
			if presentation.PrimaryActionLabel != testCase.actionLabel {
				t.Errorf("primary action label = %q, want %q", presentation.PrimaryActionLabel, testCase.actionLabel)
			}
			if presentation.TrayStatus != testCase.heading {
				t.Errorf("tray status = %q, want %q", presentation.TrayStatus, testCase.heading)
			}
		})
	}
}

func TestPresentStatusRejectsUntrustedSnapshots(t *testing.T) {
	t.Parallel()

	for _, snapshot := range []appui.StatusSnapshot{
		{State: appui.SyncState("unknown")},
		{State: appui.SyncStateSyncing, PendingItems: -1},
		{State: appui.SyncStateConflicts, ConflictCount: -1},
	} {
		if _, err := appui.PresentStatus(snapshot); err == nil {
			t.Errorf("PresentStatus(%+v) error = nil, want rejection", snapshot)
		}
	}
}

func TestCanTransitionStatusConstrainsTheFrontendProjection(t *testing.T) {
	t.Parallel()

	for _, transition := range []struct {
		from appui.SyncState
		to   appui.SyncState
		want bool
	}{
		{from: appui.SyncStateSetup, to: appui.SyncStateConnecting, want: true},
		{from: appui.SyncStateSetup, to: appui.SyncStateSetup, want: true},
		{from: appui.SyncStateConnecting, to: appui.SyncStateSyncing, want: true},
		{from: appui.SyncStateUpToDate, to: appui.SyncStatePaused, want: true},
		{from: appui.SyncStateSyncing, to: appui.SyncStateConflicts, want: true},
		{from: appui.SyncStateConflicts, to: appui.SyncStateUpToDate, want: true},
		{from: appui.SyncStatePaused, to: appui.SyncStateConnecting, want: true},
		{from: appui.SyncStateOffline, to: appui.SyncStateConnecting, want: true},
		{from: appui.SyncStateAttention, to: appui.SyncStateConflicts, want: true},
		{from: appui.SyncStateSetup, to: appui.SyncStateUpToDate, want: false},
		{from: appui.SyncStateConflicts, to: appui.SyncStateSetup, want: false},
		{from: appui.SyncState("unknown"), to: appui.SyncStateSetup, want: false},
	} {
		if got := appui.CanTransitionStatus(transition.from, transition.to); got != transition.want {
			t.Errorf("CanTransitionStatus(%q, %q) = %t, want %t", transition.from, transition.to, got, transition.want)
		}
	}
}

func TestNewStatusPanelExposesStateCopyToAccessibility(t *testing.T) {
	t.Parallel()

	panel, err := appui.NewStatusPanel(appui.StatusSnapshot{
		State:         appui.SyncStateConflicts,
		ConflictCount: 2,
	})
	if err != nil {
		t.Fatalf("NewStatusPanel() error = %v", err)
	}
	labels := accessibleLabels(panel)
	for _, want := range []string{
		"Status",
		"Local agent",
		"Conflicts need review",
		"2 divergent versions are preserved for explicit review.",
	} {
		if !labels[want] {
			t.Errorf("native accessibility labels do not contain %q", want)
		}
	}
}
