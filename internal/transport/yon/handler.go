// Package yon implements Sluice's bounded Tachyon direct-handler adapter and
// the lifecycle wrapper used to qualify the installed ty runtime.
package yon

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

const (
	handlerProtocolVersion = 1
	maxHandlerRequestBytes = 16 * 1024 * 1024
	defaultRequestTimeout  = 30 * time.Second
	agentSessionFileEnv    = "SLUICE_AGENT_SESSION_FILE"
)

var textBodyEncodings = map[string]struct{}{
	"":      {},
	"utf8":  {},
	"utf-8": {},
	"text":  {},
}

// Request is Tachyon Handler Protocol v1's direct request envelope.
type Request struct {
	Headers         map[string][]string `json:"headers"`
	Body            *RequestBody        `json:"body,omitempty"`
	Parameters      map[string]string   `json:"parameters,omitempty"`
	Kind            string              `json:"kind"`
	RequestID       string              `json:"request_id"`
	Operation       string              `json:"operation"`
	Route           string              `json:"route"`
	Method          string              `json:"method"`
	ProtocolVersion int                 `json:"protocol_version"`
	DeadlineMS      int                 `json:"deadline_ms,omitempty"`
}

// RequestBody is the bounded body representation supplied by Tachyon.
type RequestBody struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

// Response is the compact object a Tachyon direct handler writes.
type Response struct {
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
	Status  int                 `json:"status"`
}

// Serve reads exactly one bounded Handler Protocol request and writes exactly
// one direct-handler response.
func Serve(input io.Reader, output io.Writer) error {
	return ServeWithSessionFile(input, output, os.Getenv(agentSessionFileEnv))
}

// ServeWithSessionFile bridges exactly one Tachyon request to the persistent
// agent described by sessionFile. It is exported for process-contract tests.
func ServeWithSessionFile(input io.Reader, output io.Writer, sessionFile string) error {
	request, err := decodeRequest(input)
	if err != nil {
		return err
	}
	response := forward(request, sessionFile)
	if err := json.NewEncoder(output).Encode(response); err != nil {
		return fmt.Errorf("encode Yon response: %w", err)
	}
	return nil
}

func decodeRequest(input io.Reader) (Request, error) {
	limited := &io.LimitedReader{R: input, N: maxHandlerRequestBytes + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, fmt.Errorf("decode Yon request: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Request{}, fmt.Errorf("yon request must contain exactly one JSON object")
	}
	if limited.N == 0 {
		return Request{}, fmt.Errorf("yon request exceeds %d bytes", maxHandlerRequestBytes)
	}
	if request.ProtocolVersion != handlerProtocolVersion {
		return Request{}, fmt.Errorf("unsupported Yon handler protocol version")
	}
	if request.Kind != "request" || request.RequestID == "" {
		return Request{}, fmt.Errorf("invalid Yon handler request envelope")
	}
	return request, nil
}

func forward(request Request, sessionFile string) Response {
	client, err := ipc.NewClientFromFile(sessionFile)
	if err != nil {
		return agentFailure(err)
	}
	body, err := requestBody(request.Body)
	if err != nil {
		return problemResponse(http.StatusBadRequest, "invalid_argument", false)
	}
	httpRequest, err := http.NewRequest(request.Method, "http://sluice.local"+request.Route, bytes.NewReader(body))
	if err != nil {
		return problemResponse(http.StatusBadRequest, "invalid_argument", false)
	}
	httpRequest.Header = canonicalHeaders(request.Headers)
	ctx, cancel := requestContext(request.DeadlineMS)
	defer cancel()
	response, err := client.RoundTrip(ctx, httpRequest)
	if err != nil {
		return agentFailure(err)
	}
	return Response{
		Status:  response.StatusCode,
		Headers: map[string][]string(response.Header.Clone()),
		Body:    string(response.Body),
	}
}

func canonicalHeaders(headers map[string][]string) http.Header {
	canonical := make(http.Header, len(headers))
	for name, values := range headers {
		for _, value := range values {
			canonical.Add(name, value)
		}
	}
	return canonical
}

func requestBody(body *RequestBody) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return decodeRequestBody(strings.ToLower(body.Encoding), body.Data)
}

func decodeRequestBody(encoding, data string) ([]byte, error) {
	if _, textual := textBodyEncodings[encoding]; textual {
		return []byte(data), nil
	}
	if encoding == "base64" {
		return decodeBase64RequestBody(data)
	}
	return nil, fmt.Errorf("unsupported Yon request body encoding")
}

func decodeBase64RequestBody(data string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode Yon request body")
	}
	return decoded, nil
}

func requestContext(deadlineMilliseconds int) (context.Context, context.CancelFunc) {
	timeout := defaultRequestTimeout
	if deadlineMilliseconds > 0 {
		requested := time.Duration(deadlineMilliseconds) * time.Millisecond
		if requested < timeout {
			timeout = requested
		}
	}
	return context.WithTimeout(context.Background(), timeout)
}

func agentFailure(err error) Response {
	switch {
	case errors.Is(err, ipc.ErrProtocolMismatch):
		return problemResponse(http.StatusServiceUnavailable, "agent_protocol_mismatch", false)
	case errors.Is(err, ipc.ErrFrameTooLarge):
		return problemResponse(http.StatusRequestEntityTooLarge, "request_too_large", false)
	default:
		return problemResponse(http.StatusServiceUnavailable, "agent_unavailable", true)
	}
}

func problemResponse(status int, code string, retryable bool) Response {
	body, err := json.Marshal(map[string]any{
		"type":      "https://sluice.del.ma/problems/" + code,
		"code":      code,
		"title":     http.StatusText(status),
		"status":    status,
		"retryable": retryable,
		"detail":    http.StatusText(status),
	})
	if err != nil {
		return jsonResponse(status, `{"code":"agent_unavailable"}`)
	}
	response := jsonResponse(status, string(body))
	response.Headers["content-type"] = []string{"application/problem+json"}
	response.Headers["cache-control"] = []string{"no-store"}
	return response
}

func jsonResponse(status int, body string) Response {
	return Response{
		Status:  status,
		Headers: map[string][]string{"content-type": {"application/json"}},
		Body:    body,
	}
}
