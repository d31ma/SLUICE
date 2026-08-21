package localapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/d31ma/sluice/internal/transport/localapi"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestHandlerRejectsNonLoopbackClients(t *testing.T) {
	handler := newHandler(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := authorizedRequest(t, http.MethodGet, "/v1/status", "")
	request.RemoteAddr = "203.0.113.8:4242"
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertProblem(t, response, http.StatusForbidden, "forbidden")
}

func TestHandlerRequiresBearerTokenAndDoesNotAcceptQueryToken(t *testing.T) {
	handler := newHandler(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached application handler")
	}))
	request := httptest.NewRequest(http.MethodGet, "/v1/status?token="+testToken, nil)
	request.RemoteAddr = "127.0.0.1:4242"
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertProblem(t, response, http.StatusUnauthorized, "unauthenticated")
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
}

func TestMutationRequiresIdempotencyKey(t *testing.T) {
	handler := newHandler(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("mutation without idempotency key reached application handler")
	}))
	request := authorizedRequest(t, http.MethodPost, "/v1/roots", `{}`)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertProblem(t, response, http.StatusBadRequest, "invalid_argument")
}

func TestDuplicateMutationReplaysResponseWithoutRepeatingSideEffect(t *testing.T) {
	var calls atomic.Int32
	handler := newHandler(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(response, `{"rootId":"root-1"}`)
	}))

	first := performMutation(t, handler, "request-00000001", `{"path":"/sync"}`)
	second := performMutation(t, handler, "request-00000001", `{"path":"/sync"}`)

	if calls.Load() != 1 {
		t.Fatalf("application calls = %d, want 1", calls.Load())
	}
	if first.Code != http.StatusCreated || second.Code != first.Code || second.Body.String() != first.Body.String() {
		t.Fatalf("replayed response = (%d, %q), want (%d, %q)", second.Code, second.Body.String(), first.Code, first.Body.String())
	}
}

func TestIdempotencyKeyCannotBeReusedForDifferentRequest(t *testing.T) {
	handler := newHandler(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusCreated)
	}))
	_ = performMutation(t, handler, "request-00000002", `{"path":"/one"}`)
	response := performMutation(t, handler, "request-00000002", `{"path":"/two"}`)

	assertProblem(t, response, http.StatusConflict, "idempotency_key_reused")
}

func newHandler(t *testing.T, next http.Handler) http.Handler {
	t.Helper()
	handler, err := localapi.NewHandler(testToken, localapi.NewMemoryIdempotencyStore(), next)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func authorizedRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:4242"
	request.Header.Set("Authorization", "Bearer "+testToken)
	return request
}

func performMutation(t *testing.T, handler http.Handler, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := authorizedRequest(t, http.MethodPost, "/v1/roots", body)
	request.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", contentType)
	}
	var problem struct {
		Type      string `json:"type"`
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != code {
		t.Fatalf("problem code = %q, want %q", problem.Code, code)
	}
	if problem.Type != "https://sluice.del.ma/problems/"+code {
		t.Fatalf("problem type = %q, want stable code URI", problem.Type)
	}
	if !strings.HasPrefix(problem.RequestID, "req-") {
		t.Fatalf("request ID = %q, want generated req-* identifier", problem.RequestID)
	}
}

func TestMemoryStoreWaitCanBeCancelled(t *testing.T) {
	store := localapi.NewMemoryIdempotencyStore()
	claim, err := store.Begin(context.Background(), "request-00000003", "fingerprint")
	if err != nil {
		t.Fatalf("first Begin() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Begin(ctx, "request-00000003", "fingerprint"); err == nil {
		t.Fatal("waiting Begin() error = nil, want context cancellation")
	}
	claim.Abort()
}
