package ipc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Client performs bounded one-shot requests against a live agent descriptor.
type Client struct {
	descriptor Descriptor
}

var wireResponseErrors = map[string]error{
	"unauthenticated":    ErrUnauthenticated,
	"protocol_mismatch":  ErrProtocolMismatch,
	"response_too_large": ErrFrameTooLarge,
}

// NewClient validates connection data before any request is attempted.
func NewClient(descriptor Descriptor) (*Client, error) {
	if err := validateDescriptor(descriptor); err != nil {
		return nil, err
	}
	return &Client{descriptor: descriptor}, nil
}

// NewClientFromFile loads a private session descriptor.
func NewClientFromFile(path string) (*Client, error) {
	descriptor, err := ReadDescriptor(path)
	if err != nil {
		return nil, err
	}
	return NewClient(descriptor)
}

// Descriptor returns a copy of the client's immutable connection data.
func (client *Client) Descriptor() Descriptor {
	return client.descriptor
}

// RoundTrip forwards one HTTP request while preserving its authentication and
// idempotency headers independently from the IPC session capability.
func (client *Client) RoundTrip(ctx context.Context, request *http.Request) (*HTTPResponse, error) {
	frame, err := client.prepareRequest(request)
	if err != nil {
		return nil, err
	}
	connection, err := client.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = connection.Close() }()
	stopCancellation := configureConnection(ctx, connection)
	defer stopCancellation()
	response, err := exchangeRequest(connection, frame)
	if err != nil {
		return nil, err
	}
	return validatedHTTPResponse(frame.RequestID, response)
}

func (client *Client) prepareRequest(request *http.Request) (wireRequest, error) {
	target, err := requestTarget(request)
	if err != nil {
		return wireRequest{}, err
	}
	body, err := readRequestBody(request.Body)
	if err != nil {
		return wireRequest{}, err
	}
	return wireRequest{
		ProtocolVersion: ProtocolVersion,
		Kind:            "request",
		RequestID:       requestID(),
		Capability:      client.descriptor.Capability,
		Method:          request.Method,
		Target:          target,
		Headers:         request.Header.Clone(),
		Body:            body,
	}, nil
}

func requestTarget(request *http.Request) (string, error) {
	if request == nil {
		return "", fmt.Errorf("invalid local API request")
	}
	return validatedRequestTarget(request.URL)
}

func validatedRequestTarget(requestURL *url.URL) (string, error) {
	if requestURL == nil {
		return "", fmt.Errorf("invalid local API request")
	}
	target := requestURL.RequestURI()
	if !validRequestTarget(target) {
		return "", fmt.Errorf("invalid local API request")
	}
	return target, nil
}

func (client *Client) connect(ctx context.Context) (net.Conn, error) {
	connection, err := dialTransport(ctx, client.descriptor.Network, client.descriptor.Address)
	if err != nil {
		return nil, fmt.Errorf("%w: connect to local agent", ErrUnavailable)
	}
	return connection, nil
}

func configureConnection(ctx context.Context, connection net.Conn) func() bool {
	_ = connection.SetDeadline(connectionContextDeadline(ctx))
	return context.AfterFunc(ctx, func() { _ = connection.Close() })
}

func connectionContextDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(connectionDeadline)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

func exchangeRequest(connection net.Conn, frame wireRequest) (wireResponse, error) {
	encoded, err := encodeWireRequest(frame)
	if err != nil {
		return wireResponse{}, err
	}
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		return wireResponse{}, fmt.Errorf("%w: send local agent request", ErrUnavailable)
	}
	return receiveWireResponse(connection)
}

func encodeWireRequest(frame wireRequest) ([]byte, error) {
	encoded, err := json.Marshal(frame)
	if err != nil {
		return nil, fmt.Errorf("encode agent request: %w", err)
	}
	if len(encoded) > MaxRequestBytes {
		return nil, ErrFrameTooLarge
	}
	return encoded, nil
}

func receiveWireResponse(connection net.Conn) (wireResponse, error) {
	response, err := readWireResponse(connection)
	if errors.Is(err, ErrFrameTooLarge) {
		return wireResponse{}, err
	}
	if err != nil {
		return wireResponse{}, fmt.Errorf("%w: receive local agent response", ErrUnavailable)
	}
	return response, nil
}

func validatedHTTPResponse(requestID string, response wireResponse) (*HTTPResponse, error) {
	if err := validateWireResponse(requestID, response); err != nil {
		return nil, err
	}
	return &HTTPResponse{
		StatusCode: response.StatusCode,
		Header:     response.Headers.Clone(),
		Body:       response.Body,
	}, nil
}

func validateWireResponse(requestID string, response wireResponse) error {
	if !matchingResponseEnvelope(requestID, response) {
		return ErrProtocolMismatch
	}
	return validateWireResponseOutcome(response)
}

func validateWireResponseOutcome(response wireResponse) error {
	if err := wireResponseError(response.ErrorCode); err != nil {
		return err
	}
	if response.StatusCode < 100 || response.StatusCode > 599 {
		return ErrProtocolMismatch
	}
	return nil
}

func matchingResponseEnvelope(requestID string, response wireResponse) bool {
	return response.ProtocolVersion == ProtocolVersion &&
		response.Kind == "response" &&
		response.RequestID == requestID
}

func wireResponseError(code string) error {
	if code == "" {
		return nil
	}
	if responseErr, known := wireResponseErrors[code]; known {
		return responseErr
	}
	return fmt.Errorf("%w: agent rejected the request", ErrUnavailable)
}

func readRequestBody(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, MaxRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read local API request body: %w", err)
	}
	if len(data) > MaxRequestBytes {
		return nil, ErrFrameTooLarge
	}
	return data, nil
}

func readWireResponse(input io.Reader) (wireResponse, error) {
	line, err := readFrame(input, MaxResponseBytes)
	if err != nil {
		return wireResponse{}, err
	}
	var response wireResponse
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return wireResponse{}, fmt.Errorf("decode agent response")
	}
	return response, nil
}

func requestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "ipc-request-unavailable"
	}
	return "ipc-" + hex.EncodeToString(random[:])
}
