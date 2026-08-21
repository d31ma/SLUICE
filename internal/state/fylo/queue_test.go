package fylo_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	fylostate "github.com/d31ma/sluice/internal/state/fylo"
)

func TestInstalledFyloQueueClaimRecoversAfterOwnerRestart(t *testing.T) {
	binary := installedFyloRuntime(t)
	root := filepath.Join(t.TempDir(), "state")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	first := startedSupervisor(t, binary, root)
	publication, err := first.QueuePublish(
		ctx,
		"sluice.operations.v1",
		map[string]any{"operationId": "op-1", "fence": 7},
		"publish-op-1",
	)
	if err != nil {
		t.Fatalf("publish queue message: %v", err)
	}
	claimed, err := first.QueueClaim(ctx, "sluice.operations.v1", "agent-v1", 1, 100, 3)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	firstDelivery := requireOneDelivery(t, claimed, publication.ID)
	closeSupervisor(t, first)

	time.Sleep(150 * time.Millisecond)
	second := startedSupervisor(t, binary, root)
	defer closeSupervisor(t, second)
	recovered, err := second.QueueClaim(ctx, "sluice.operations.v1", "agent-v1", 1, 100, 3)
	if err != nil {
		t.Fatalf("recovery claim: %v", err)
	}
	secondDelivery := requireOneDelivery(t, recovered, publication.ID)
	if secondDelivery.Attempt != 2 || secondDelivery.Receipt == firstDelivery.Receipt {
		t.Fatalf("recovered delivery attempt/receipt = %d/%q", secondDelivery.Attempt, secondDelivery.Receipt)
	}
	var payload struct {
		OperationID string `json:"operationId"`
		Fence       int    `json:"fence"`
	}
	if decodeErr := json.Unmarshal(secondDelivery.Payload, &payload); decodeErr != nil {
		t.Fatalf("decode recovered payload: %v", decodeErr)
	}
	if payload.OperationID != "op-1" || payload.Fence != 7 {
		t.Fatalf("recovered payload = %#v", payload)
	}
	if ackErr := second.QueueAck(ctx, secondDelivery.Topic, "agent-v1", secondDelivery.ID, secondDelivery.Receipt); ackErr != nil {
		t.Fatalf("ack recovered delivery: %v", ackErr)
	}
	empty, err := second.QueueClaim(ctx, "sluice.operations.v1", "agent-v1", 1, 100, 3)
	if err != nil {
		t.Fatalf("claim after ack: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("deliveries after ack = %d, want 0", len(empty))
	}
}

func requireOneDelivery(t *testing.T, deliveries []fylostate.QueueDelivery, id string) fylostate.QueueDelivery {
	t.Helper()
	if len(deliveries) != 1 {
		t.Fatalf("delivery count = %d, want 1", len(deliveries))
	}
	if deliveries[0].ID != id {
		t.Fatalf("delivery ID = %q, want %q", deliveries[0].ID, id)
	}
	return deliveries[0]
}
