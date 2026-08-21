package fylo_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	fylostate "github.com/d31ma/sluice/internal/state/fylo"
)

const qualifiedFyloVersion = "26.33.02"

func TestInstalledFyloHandshakeExclusiveOwnershipAndCleanRestart(t *testing.T) {
	binary := installedRuntime(t, "fylo")
	root := filepath.Join(t.TempDir(), "state")
	requirements := fylostate.SluiceRequirements(qualifiedFyloVersion)

	first, err := fylostate.NewSupervisor(fylostate.Config{
		Binary:       binary,
		Root:         root,
		Requirements: requirements,
	})
	if err != nil {
		t.Fatalf("create first supervisor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	handshake, err := first.Start(ctx)
	if err != nil {
		t.Fatalf("start first supervisor: %v", err)
	}
	if handshake.RuntimeVersion != qualifiedFyloVersion || handshake.ProtocolVersion != 1 {
		t.Fatalf("unexpected handshake: %#v", handshake)
	}

	second, err := fylostate.NewSupervisor(fylostate.Config{
		Binary:       binary,
		Root:         root,
		Requirements: requirements,
	})
	if err != nil {
		t.Fatalf("create second supervisor: %v", err)
	}
	if _, startErr := second.Start(ctx); !errors.Is(startErr, fylostate.ErrRootOwned) {
		t.Fatalf("second owner error = %v, want ErrRootOwned", startErr)
	}

	if _, handshakeErr := first.Handshake(ctx); handshakeErr != nil {
		t.Fatalf("first owner became unhealthy after rejected contender: %v", handshakeErr)
	}
	if closeErr := first.Close(ctx); closeErr != nil {
		t.Fatalf("close first supervisor: %v", closeErr)
	}

	restarted, err := first.Start(ctx)
	if err != nil {
		t.Fatalf("restart supervisor: %v", err)
	}
	if restarted.RuntimeVersion != qualifiedFyloVersion {
		t.Fatalf("restart handshake version = %q", restarted.RuntimeVersion)
	}
	if err := first.Close(ctx); err != nil {
		t.Fatalf("close restarted supervisor: %v", err)
	}
}

func TestInstalledFyloMismatchFailsBeforeOpeningTheRoot(t *testing.T) {
	binary := installedRuntime(t, "fylo")

	tests := []struct {
		name         string
		requirements fylostate.Requirements
	}{
		{
			name:         "runtime",
			requirements: fylostate.SluiceRequirements("0.0.0-not-qualified"),
		},
		{
			name: "protocol",
			requirements: func() fylostate.Requirements {
				requirements := fylostate.SluiceRequirements(qualifiedFyloVersion)
				requirements.ProtocolVersion = 999
				return requirements
			}(),
		},
		{
			name: "capability",
			requirements: func() fylostate.Requirements {
				requirements := fylostate.SluiceRequirements(qualifiedFyloVersion)
				requirements.ServerlessQueueVersion = 999
				return requirements
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "must-not-open")
			supervisor, err := fylostate.NewSupervisor(fylostate.Config{
				Binary:       binary,
				Root:         root,
				Requirements: test.requirements,
			})
			if err != nil {
				t.Fatalf("create supervisor: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := supervisor.Start(ctx); !errors.Is(err, fylostate.ErrCompatibility) {
				t.Fatalf("start error = %v, want ErrCompatibility", err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("mismatch opened state root; stat error: %v", err)
			}
		})
	}
}

func installedRuntime(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("SLUICE_REQUIRE_INSTALLED_RUNTIMES") == "1" {
			t.Fatalf("required installed runtime %q is unavailable: %v", name, err)
		}
		t.Skipf("installed runtime %q is unavailable", name)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve %q: %v", name, err)
	}
	return absolute
}
