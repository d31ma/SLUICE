package fylo

import (
	"context"
	"encoding/json"
	"fmt"
)

// QueuePublication identifies one durably published message.
type QueuePublication struct {
	ID           string `json:"id"`
	Deduplicated bool   `json:"deduplicated"`
}

// QueueDelivery is one leased Fylo queue message.
type QueueDelivery struct {
	ID             string          `json:"id"`
	Topic          string          `json:"topic"`
	Receipt        string          `json:"receipt"`
	Payload        json.RawMessage `json:"payload"`
	PublishedAt    int64           `json:"publishedAt"`
	LeaseExpiresAt int64           `json:"leaseExpiresAt"`
	Attempt        int             `json:"attempt"`
}

// QueuePublish durably publishes a payload with a required deduplication key.
func (supervisor *Supervisor) QueuePublish(
	ctx context.Context,
	topic string,
	payload any,
	idempotencyKey string,
) (QueuePublication, error) {
	if topic == "" || idempotencyKey == "" {
		return QueuePublication{}, fmt.Errorf("fylo queue topic and idempotency key are required")
	}
	result, err := supervisor.execute(ctx, "queuePublish", map[string]any{
		"topic":          topic,
		"payload":        payload,
		"idempotencyKey": idempotencyKey,
	})
	if err != nil {
		return QueuePublication{}, fmt.Errorf("publish Fylo queue message: %w", err)
	}
	var publication QueuePublication
	if err := json.Unmarshal(result, &publication); err != nil || publication.ID == "" {
		return QueuePublication{}, fmt.Errorf("decode Fylo queue publication")
	}
	return publication, nil
}

// QueueClaim leases at most maxMessages for a bounded visibility interval.
func (supervisor *Supervisor) QueueClaim(
	ctx context.Context,
	topic string,
	group string,
	maxMessages int,
	visibilityTimeoutMS int,
	maxAttempts int,
) ([]QueueDelivery, error) {
	if err := validateQueueClaim(topic, group, maxMessages, visibilityTimeoutMS, maxAttempts); err != nil {
		return nil, err
	}
	result, err := supervisor.execute(ctx, "queueClaim", map[string]any{
		"topic":               topic,
		"group":               group,
		"maxMessages":         maxMessages,
		"visibilityTimeoutMs": visibilityTimeoutMS,
		"maxAttempts":         maxAttempts,
	})
	if err != nil {
		return nil, fmt.Errorf("claim Fylo queue messages: %w", err)
	}
	var deliveries []QueueDelivery
	if err := json.Unmarshal(result, &deliveries); err != nil {
		return nil, fmt.Errorf("decode Fylo queue deliveries: %w", err)
	}
	for _, delivery := range deliveries {
		if delivery.ID == "" || delivery.Receipt == "" || delivery.Attempt < 1 {
			return nil, fmt.Errorf("fylo queue returned an invalid delivery")
		}
	}
	return deliveries, nil
}

func validateQueueClaim(topic, group string, maximum, visibility, attempts int) error {
	if topic == "" || group == "" {
		return fmt.Errorf("fylo queue topic and group are required")
	}
	if maximum < 1 || maximum > 100 {
		return fmt.Errorf("fylo queue claim maximum is outside 1..100")
	}
	if visibility < 1 || attempts < 1 {
		return fmt.Errorf("fylo queue visibility and attempts must be positive")
	}
	return nil
}

// QueueAck permanently settles one leased message using its opaque receipt.
func (supervisor *Supervisor) QueueAck(
	ctx context.Context,
	topic string,
	group string,
	id string,
	receipt string,
) error {
	if topic == "" || group == "" || id == "" || receipt == "" {
		return fmt.Errorf("fylo queue acknowledgement fields are required")
	}
	_, err := supervisor.execute(ctx, "queueAck", map[string]any{
		"topic":   topic,
		"group":   group,
		"id":      id,
		"receipt": receipt,
	})
	if err != nil {
		return fmt.Errorf("acknowledge Fylo queue message: %w", err)
	}
	return nil
}
