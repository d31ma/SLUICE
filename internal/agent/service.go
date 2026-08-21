// Package agent owns the long-running Sluice process lifecycle and its sole
// state-writer boundary.
package agent

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
	"github.com/d31ma/sluice/internal/transport/localapi"
)

const shutdownTimeout = 5 * time.Second

// StateOwner is the persistent state lifecycle exclusively owned by the
// agent. Production supplies the warm Fylo supervisor.
type StateOwner interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

// Config describes one persistent agent instance.
type Config struct {
	State       StateOwner
	Application http.Handler
	Idempotency localapi.IdempotencyStore
	APIToken    string
	SessionFile string
	Listen      string
}

// Service owns state, local API idempotency, and the authenticated IPC server.
type Service struct {
	state       StateOwner
	handler     http.Handler
	sessionFile string
	listen      string
}

// NewService validates immutable configuration without starting state.
func NewService(config Config) (*Service, error) {
	if config.State == nil {
		return nil, fmt.Errorf("agent state owner is required")
	}
	if config.Idempotency == nil {
		return nil, fmt.Errorf("durable agent idempotency store is required")
	}
	if config.SessionFile == "" || !filepath.IsAbs(config.SessionFile) {
		return nil, fmt.Errorf("agent session file must be absolute")
	}
	application := config.Application
	if application == nil {
		application = NewApplication()
	}
	handler, err := localapi.NewHandler(
		config.APIToken,
		config.Idempotency,
		application,
	)
	if err != nil {
		return nil, fmt.Errorf("configure local API: %w", err)
	}
	listen := config.Listen
	if listen == "" {
		listen = "127.0.0.1:0"
	}
	return &Service{
		state:       config.State,
		handler:     handler,
		sessionFile: filepath.Clean(config.SessionFile),
		listen:      listen,
	}, nil
}

// RejectingIdempotencyStore prevents mutations when a durable store has not
// yet been connected. This fail-closed Phase 1 policy makes retries safe across
// agent restarts without pretending that process memory is durable.
type RejectingIdempotencyStore struct{}

// Begin rejects every mutation before the application handler runs.
func (RejectingIdempotencyStore) Begin(context.Context, string, string) (localapi.IdempotencyClaim, error) {
	return nil, errors.New("durable idempotency state is unavailable")
}

// Run owns the Fylo lifecycle and serves until cancellation or listener
// failure. The session descriptor exists only while both are live.
func (service *Service) Run(ctx context.Context) error {
	if err := service.state.Start(ctx); err != nil {
		return fmt.Errorf("start agent state: %w", err)
	}
	capability, err := ipc.NewCapability()
	if err != nil {
		return service.closeStateAfterError(err)
	}
	server, err := ipc.Listen(ipc.ServerConfig{
		Address:    service.listen,
		Capability: capability,
		Handler:    service.handler,
	})
	if err != nil {
		return service.closeStateAfterError(err)
	}
	if err := ipc.WriteDescriptor(service.sessionFile, server.Descriptor()); err != nil {
		_ = server.Close(context.Background())
		return service.closeStateAfterError(err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve() }()
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-serveDone:
	}
	removeDescriptor(service.sessionFile, capability)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	serverErr := server.Close(shutdownCtx)
	stateErr := service.state.Close(shutdownCtx)
	if ctx.Err() != nil {
		runErr = nil
	}
	return errors.Join(runErr, serverErr, stateErr)
}

func (service *Service) closeStateAfterError(cause error) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return errors.Join(cause, service.state.Close(shutdownCtx))
}

func removeDescriptor(path, capability string) {
	descriptor, err := ipc.ReadDescriptor(path)
	if err != nil {
		return
	}
	if subtle.ConstantTimeCompare([]byte(descriptor.Capability), []byte(capability)) == 1 {
		_ = os.Remove(path)
	}
}

// NewApplication returns the Phase 1 observable agent API surface.
func NewApplication() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(response, `{"status":"ok"}`)
	})
	return mux
}
