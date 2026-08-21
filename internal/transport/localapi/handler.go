package localapi

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
)

const maxMutationBody = 1024 * 1024

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{15,127}$`)

type Handler struct {
	store IdempotencyStore
	next  http.Handler
	token []byte
}

func NewHandler(token string, store IdempotencyStore, next http.Handler) (*Handler, error) {
	if len(token) < 32 {
		return nil, errors.New("local API token must contain at least 32 bytes")
	}
	if store == nil {
		return nil, errors.New("local API idempotency store is required")
	}
	if next == nil {
		return nil, errors.New("local API application handler is required")
	}
	return &Handler{token: []byte(token), store: store, next: next}, nil
}

func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	secureResponse(response.Header())
	if !isLoopback(request.RemoteAddr) {
		writeProblem(response, http.StatusForbidden, "forbidden", "Loopback access required", false)
		return
	}
	if !handler.authenticated(request) {
		response.Header().Set("WWW-Authenticate", "Bearer")
		writeProblem(response, http.StatusUnauthorized, "unauthenticated", "Authentication required", false)
		return
	}
	if !isMutation(request.Method) {
		handler.next.ServeHTTP(response, request)
		return
	}
	handler.serveMutation(response, request)
}

func (handler *Handler) authenticated(request *http.Request) bool {
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return false
	}
	presented := []byte(strings.TrimPrefix(authorization, prefix))
	return subtle.ConstantTimeCompare(presented, handler.token) == 1
}

func (handler *Handler) serveMutation(response http.ResponseWriter, request *http.Request) {
	key := request.Header.Get("Idempotency-Key")
	if !idempotencyKeyPattern.MatchString(key) {
		writeProblem(response, http.StatusBadRequest, "invalid_argument", "A valid Idempotency-Key header is required", false)
		return
	}
	body, err := readMutationBody(request)
	if err != nil {
		writeProblem(response, http.StatusRequestEntityTooLarge, "invalid_argument", "Request body exceeds the local API limit", false)
		return
	}
	fingerprint := requestFingerprint(request, body)
	storeKey := request.Method + "\n" + request.URL.Path + "\n" + key
	claim, err := handler.store.Begin(request.Context(), storeKey, fingerprint)
	if err != nil {
		handler.writeClaimError(response, err)
		return
	}
	if replay, found := claim.Replay(); found {
		writeStoredResponse(response, replay)
		return
	}

	completed := false
	defer func() {
		if !completed {
			claim.Abort()
		}
	}()
	recorded := newResponseCapture()
	handler.next.ServeHTTP(recorded, request)
	stored := recorded.StoredResponse()
	if err := claim.Complete(stored); err != nil {
		writeProblem(response, http.StatusServiceUnavailable, "state_unavailable", "Unable to record the operation result", true)
		return
	}
	completed = true
	writeStoredResponse(response, stored)
}

func (handler *Handler) writeClaimError(response http.ResponseWriter, err error) {
	if errors.Is(err, ErrIdempotencyConflict) {
		writeProblem(response, http.StatusConflict, "idempotency_key_reused", "Idempotency key conflicts with an earlier request", false)
		return
	}
	writeProblem(response, http.StatusServiceUnavailable, "state_unavailable", "Unable to claim the operation", true)
}

func readMutationBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxMutationBody+1))
	if err != nil {
		return nil, fmt.Errorf("read mutation body: %w", err)
	}
	if len(body) > maxMutationBody {
		return nil, errors.New("mutation body is too large")
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func requestFingerprint(request *http.Request, body []byte) string {
	digest := sha256.New()
	_, _ = io.WriteString(digest, request.Method)
	_, _ = io.WriteString(digest, "\n")
	_, _ = io.WriteString(digest, request.URL.RequestURI())
	_, _ = io.WriteString(digest, "\n")
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func isLoopback(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return false
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

type problem struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	RequestID string `json:"requestId"`
	Status    int    `json:"status"`
	Retryable bool   `json:"retryable"`
}

func writeProblem(response http.ResponseWriter, status int, code, title string, retryable bool) {
	response.Header().Set("Content-Type", "application/problem+json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(problem{
		Type:      "https://sluice.del.ma/problems/" + code,
		Code:      code,
		Title:     title,
		Status:    status,
		Retryable: retryable,
		Detail:    title,
		RequestID: newRequestID(),
	})
}

func newRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "req-unavailable"
	}
	return "req-" + hex.EncodeToString(random[:])
}

func secureResponse(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
}

func writeStoredResponse(response http.ResponseWriter, stored StoredResponse) {
	for name, values := range stored.Header {
		for _, value := range values {
			response.Header().Add(name, value)
		}
	}
	secureResponse(response.Header())
	response.WriteHeader(stored.Status)
	_, _ = response.Write(stored.Body)
}

type responseCapture struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newResponseCapture() *responseCapture {
	return &responseCapture{header: make(http.Header)}
}

func (capture *responseCapture) Header() http.Header {
	return capture.header
}

func (capture *responseCapture) WriteHeader(status int) {
	if capture.status != 0 {
		return
	}
	capture.status = status
}

func (capture *responseCapture) Write(body []byte) (int, error) {
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	return capture.body.Write(body)
}

func (capture *responseCapture) StoredResponse() StoredResponse {
	status := capture.status
	if status == 0 {
		status = http.StatusOK
	}
	return StoredResponse{Status: status, Header: capture.header.Clone(), Body: capture.body.Bytes()}
}
