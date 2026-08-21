package localapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

var ErrIdempotencyConflict = errors.New("idempotency key was already used for a different request")

type StoredResponse struct {
	Header http.Header
	Body   []byte
	Status int
}

type IdempotencyClaim interface {
	Replay() (StoredResponse, bool)
	Complete(response StoredResponse) error
	Abort()
}

type IdempotencyStore interface {
	Begin(ctx context.Context, key, fingerprint string) (IdempotencyClaim, error)
}

type memoryEntry struct {
	done        chan struct{}
	fingerprint string
	response    StoredResponse
	completed   bool
}

type MemoryIdempotencyStore struct {
	entries map[string]*memoryEntry
	mu      sync.Mutex
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{entries: make(map[string]*memoryEntry)}
}

func (store *MemoryIdempotencyStore) Begin(
	ctx context.Context,
	key string,
	fingerprint string,
) (IdempotencyClaim, error) {
	for {
		store.mu.Lock()
		entry, found := store.entries[key]
		if !found {
			entry = &memoryEntry{fingerprint: fingerprint, done: make(chan struct{})}
			store.entries[key] = entry
			store.mu.Unlock()
			return &memoryClaim{store: store, key: key, entry: entry, owner: true}, nil
		}
		if entry.fingerprint != fingerprint {
			store.mu.Unlock()
			return nil, ErrIdempotencyConflict
		}
		if entry.completed {
			response := cloneResponse(entry.response)
			store.mu.Unlock()
			return &memoryClaim{replay: &response}, nil
		}
		done := entry.done
		store.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

type memoryClaim struct {
	store  *MemoryIdempotencyStore
	entry  *memoryEntry
	replay *StoredResponse
	key    string
	once   sync.Once
	owner  bool
}

func (claim *memoryClaim) Replay() (StoredResponse, bool) {
	if claim.replay == nil {
		return StoredResponse{}, false
	}
	return cloneResponse(*claim.replay), true
}

func (claim *memoryClaim) Complete(response StoredResponse) error {
	if !claim.owner {
		return nil
	}
	claim.once.Do(func() {
		claim.store.mu.Lock()
		defer claim.store.mu.Unlock()
		claim.entry.response = cloneResponse(response)
		claim.entry.completed = true
		close(claim.entry.done)
	})
	return nil
}

func (claim *memoryClaim) Abort() {
	if !claim.owner {
		return
	}
	claim.once.Do(func() {
		claim.store.mu.Lock()
		defer claim.store.mu.Unlock()
		if claim.store.entries[claim.key] == claim.entry {
			delete(claim.store.entries, claim.key)
		}
		close(claim.entry.done)
	})
}

func cloneResponse(response StoredResponse) StoredResponse {
	return StoredResponse{
		Status: response.Status,
		Header: response.Header.Clone(),
		Body:   append([]byte(nil), response.Body...),
	}
}
