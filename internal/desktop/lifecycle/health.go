package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

const (
	healthRetryInterval = 40 * time.Millisecond
	maximumHealthBytes  = 4096
)

func waitForDescriptor(ctx context.Context, request descriptorWaitRequest) (ipc.Descriptor, error) {
	ticker := time.NewTicker(healthRetryInterval)
	defer ticker.Stop()
	for {
		descriptor, err := ipc.ReadDescriptor(request.Path)
		if err == nil && descriptor.Capability != request.PreviousCapability {
			if _, err := ipc.NewClient(descriptor); err == nil {
				return descriptor, nil
			}
		}
		select {
		case <-request.Process.Done():
			return ipc.Descriptor{}, stoppedProcessError("agent exited before publishing its descriptor", request.Process.Err())
		case <-ctx.Done():
			return ipc.Descriptor{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func chooseLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

func awaitAuthenticatedHealth(
	ctx context.Context,
	endpoint string,
	token string,
	process managedProcess,
) error {
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{Proxy: nil},
	}
	ticker := time.NewTicker(healthRetryInterval)
	defer ticker.Stop()
	for {
		healthy, terminalErr := probeHealthOnce(ctx, client, endpoint, token)
		if healthy || terminalErr != nil {
			return terminalErr
		}
		select {
		case <-process.Done():
			return stoppedProcessError("Tachyon exited before health succeeded", process.Err())
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func stoppedProcessError(message string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func probeHealthOnce(ctx context.Context, client *http.Client, endpoint, token string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/health", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, nil
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumHealthBytes+1))
	if err != nil || len(body) > maximumHealthBytes {
		return false, fmt.Errorf("read local API health response")
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return false, fmt.Errorf("local API rejected its packaged credential")
	}
	if response.StatusCode != http.StatusOK {
		return false, nil
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &status); err != nil || status.Status != "ok" {
		return false, fmt.Errorf("local API returned invalid health data")
	}
	return true, nil
}
