package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	defaultMaximumConnections = 32
	connectionDeadline        = 30 * time.Second
)

// ServerConfig configures one local agent protocol listener. Network is
// required unless Address explicitly selects the diagnostic loopback TCP
// compatibility mode.
type ServerConfig struct {
	Handler            http.Handler
	Network            string
	Address            string
	Capability         string
	MaximumConnections int
}

// Server serves one request per authenticated local connection.
type Server struct {
	handler     http.Handler
	listener    net.Listener
	runContext  context.Context
	closeErr    error
	closed      chan struct{}
	slots       chan struct{}
	cancel      context.CancelFunc
	cleanup     func() error
	connections map[net.Conn]struct{}
	descriptor  Descriptor
	workers     sync.WaitGroup
	closeOnce   sync.Once
	admission   sync.Mutex
	stopping    bool
}

// Listen binds an explicit local endpoint without starting the accept loop.
// An omitted Network with a non-empty Address selects diagnostic loopback TCP
// for compatibility with contract tests and troubleshooting tools.
func Listen(config ServerConfig) (*Server, error) {
	if config.Handler == nil {
		return nil, fmt.Errorf("agent IPC handler is required")
	}
	network := config.Network
	if network == "" && config.Address != "" {
		network = "tcp"
	}
	probe := Descriptor{
		ProtocolVersion: ProtocolVersion,
		Network:         network,
		Address:         config.Address,
		Capability:      config.Capability,
	}
	if err := validateDescriptor(probe); err != nil {
		return nil, err
	}
	maximum := config.MaximumConnections
	if maximum == 0 {
		maximum = defaultMaximumConnections
	}
	if maximum < 1 || maximum > 1024 {
		return nil, fmt.Errorf("maximum agent IPC connections must be within 1..1024")
	}
	listener, cleanup, err := listenTransport(probe.Network, probe.Address)
	if err != nil {
		return nil, fmt.Errorf("listen for Sluice agent IPC: %w", err)
	}
	descriptor := probe
	descriptor.Address = listener.Addr().String()
	runContext, cancel := context.WithCancel(context.Background())
	return &Server{
		listener:    listener,
		handler:     config.Handler,
		runContext:  runContext,
		cancel:      cancel,
		descriptor:  descriptor,
		connections: make(map[net.Conn]struct{}),
		slots:       make(chan struct{}, maximum),
		closed:      make(chan struct{}),
		cleanup:     cleanup,
	}, nil
}

// ListenNative binds the platform's production local transport: a Unix-domain
// socket on macOS/Linux or a current-user-only named pipe on Windows.
func ListenNative(config ServerConfig, sessionFile string) (*Server, error) {
	if config.Network != "" || config.Address != "" {
		return nil, fmt.Errorf("native agent IPC does not accept an explicit endpoint")
	}
	network, address, err := nativeEndpoint(sessionFile, config.Capability)
	if err != nil {
		return nil, err
	}
	config.Network = network
	config.Address = address
	return Listen(config)
}

// Descriptor returns the immutable connection information for this server.
func (server *Server) Descriptor() Descriptor {
	return server.descriptor
}

// Serve accepts connections until Close is called.
func (server *Server) Serve() error {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			select {
			case <-server.closed:
				return nil
			default:
				return fmt.Errorf("accept Sluice agent IPC connection: %w", err)
			}
		}
		server.admission.Lock()
		if server.stopping {
			server.admission.Unlock()
			_ = connection.Close()
			return nil
		}
		server.connections[connection] = struct{}{}
		server.workers.Add(1)
		server.admission.Unlock()
		select {
		case server.slots <- struct{}{}:
		case <-server.closed:
			server.finishConnection(connection)
			return nil
		}
		go server.serveConnection(connection)
	}
}

func (server *Server) serveConnection(connection net.Conn) {
	defer func() {
		<-server.slots
		server.finishConnection(connection)
	}()
	_ = connection.SetDeadline(time.Now().Add(connectionDeadline))
	request, err := readWireRequest(connection)
	if err != nil {
		_ = writeWireResponse(connection, protocolFailure("", "invalid_frame"))
		return
	}
	if request.ProtocolVersion != ProtocolVersion {
		_ = writeWireResponse(connection, protocolFailure(request.RequestID, "protocol_mismatch"))
		return
	}
	if !capabilityMatches(request.Capability, server.descriptor.Capability) {
		_ = writeWireResponse(connection, protocolFailure(request.RequestID, "unauthenticated"))
		return
	}
	if request.Kind != "request" || request.RequestID == "" || !validRequestTarget(request.Target) {
		_ = writeWireResponse(connection, protocolFailure(request.RequestID, "invalid_request"))
		return
	}
	response := server.handle(request)
	if err := writeWireResponse(connection, response); err != nil {
		return
	}
}

func (server *Server) finishConnection(connection net.Conn) {
	_ = connection.Close()
	server.admission.Lock()
	delete(server.connections, connection)
	server.admission.Unlock()
	server.workers.Done()
}

func (server *Server) handle(frame wireRequest) wireResponse {
	request, err := http.NewRequestWithContext(
		server.runContext,
		frame.Method,
		"http://sluice.local"+frame.Target,
		bytes.NewReader(frame.Body),
	)
	if err != nil {
		return protocolFailure(frame.RequestID, "invalid_request")
	}
	request.Header = frame.Headers.Clone()
	request.RemoteAddr = "127.0.0.1:1"
	capture := newBoundedResponseCapture(MaxResponseBytes / 2)
	server.handler.ServeHTTP(capture, request)
	if capture.overflow {
		return protocolFailure(frame.RequestID, "response_too_large")
	}
	return wireResponse{
		ProtocolVersion: ProtocolVersion,
		Kind:            "response",
		RequestID:       frame.RequestID,
		StatusCode:      capture.statusCode(),
		Headers:         capture.header.Clone(),
		Body:            capture.body,
	}
}

// Close stops acceptance and waits for active bounded requests to drain.
func (server *Server) Close(ctx context.Context) error {
	server.closeOnce.Do(func() {
		server.admission.Lock()
		server.stopping = true
		close(server.closed)
		server.cancel()
		listenerErr := server.listener.Close()
		if errors.Is(listenerErr, net.ErrClosed) {
			listenerErr = nil
		}
		var cleanupErr error
		if server.cleanup != nil {
			cleanupErr = server.cleanup()
		}
		for connection := range server.connections {
			_ = connection.Close()
		}
		server.closeErr = errors.Join(listenerErr, cleanupErr)
		server.admission.Unlock()
	})
	done := make(chan struct{})
	go func() {
		server.workers.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for agent IPC shutdown: %w", ctx.Err())
	case <-done:
		if server.closeErr != nil {
			return fmt.Errorf("close agent IPC listener: %w", server.closeErr)
		}
		return nil
	}
}

func readWireRequest(input io.Reader) (wireRequest, error) {
	line, err := readFrame(input, MaxRequestBytes)
	if err != nil {
		return wireRequest{}, err
	}
	var request wireRequest
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return wireRequest{}, fmt.Errorf("decode agent request")
	}
	return request, nil
}

func writeWireResponse(output io.Writer, response wireResponse) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode agent response: %w", err)
	}
	if len(encoded) > MaxResponseBytes {
		return ErrFrameTooLarge
	}
	encoded = append(encoded, '\n')
	if _, err := output.Write(encoded); err != nil {
		return fmt.Errorf("write agent response: %w", err)
	}
	return nil
}

func readFrame(input io.Reader, maximum int) ([]byte, error) {
	reader := bufio.NewReaderSize(input, maximum+2)
	line, err := reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) || len(line) > maximum+1 {
		return nil, ErrFrameTooLarge
	}
	if err != nil {
		return nil, fmt.Errorf("read agent frame")
	}
	if len(line) == 1 {
		return nil, fmt.Errorf("agent frame is empty")
	}
	return line[:len(line)-1], nil
}

func protocolFailure(requestID, code string) wireResponse {
	return wireResponse{
		ProtocolVersion: ProtocolVersion,
		Kind:            "response",
		RequestID:       requestID,
		ErrorCode:       code,
	}
}

type boundedResponseCapture struct {
	header   http.Header
	body     []byte
	limit    int
	status   int
	overflow bool
}

func newBoundedResponseCapture(limit int) *boundedResponseCapture {
	return &boundedResponseCapture{header: make(http.Header), limit: limit}
}

func (capture *boundedResponseCapture) Header() http.Header {
	return capture.header
}

func (capture *boundedResponseCapture) WriteHeader(status int) {
	if capture.status == 0 {
		capture.status = status
	}
}

func (capture *boundedResponseCapture) Write(data []byte) (int, error) {
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	if len(data) > capture.limit-len(capture.body) {
		capture.overflow = true
		return 0, ErrFrameTooLarge
	}
	capture.body = append(capture.body, data...)
	return len(data), nil
}

func (capture *boundedResponseCapture) statusCode() int {
	if capture.status == 0 {
		return http.StatusOK
	}
	return capture.status
}
