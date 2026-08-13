package host

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifuryst/open-browser-use/internal/wire"
)

const NativeHostName = "com.ifuryst.open_browser_use.extension"

var DefaultSocketDir = defaultSocketDir()

type Config struct {
	SocketDir  string
	SocketPath string
	Logger     *slog.Logger
}

type Relay struct {
	config Config
	logger *slog.Logger

	stdin  io.Reader
	stdout io.Writer

	listener net.Listener

	mu        sync.Mutex
	clients   map[int64]*clientConn
	pending   map[string]pendingRequest
	nextID    atomic.Int64
	nextReqID atomic.Int64
	writeMu   sync.Mutex
}

type clientConn struct {
	id   int64
	conn net.Conn
	mu   sync.Mutex
}

type pendingRequest struct {
	client     *clientConn
	originalID any
}

type ActiveSocketRecord struct {
	SocketPath string    `json:"socketPath"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"startedAt"`
}

func NewRelay(config Config, stdin io.Reader, stdout io.Writer) *Relay {
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return &Relay{
		config:  config,
		logger:  logger,
		stdin:   stdin,
		stdout:  stdout,
		clients: make(map[int64]*clientConn),
		pending: make(map[string]pendingRequest),
	}
}

func (r *Relay) SocketPath() string {
	if r.config.SocketPath != "" {
		return r.config.SocketPath
	}
	return filepath.Join(socketDir(r.config.SocketDir), fmt.Sprintf("%s.sock", randomID()))
}

func (r *Relay) Serve(ctx context.Context) error {
	socketPath := r.SocketPath()
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket %q (path length %d): %w", socketPath, len(socketPath), err)
	}
	r.listener = listener
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return err
	}
	if err := WriteActiveSocketRecord(r.config.SocketDir, socketPath); err != nil {
		_ = listener.Close()
		return err
	}
	r.logger.Info("open-browser-use native host listening", "socket", socketPath)

	errCh := make(chan error, 2)
	go func() { errCh <- r.acceptLoop(ctx) }()
	go func() { errCh <- r.readExtensionLoop() }()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		r.closeClients()
	}()

	err = <-errCh
	_ = listener.Close()
	_ = os.Remove(socketPath)
	_ = RemoveActiveSocketRecord(r.config.SocketDir, socketPath)
	r.closeClients()
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (r *Relay) acceptLoop(ctx context.Context) error {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		id := r.nextID.Add(1)
		client := &clientConn{id: id, conn: conn}
		r.mu.Lock()
		r.clients[id] = client
		r.mu.Unlock()
		go r.readClientLoop(client)
	}
}

func (r *Relay) readClientLoop(client *clientConn) {
	defer func() {
		_ = client.conn.Close()
		r.mu.Lock()
		delete(r.clients, client.id)
		for requestID, pending := range r.pending {
			if pending.client == client {
				delete(r.pending, requestID)
			}
		}
		r.mu.Unlock()
	}()

	for {
		payload, err := wire.ReadFrame(client.conn, wire.DefaultMaxFrame)
		if err != nil {
			return
		}
		forwarded, hostID, originalID, err := r.rewriteClientRequest(payload)
		if err != nil {
			r.writeClientError(client, nil, err)
			continue
		}
		if hostID != "" {
			r.mu.Lock()
			r.pending[hostID] = pendingRequest{client: client, originalID: originalID}
			r.mu.Unlock()
		}
		if err := r.writeExtension(forwarded); err != nil {
			if hostID != "" {
				r.mu.Lock()
				delete(r.pending, hostID)
				r.mu.Unlock()
			}
			r.writeClientError(client, originalID, err)
		}
	}
}

func (r *Relay) readExtensionLoop() error {
	for {
		payload, err := wire.ReadFrame(r.stdin, wire.DefaultMaxFrame)
		if err != nil {
			return err
		}
		if err := r.dispatchExtensionPayload(payload); err != nil {
			r.logger.Warn("failed to dispatch extension payload", "error", err)
		}
	}
}

func (r *Relay) rewriteClientRequest(payload []byte) ([]byte, string, any, error) {
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		return nil, "", nil, err
	}
	id, hasID := message["id"]
	if !hasID {
		return payload, "", nil, nil
	}
	hostID := "obu:" + strconv.FormatInt(r.nextReqID.Add(1), 10)
	message["id"] = hostID
	forwarded, err := json.Marshal(message)
	return forwarded, hostID, id, err
}

func (r *Relay) dispatchExtensionPayload(payload []byte) error {
	var message map[string]any
	if err := json.Unmarshal(payload, &message); err != nil {
		return err
	}
	id, hasID := message["id"].(string)
	if hasID {
		r.mu.Lock()
		pending, ok := r.pending[id]
		if ok {
			delete(r.pending, id)
		}
		r.mu.Unlock()
		if ok {
			message["id"] = pending.originalID
			rewritten, err := json.Marshal(message)
			if err != nil {
				return err
			}
			return pending.client.write(rewritten)
		}
		// Every request id generated by this relay is namespaced with `obu:`.
		// If its client disconnected or timed out, the pending owner is gone.
		// Never broadcast that late response to unrelated clients: doing so can
		// make their next synchronous request consume another caller's result.
		if strings.HasPrefix(id, "obu:") {
			r.logger.Debug("dropping orphaned extension response", "request_id", id)
			return nil
		}
	}
	r.broadcast(payload)
	return nil
}

func (r *Relay) writeExtension(payload []byte) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return wire.WriteFrame(r.stdout, payload)
}

func (r *Relay) writeClientError(client *clientConn, id any, err error) {
	if id == nil {
		id = nil
	}
	_ = client.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32000,
			"message": err.Error(),
		},
	})
}

func (r *Relay) broadcast(payload []byte) {
	r.mu.Lock()
	clients := make([]*clientConn, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.mu.Unlock()
	for _, client := range clients {
		_ = client.write(payload)
	}
}

func (r *Relay) closeClients() {
	r.mu.Lock()
	clients := make([]*clientConn, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clients = make(map[int64]*clientConn)
	r.pending = make(map[string]pendingRequest)
	r.mu.Unlock()
	for _, client := range clients {
		_ = client.conn.Close()
	}
}

func (c *clientConn) writeJSON(value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.write(payload)
}

func (c *clientConn) write(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return wire.WriteFrame(c.conn, payload)
}

func socketDir(configured string) string {
	if configured != "" {
		return configured
	}
	return DefaultSocketDir
}

func defaultSocketDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "open-browser-use")
	}
	return "/tmp/open-browser-use"
}

func ActiveSocketRecordPath(configuredDir string) string {
	return filepath.Join(socketDir(configuredDir), "active.json")
}

func WriteActiveSocketRecord(configuredDir string, socketPath string) error {
	dir := socketDir(configuredDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	record := ActiveSocketRecord{
		SocketPath: socketPath,
		PID:        os.Getpid(),
		StartedAt:  time.Now().UTC(),
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	path := ActiveSocketRecordPath(configuredDir)
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func ReadActiveSocketRecord(configuredDir string) (ActiveSocketRecord, error) {
	payload, err := os.ReadFile(ActiveSocketRecordPath(configuredDir))
	if err != nil {
		return ActiveSocketRecord{}, err
	}
	var record ActiveSocketRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return ActiveSocketRecord{}, err
	}
	if record.SocketPath == "" {
		return ActiveSocketRecord{}, errors.New("active socket record has empty socketPath")
	}
	return record, nil
}

func RemoveActiveSocketRecord(configuredDir string, socketPath string) error {
	record, err := ReadActiveSocketRecord(configuredDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if record.SocketPath != socketPath {
		return nil
	}
	return os.Remove(ActiveSocketRecordPath(configuredDir))
}

func randomID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	)
}
