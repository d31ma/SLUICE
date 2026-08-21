package fylo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/d31ma/sluice/internal/transport/localapi"
)

const (
	idempotencyCollection = "idempotency"
	recordSchemaVersion   = 1
	recordStatePending    = "pending"
	recordStateCompleted  = "completed"
	idempotencyWriteLimit = 5 * time.Second
)

var errPendingIdempotencyClaim = errors.New("idempotency result is pending recovery")

// IdempotencyStore persists local API mutation reservations and responses in
// the warm Fylo process owned by the agent.
type IdempotencyStore struct {
	supervisor *Supervisor
	mu         sync.Mutex
	ready      bool
}

type idempotencyRecord struct {
	Header        http.Header `json:"header,omitempty"`
	KeyHash       string      `json:"keyHash"`
	Fingerprint   string      `json:"fingerprint"`
	State         string      `json:"state"`
	Body          []byte      `json:"body,omitempty"`
	SchemaVersion int         `json:"schemaVersion"`
	Status        int         `json:"status,omitempty"`
}

type storedDocument struct {
	id     string
	record idempotencyRecord
}

// NewIdempotencyStore binds durable local API replay state to a supervisor.
// The supervisor must be started before Begin is called.
func NewIdempotencyStore(supervisor *Supervisor) (*IdempotencyStore, error) {
	if supervisor == nil {
		return nil, fmt.Errorf("fylo supervisor is required")
	}
	return &IdempotencyStore{supervisor: supervisor}, nil
}

// Begin durably reserves a mutation key before application code runs.
func (store *IdempotencyStore) Begin(
	ctx context.Context,
	key string,
	fingerprint string,
) (localapi.IdempotencyClaim, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.ensureCollection(ctx); err != nil {
		return nil, err
	}
	document, found, err := store.find(ctx, hashKey(key))
	if err != nil {
		return nil, err
	}
	if found {
		return existingClaim(document, fingerprint)
	}
	return store.reserve(ctx, hashKey(key), fingerprint)
}

func (store *IdempotencyStore) ensureCollection(ctx context.Context) error {
	if store.ready {
		return nil
	}
	_, err := store.supervisor.execute(ctx, "createCollection", map[string]any{
		"collection": idempotencyCollection,
		"kind":       "document",
	})
	if err != nil {
		return fmt.Errorf("create Fylo idempotency collection: %w", err)
	}
	store.ready = true
	return nil
}

func (store *IdempotencyStore) find(
	ctx context.Context,
	keyHash string,
) (storedDocument, bool, error) {
	result, err := store.supervisor.execute(ctx, "findDocs", map[string]any{
		"collection": idempotencyCollection,
		"query":      map[string]any{"keyHash": keyHash},
	})
	if err != nil {
		return storedDocument{}, false, fmt.Errorf("find Fylo idempotency claim: %w", err)
	}
	var documents map[string]idempotencyRecord
	if err := json.Unmarshal(result, &documents); err != nil {
		return storedDocument{}, false, fmt.Errorf("decode Fylo idempotency claim: %w", err)
	}
	return uniqueDocument(documents)
}

func uniqueDocument(documents map[string]idempotencyRecord) (storedDocument, bool, error) {
	if len(documents) == 0 {
		return storedDocument{}, false, nil
	}
	if len(documents) != 1 {
		return storedDocument{}, false, fmt.Errorf("multiple Fylo idempotency claims share one key hash")
	}
	for identifier, record := range documents {
		if err := validateRecord(record); err != nil {
			return storedDocument{}, false, err
		}
		return storedDocument{id: identifier, record: record}, true, nil
	}
	return storedDocument{}, false, fmt.Errorf("fylo idempotency result is inconsistent")
}

func validateRecord(record idempotencyRecord) error {
	if err := validateRecordShape(record); err != nil {
		return err
	}
	if err := validateRecordState(record.State); err != nil {
		return err
	}
	return validateCompletedStatus(record.State, record.Status)
}

func validateRecordShape(record idempotencyRecord) error {
	if record.SchemaVersion != recordSchemaVersion || record.KeyHash == "" || record.Fingerprint == "" {
		return fmt.Errorf("fylo idempotency record has an incompatible shape")
	}
	return nil
}

func validateRecordState(state string) error {
	if state != recordStatePending && state != recordStateCompleted {
		return fmt.Errorf("fylo idempotency record has an unknown state")
	}
	return nil
}

func validateCompletedStatus(state string, status int) error {
	if state == recordStateCompleted && (status < 100 || status > 599) {
		return fmt.Errorf("fylo idempotency record has an invalid HTTP status")
	}
	return nil
}

func existingClaim(
	document storedDocument,
	fingerprint string,
) (localapi.IdempotencyClaim, error) {
	if document.record.Fingerprint != fingerprint {
		return nil, localapi.ErrIdempotencyConflict
	}
	if document.record.State != recordStateCompleted {
		return nil, errPendingIdempotencyClaim
	}
	return &fyloClaim{replay: storedResponse(document.record)}, nil
}

func (store *IdempotencyStore) reserve(
	ctx context.Context,
	keyHash string,
	fingerprint string,
) (localapi.IdempotencyClaim, error) {
	record := idempotencyRecord{
		SchemaVersion: recordSchemaVersion,
		KeyHash:       keyHash,
		Fingerprint:   fingerprint,
		State:         recordStatePending,
	}
	result, err := store.supervisor.execute(ctx, "putData", map[string]any{
		"collection": idempotencyCollection,
		"data":       record,
	})
	if err != nil {
		return nil, fmt.Errorf("reserve Fylo idempotency claim: %w", err)
	}
	var identifier string
	if err := json.Unmarshal(result, &identifier); err != nil || identifier == "" {
		return nil, fmt.Errorf("decode Fylo idempotency reservation")
	}
	return &fyloClaim{
		store:    store,
		document: storedDocument{id: identifier, record: record},
		owner:    true,
	}, nil
}

type fyloClaim struct {
	completionErr error
	store         *IdempotencyStore
	replay        localapi.StoredResponse
	document      storedDocument
	once          sync.Once
	owner         bool
}

func (claim *fyloClaim) Replay() (localapi.StoredResponse, bool) {
	if claim.owner {
		return localapi.StoredResponse{}, false
	}
	return cloneStoredResponse(claim.replay), true
}

func (claim *fyloClaim) Complete(response localapi.StoredResponse) error {
	if !claim.owner {
		return nil
	}
	claim.once.Do(func() {
		claim.completionErr = claim.store.complete(claim.document, response)
	})
	return claim.completionErr
}

// Abort intentionally preserves the pending reservation. Once application
// code may have run, deleting it could repeat a side effect after a crash.
func (*fyloClaim) Abort() {}

func (store *IdempotencyStore) complete(
	document storedDocument,
	response localapi.StoredResponse,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	record := document.record
	record.State = recordStateCompleted
	record.Status = response.Status
	record.Header = response.Header.Clone()
	record.Body = append([]byte(nil), response.Body...)
	if err := validateRecord(record); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), idempotencyWriteLimit)
	defer cancel()
	_, err := store.supervisor.execute(ctx, "patchDoc", map[string]any{
		"collection": idempotencyCollection,
		"id":         document.id,
		"newDoc":     record,
	})
	if err != nil {
		return fmt.Errorf("complete Fylo idempotency claim: %w", err)
	}
	return nil
}

func hashKey(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func storedResponse(record idempotencyRecord) localapi.StoredResponse {
	return localapi.StoredResponse{
		Status: record.Status,
		Header: record.Header.Clone(),
		Body:   append([]byte(nil), record.Body...),
	}
}

func cloneStoredResponse(response localapi.StoredResponse) localapi.StoredResponse {
	return localapi.StoredResponse{
		Status: response.Status,
		Header: response.Header.Clone(),
		Body:   append([]byte(nil), response.Body...),
	}
}
