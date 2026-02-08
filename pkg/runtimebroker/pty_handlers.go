package runtimebroker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ptone/scion-agent/pkg/wsprotocol"
)

// PTY endpoint configuration
const (
	ptyMaxDataSize = 32 * 1024 // 32KB max per message
)

var ptyUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // Auth is handled separately
	},
}

// handleAgentAttach handles direct WebSocket PTY connections.
// This is used when clients connect directly to the runtime broker.
// Route: GET /api/v1/agents/{id}/attach
func (s *Server) handleAgentAttach(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID := extractAgentIDFromAttachPath(r.URL.Path)
	if agentID == "" {
		BadRequest(w, "Invalid agent ID")
		return
	}

	// Verify WebSocket upgrade
	if !isPTYWebSocketUpgrade(r) {
		BadRequest(w, "WebSocket upgrade required")
		return
	}

	// Look up agent using List with filter
	agents, err := s.manager.List(ctx, map[string]string{"scion.name": agentID})
	if err != nil || len(agents) == 0 {
		NotFound(w, "Agent")
		return
	}

	agent := agents[0]

	// Check if agent has tmux support
	if agent.Labels == nil || agent.Labels["scion.tmux"] != "true" {
		Unprocessable(w, "Agent does not support attach")
		return
	}

	// Get container ID
	containerID := agent.Labels["scion.container.id"]
	if containerID == "" {
		containerID = agent.ID // Fall back to agent ID
	}

	// Upgrade to WebSocket
	conn, err := ptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed for agent", "agentID", agentID, "error", err)
		return
	}
	defer conn.Close()

	// Get terminal size from query params
	cols := 80
	rows := 24
	if c := r.URL.Query().Get("cols"); c != "" {
		fmt.Sscanf(c, "%d", &cols)
	}
	if rowStr := r.URL.Query().Get("rows"); rowStr != "" {
		fmt.Sscanf(rowStr, "%d", &rows)
	}

	slog.Info("Attach session started", "agentID", agentID, "containerID", containerID)

	// Start PTY session
	session := newLocalPTYSession(ctx, agentID, containerID, conn, cols, rows)
	if err := session.Run(); err != nil && err != io.EOF {
		slog.Error("Attach session error", "agentID", agentID, "error", err)
	}

	slog.Info("Attach session ended", "agentID", agentID)
}

// extractAgentIDFromAttachPath extracts agent ID from /api/v1/agents/{id}/attach
func extractAgentIDFromAttachPath(path string) string {
	const prefix = "/api/v1/agents/"
	const suffix = "/attach"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}

	path = strings.TrimPrefix(path, prefix)
	path = strings.TrimSuffix(path, suffix)
	return path
}

// isPTYWebSocketUpgrade checks if the request is a WebSocket upgrade.
func isPTYWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// LocalPTYSession manages a local PTY session attached to a container.
type LocalPTYSession struct {
	ctx         context.Context
	cancel      context.CancelFunc
	agentID     string
	containerID string
	conn        *websocket.Conn
	cols        int
	rows        int
	cmd         *exec.Cmd
	ptyMaster   *os.File
	ptySlave    *os.File
	writeMu     sync.Mutex
}

// newLocalPTYSession creates a new local PTY session.
func newLocalPTYSession(ctx context.Context, agentID, containerID string, conn *websocket.Conn, cols, rows int) *LocalPTYSession {
	ctx, cancel := context.WithCancel(ctx)
	return &LocalPTYSession{
		ctx:         ctx,
		cancel:      cancel,
		agentID:     agentID,
		containerID: containerID,
		conn:        conn,
		cols:        cols,
		rows:        rows,
	}
}

// Run starts the PTY session.
func (s *LocalPTYSession) Run() error {
	// Start docker exec with PTY
	if err := s.startDockerExec(); err != nil {
		return fmt.Errorf("failed to start docker exec: %w", err)
	}

	defer func() {
		if s.ptyMaster != nil {
			s.ptyMaster.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			s.cmd.Process.Kill()
			s.cmd.Wait()
		}
	}()

	errCh := make(chan error, 2)

	// Read from PTY, write to WebSocket
	go func() {
		errCh <- s.readFromPTY()
	}()

	// Read from WebSocket, write to PTY
	go func() {
		errCh <- s.readFromWebSocket()
	}()

	// Wait for either direction to fail
	err := <-errCh
	s.cancel()
	return err
}

// startDockerExec starts a docker exec session with tmux attach.
func (s *LocalPTYSession) startDockerExec() error {
	// For docker exec -it, we use os.Pipe and let docker handle PTY
	// Docker exec -it will allocate a PTY on its side

	// Create pipes for stdin/stdout
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Build docker exec command
	args := []string{
		"exec", "-i",
		"--user", "scion",
		s.containerID,
		"tmux", "attach-session", "-t", "scion",
	}

	s.cmd = exec.CommandContext(s.ctx, "docker", args...)
	s.cmd.Stdin = stdinReader
	s.cmd.Stdout = stdoutWriter
	s.cmd.Stderr = stdoutWriter

	if err := s.cmd.Start(); err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		stdoutReader.Close()
		stdoutWriter.Close()
		return fmt.Errorf("failed to start docker exec: %w", err)
	}

	// Close the ends we don't need
	stdinReader.Close()
	stdoutWriter.Close()

	s.ptyMaster = stdinWriter
	s.ptySlave = stdoutReader

	return nil
}

// readFromPTY reads data from the PTY and sends to WebSocket.
func (s *LocalPTYSession) readFromPTY() error {
	buf := make([]byte, ptyMaxDataSize)

	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		n, err := s.ptySlave.Read(buf)
		if err != nil {
			return err
		}

		if n > 0 {
			msg := wsprotocol.NewPTYDataMessage(buf[:n])
			if err := s.writeToWebSocket(msg); err != nil {
				return err
			}
		}
	}
}

// readFromWebSocket reads messages from WebSocket and writes to PTY.
func (s *LocalPTYSession) readFromWebSocket() error {
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return err
		}

		env, err := wsprotocol.ParseEnvelope(data)
		if err != nil {
			continue
		}

		switch env.Type {
		case wsprotocol.TypeData:
			var msg wsprotocol.PTYDataMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if _, err := s.ptyMaster.Write(msg.Data); err != nil {
				return err
			}

		case wsprotocol.TypeResize:
			var msg wsprotocol.PTYResizeMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			// Resize is handled by sending escape sequence to tmux
			// For now, log it
			slog.Debug("PTY Resize", "agentID", s.agentID, "cols", msg.Cols, "rows", msg.Rows)
		}
	}
}

// writeToWebSocket writes a message to the WebSocket connection.
func (s *LocalPTYSession) writeToWebSocket(v interface{}) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteJSON(v)
}

// StreamPTYHandler handles PTY streams coming through the control channel.
type StreamPTYHandler struct {
	client      *ControlChannelClient
	handler     *StreamHandler
	slug        string
	containerID string
	cols        int
	rows        int
	ptyMaster   *os.File
	ptySlave    *os.File
	cmd         *exec.Cmd
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewStreamPTYHandler creates a handler for a PTY stream from the control channel.
func NewStreamPTYHandler(client *ControlChannelClient, handler *StreamHandler, containerID string, cols, rows int) *StreamPTYHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &StreamPTYHandler{
		client:      client,
		handler:     handler,
		slug:        handler.slug,
		containerID: containerID,
		cols:        cols,
		rows:        rows,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Run starts the PTY stream handler.
func (h *StreamPTYHandler) Run() error {
	// Start docker exec with tmux attach
	if err := h.startDockerExec(); err != nil {
		return err
	}

	defer func() {
		if h.ptyMaster != nil {
			h.ptyMaster.Close()
		}
		if h.ptySlave != nil {
			h.ptySlave.Close()
		}
		if h.cmd != nil && h.cmd.Process != nil {
			h.cmd.Process.Kill()
			h.cmd.Wait()
		}
	}()

	errCh := make(chan error, 2)

	// Read from PTY, send to control channel
	go func() {
		errCh <- h.readFromPTY()
	}()

	// Read from control channel, write to PTY
	go func() {
		errCh <- h.readFromStream()
	}()

	err := <-errCh
	h.cancel()
	return err
}

// startDockerExec starts docker exec with tmux attach.
func (h *StreamPTYHandler) startDockerExec() error {
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	args := []string{
		"exec", "-i",
		"--user", "scion",
		h.containerID,
		"tmux", "attach-session", "-t", "scion",
	}

	h.cmd = exec.CommandContext(h.ctx, "docker", args...)
	h.cmd.Stdin = stdinReader
	h.cmd.Stdout = stdoutWriter
	h.cmd.Stderr = stdoutWriter

	if err := h.cmd.Start(); err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		stdoutReader.Close()
		stdoutWriter.Close()
		return fmt.Errorf("failed to start docker exec: %w", err)
	}

	stdinReader.Close()
	stdoutWriter.Close()

	h.ptyMaster = stdinWriter
	h.ptySlave = stdoutReader

	return nil
}

// readFromPTY reads from the PTY and sends to the control channel stream.
func (h *StreamPTYHandler) readFromPTY() error {
	buf := make([]byte, ptyMaxDataSize)

	for {
		select {
		case <-h.ctx.Done():
			return h.ctx.Err()
		case <-h.handler.closeCh:
			return io.EOF
		default:
		}

		n, err := h.ptySlave.Read(buf)
		if err != nil {
			return err
		}

		if n > 0 {
			if err := h.client.SendStreamData(h.handler.streamID, buf[:n]); err != nil {
				return err
			}
		}
	}
}

// readFromStream reads from the control channel stream and writes to PTY.
func (h *StreamPTYHandler) readFromStream() error {
	for {
		select {
		case <-h.ctx.Done():
			return h.ctx.Err()
		case <-h.handler.closeCh:
			return io.EOF
		case data := <-h.handler.dataCh:
			if _, err := h.ptyMaster.Write(data); err != nil {
				return err
			}
		}
	}
}

// Close stops the PTY handler.
func (h *StreamPTYHandler) Close() {
	h.cancel()
	if h.ptyMaster != nil {
		h.ptyMaster.Close()
	}
	if h.ptySlave != nil {
		h.ptySlave.Close()
	}
	if h.cmd != nil && h.cmd.Process != nil {
		h.cmd.Process.Kill()
	}
}

// handlePTYStreamWithAgent is called by the control channel to handle PTY streams.
func (c *ControlChannelClient) handlePTYStreamWithAgent(handler *StreamHandler, cols, rows int, containerID string) {
	ptyHandler := NewStreamPTYHandler(c, handler, containerID, cols, rows)
	if err := ptyHandler.Run(); err != nil && err != io.EOF {
		slog.Error("PTY stream error", "slug", handler.slug, "error", err)
	}
}
