package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

func TestSessionRecoversTheCompleteTopologyAfterTachyonExits(t *testing.T) {
	harness := newSupervisorHarness(t)
	session := harness.start(t)
	firstEndpoint := session.Endpoint()

	harness.tachyon(t, 0).fail(errors.New("signal: terminated"))
	retry := harness.nextRetry(t)
	if retry.delay != initialRestartBackoff {
		t.Fatalf("first retry delay = %s, want %s", retry.delay, initialRestartBackoff)
	}
	close(retry.release)
	harness.awaitTwoGenerations(t)

	harness.awaitRecoveredEndpoint(t, session, firstEndpoint)
	if err := session.Err(); err == nil || !strings.Contains(err.Error(), "Tachyon") ||
		!strings.Contains(err.Error(), "signal: terminated") {
		t.Fatalf("Err() = %v, want actionable Tachyon exit", err)
	}
	if harness.overlapped() {
		t.Fatal("a recovered generation overlapped the generation it replaced")
	}
	harness.closeSession(t, session)
	harness.assertOrdered(t,
		"tachyon-1:exit",
		"tachyon-1:close",
		"agent-1:close",
		"agent-2:start",
	)
}

func TestSessionRecoversTheCompleteTopologyAfterAgentExits(t *testing.T) {
	harness := newSupervisorHarness(t)
	session := harness.start(t)

	harness.agent(t, 0).fail(errors.New("exit status 71"))
	retry := harness.nextRetry(t)
	close(retry.release)
	harness.awaitTwoGenerations(t)

	if err := session.Err(); err == nil || !strings.Contains(err.Error(), "agent") ||
		!strings.Contains(err.Error(), "exit status 71") {
		t.Fatalf("Err() = %v, want actionable agent exit", err)
	}
	if harness.overlapped() {
		t.Fatal("agent recovery started before the old Tachyon process stopped")
	}
	harness.closeSession(t, session)
}

func TestSessionExponentiallyBacksOffAfterRecoveryStartupFailures(t *testing.T) {
	harness := newSupervisorHarness(t)
	harness.agentStartErrors[2] = errors.New("temporary process limit")
	session := harness.start(t)

	harness.tachyon(t, 0).fail(errors.New("exit status 9"))
	first := harness.nextRetry(t)
	close(first.release)
	second := harness.nextRetry(t)
	if first.delay != initialRestartBackoff || second.delay != 2*initialRestartBackoff {
		t.Fatalf("retry delays = %s, %s", first.delay, second.delay)
	}
	close(second.release)
	harness.awaitTwoGenerations(t)

	if err := session.Err(); err == nil || !strings.Contains(err.Error(), "temporary process limit") {
		t.Fatalf("Err() = %v, want retained restart failure", err)
	}
	if harness.agentStartCount() != 3 {
		t.Fatalf("agent start attempts = %d, want 3", harness.agentStartCount())
	}
	harness.closeSession(t, session)
}

func TestSessionCloseDuringBackoffStopsRetriesAndCleansTheGeneration(t *testing.T) {
	harness := newSupervisorHarness(t)
	session := harness.start(t)
	harness.agent(t, 0).fail(errors.New("exit status 42"))
	retry := harness.nextRetry(t)

	harness.closeSession(t, session)
	close(retry.release)
	time.Sleep(20 * time.Millisecond)
	if harness.agentStartCount() != 1 {
		t.Fatalf("agent start attempts after Close = %d, want 1", harness.agentStartCount())
	}
	if harness.activeCount() != 0 {
		t.Fatalf("active processes after Close = %d, want 0", harness.activeCount())
	}
}

func TestSessionCleanCloseDoesNotRetryOrReturnHistoricalRecoveryFailure(t *testing.T) {
	harness := newSupervisorHarness(t)
	session := harness.start(t)
	harness.tachyon(t, 0).fail(errors.New("one recovered failure"))
	retry := harness.nextRetry(t)
	close(retry.release)
	harness.awaitTwoGenerations(t)

	harness.closeSession(t, session)
	if harness.agentStartCount() != 2 {
		t.Fatalf("agent start attempts after clean Close = %d, want 2", harness.agentStartCount())
	}
	if session.Err() == nil {
		t.Fatal("Err() lost the historical recovery diagnostic")
	}
}

func TestBoundedRestartDelayCapsExponentialGrowth(t *testing.T) {
	if delay := boundedRestartDelay(0); delay != initialRestartBackoff {
		t.Fatalf("attempt 0 delay = %s, want %s", delay, initialRestartBackoff)
	}
	if delay := boundedRestartDelay(100); delay != maximumRestartBackoff {
		t.Fatalf("large attempt delay = %s, want %s", delay, maximumRestartBackoff)
	}
}

type retryRequest struct {
	release chan struct{}
	delay   time.Duration
}

type supervisorHarness struct {
	t                *testing.T
	retries          chan retryRequest
	agentStartErrors map[int]error
	stateRoot        string
	events           []string
	agents           []*supervisorProcess
	tachyons         []*supervisorProcess
	mu               sync.Mutex
	agentStarts      int
	port             int
	active           int
	overlap          bool
}

func newSupervisorHarness(t *testing.T) *supervisorHarness {
	t.Helper()
	return &supervisorHarness{
		t:                t,
		stateRoot:        filepath.Join(t.TempDir(), "state"),
		port:             43128,
		retries:          make(chan retryRequest, 8),
		agentStartErrors: make(map[int]error),
	}
}

func (harness *supervisorHarness) start(t *testing.T) *Session {
	t.Helper()
	coordinator, err := newCoordinator(Config{
		Bundle:         lifecycleBundleFixture(t),
		StateRoot:      harness.stateRoot,
		RuntimeVersion: "26.33.02",
		StartupTimeout: time.Second,
	}, harness.dependencies())
	if err != nil {
		t.Fatalf("newCoordinator() error = %v", err)
	}
	session, err := coordinator.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return session
}

func (harness *supervisorHarness) dependencies() dependencies {
	return dependencies{
		startAgent: func(context.Context, agentRequest) (managedProcess, error) {
			harness.mu.Lock()
			defer harness.mu.Unlock()
			harness.agentStarts++
			attempt := harness.agentStarts
			harness.events = append(harness.events, fmt.Sprintf("agent-%d:start", attempt))
			if err := harness.agentStartErrors[attempt]; err != nil {
				return nil, err
			}
			process := newSupervisorProcess(harness, fmt.Sprintf("agent-%d", attempt))
			harness.agents = append(harness.agents, process)
			if harness.active != 0 {
				harness.overlap = true
			}
			harness.active++
			return process, nil
		},
		waitDescriptor: func(context.Context, descriptorWaitRequest) (ipc.Descriptor, error) {
			return ipc.Descriptor{Capability: fmt.Sprintf("generation-%d", harness.agentStartCount())}, nil
		},
		startTachyon: func(context.Context, tachyonRequest) (managedProcess, error) {
			harness.mu.Lock()
			defer harness.mu.Unlock()
			name := fmt.Sprintf("tachyon-%d", len(harness.tachyons)+1)
			harness.events = append(harness.events, name+":start")
			process := newSupervisorProcess(harness, name)
			harness.tachyons = append(harness.tachyons, process)
			harness.active++
			return process, nil
		},
		probeHealth: func(context.Context, string, string, managedProcess) error { return nil },
		choosePort: func() (int, error) {
			harness.mu.Lock()
			defer harness.mu.Unlock()
			harness.port++
			return harness.port, nil
		},
		restartDelay: boundedRestartDelay,
		waitRestart: func(ctx context.Context, delay time.Duration) error {
			request := retryRequest{release: make(chan struct{}), delay: delay}
			select {
			case harness.retries <- request:
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case <-request.release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

func (harness *supervisorHarness) nextRetry(t *testing.T) retryRequest {
	t.Helper()
	select {
	case retry := <-harness.retries:
		return retry
	case <-time.After(time.Second):
		t.Fatal("supervisor did not enter restart backoff")
		return retryRequest{}
	}
}

func (harness *supervisorHarness) awaitTwoGenerations(t *testing.T) {
	t.Helper()
	const count = 2
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		harness.mu.Lock()
		got := len(harness.tachyons)
		harness.mu.Unlock()
		if got >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("healthy generations = %d, want %d", len(harness.tachyons), count)
}

func (*supervisorHarness) awaitRecoveredEndpoint(t *testing.T, session *Session, previous string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if endpoint := session.Endpoint(); endpoint != "" && endpoint != previous {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("recovered endpoint = %q, previous = %q", session.Endpoint(), previous)
}

func (harness *supervisorHarness) agent(t *testing.T, index int) *supervisorProcess {
	t.Helper()
	harness.mu.Lock()
	defer harness.mu.Unlock()
	if index >= len(harness.agents) {
		t.Fatalf("agent index %d is unavailable", index)
	}
	return harness.agents[index]
}

func (harness *supervisorHarness) tachyon(t *testing.T, index int) *supervisorProcess {
	t.Helper()
	harness.mu.Lock()
	defer harness.mu.Unlock()
	if index >= len(harness.tachyons) {
		t.Fatalf("Tachyon index %d is unavailable", index)
	}
	return harness.tachyons[index]
}

func (harness *supervisorHarness) closeSession(t *testing.T, session *Session) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := session.Close(ctx); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func (harness *supervisorHarness) assertOrdered(t *testing.T, expected ...string) {
	t.Helper()
	harness.mu.Lock()
	events := append([]string(nil), harness.events...)
	harness.mu.Unlock()
	position := 0
	for _, event := range events {
		if position < len(expected) && event == expected[position] {
			position++
		}
	}
	if position != len(expected) {
		t.Fatalf("events = %v, want ordered subsequence %v", events, expected)
	}
}

func (harness *supervisorHarness) agentStartCount() int {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.agentStarts
}

func (harness *supervisorHarness) activeCount() int {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.active
}

func (harness *supervisorHarness) overlapped() bool {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return harness.overlap
}

type supervisorProcess struct {
	harness *supervisorHarness
	name    string
	done    chan struct{}
	errText string
	finish  sync.Once
	close   sync.Once
	errMu   sync.Mutex
}

func newSupervisorProcess(harness *supervisorHarness, name string) *supervisorProcess {
	return &supervisorProcess{harness: harness, name: name, done: make(chan struct{})}
}

func (process *supervisorProcess) Done() <-chan struct{} { return process.done }

func (process *supervisorProcess) Err() error {
	process.errMu.Lock()
	defer process.errMu.Unlock()
	if process.errText == "" {
		return nil
	}
	return errors.New(process.errText)
}

func (process *supervisorProcess) fail(err error) {
	process.harness.mu.Lock()
	process.harness.events = append(process.harness.events, process.name+":exit")
	process.harness.mu.Unlock()
	process.complete(err)
}

func (process *supervisorProcess) Close(context.Context) error {
	process.close.Do(func() {
		process.harness.mu.Lock()
		process.harness.events = append(process.harness.events, process.name+":close")
		process.harness.mu.Unlock()
		process.complete(nil)
	})
	return nil
}

func (process *supervisorProcess) complete(err error) {
	process.finish.Do(func() {
		process.errMu.Lock()
		if err != nil {
			process.errText = err.Error()
		}
		process.errMu.Unlock()
		process.harness.mu.Lock()
		process.harness.active--
		process.harness.mu.Unlock()
		close(process.done)
	})
}
