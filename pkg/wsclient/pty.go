// Package wsclient provides WebSocket client utilities for the CLI.
package wsclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/ptone/scion-agent/pkg/wsprotocol"
	"golang.org/x/term"
)

// PTYClientConfig holds configuration for the PTY client.
type PTYClientConfig struct {
	// Endpoint is the Hub or Runtime Broker URL.
	Endpoint string
	// Token is the Bearer token for authentication.
	Token string
	// Slug is the agent's URL-safe identifier.
	Slug string
	// Cols is the initial terminal width.
	Cols int
	// Rows is the initial terminal height.
	Rows int
}

// PTYClient manages a WebSocket PTY connection.
type PTYClient struct {
	config    PTYClientConfig
	conn      *websocket.Conn
	termState *term.State
	oldFd     int
	writeMu   sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewPTYClient creates a new PTY client.
func NewPTYClient(config PTYClientConfig) *PTYClient {
	return &PTYClient{
		config: config,
		oldFd:  int(os.Stdin.Fd()),
	}
}

// Connect establishes the WebSocket connection.
func (c *PTYClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Build WebSocket URL
	wsURL, err := c.buildWebSocketURL()
	if err != nil {
		return fmt.Errorf("failed to build URL: %w", err)
	}

	// Build headers
	headers := http.Header{}
	if c.config.Token != "" {
		headers.Set("Authorization", "Bearer "+c.config.Token)
	}

	// Connect
	dialer := websocket.Dialer{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
	}

	conn, resp, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		if resp != nil && resp.StatusCode >= 400 {
			return fmt.Errorf("connection failed with status %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("connection failed: %w", err)
	}

	c.conn = conn
	return nil
}

// buildWebSocketURL constructs the WebSocket URL.
func (c *PTYClient) buildWebSocketURL() (string, error) {
	u, err := url.Parse(c.config.Endpoint)
	if err != nil {
		return "", err
	}

	// Convert http(s) to ws(s)
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	case "wss", "ws":
		// Already WebSocket
	default:
		u.Scheme = "ws"
	}

	// Build path
	u.Path = fmt.Sprintf("/api/v1/agents/%s/pty", c.config.Slug)

	// Add query params for terminal size
	q := u.Query()
	if c.config.Cols > 0 {
		q.Set("cols", fmt.Sprintf("%d", c.config.Cols))
	}
	if c.config.Rows > 0 {
		q.Set("rows", fmt.Sprintf("%d", c.config.Rows))
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// Run starts the PTY session and blocks until it ends.
func (c *PTYClient) Run() error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Put terminal in raw mode
	if err := c.setupTerminal(); err != nil {
		return fmt.Errorf("failed to setup terminal: %w", err)
	}
	defer c.restoreTerminal()

	// Set up signal handler for resize
	go c.handleResize()

	// Set up signal handler for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			c.cancel()
		case <-c.ctx.Done():
		}
	}()

	errCh := make(chan error, 2)

	// Read from stdin, send to WebSocket
	go func() {
		errCh <- c.readFromStdin()
	}()

	// Read from WebSocket, write to stdout
	go func() {
		errCh <- c.readFromWebSocket()
	}()

	// Wait for either direction to fail
	err := <-errCh
	c.cancel()

	// Close connection
	c.writeMu.Lock()
	c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	c.writeMu.Unlock()

	return err
}

// setupTerminal puts the terminal in raw mode.
func (c *PTYClient) setupTerminal() error {
	if !term.IsTerminal(c.oldFd) {
		return nil // Not a terminal, no setup needed
	}

	state, err := term.MakeRaw(c.oldFd)
	if err != nil {
		return err
	}
	c.termState = state

	return nil
}

// restoreTerminal restores the terminal to its original state.
func (c *PTYClient) restoreTerminal() {
	if c.termState != nil {
		term.Restore(c.oldFd, c.termState)
	}
}

// handleResize handles terminal resize events.
func (c *PTYClient) handleResize() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-sigCh:
			cols, rows, err := term.GetSize(c.oldFd)
			if err != nil {
				continue
			}
			msg := wsprotocol.NewPTYResizeMessage(cols, rows)
			c.writeToWebSocket(msg)
		}
	}
}

// readFromStdin reads from stdin and sends to WebSocket.
func (c *PTYClient) readFromStdin() error {
	buf := make([]byte, 4096)

	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if n > 0 {
			msg := wsprotocol.NewPTYDataMessage(buf[:n])
			if err := c.writeToWebSocket(msg); err != nil {
				return err
			}
		}
	}
}

// readFromWebSocket reads from WebSocket and writes to stdout.
func (c *PTYClient) readFromWebSocket() error {
	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
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
			os.Stdout.Write(msg.Data)

		case wsprotocol.TypeError:
			var errMsg wsprotocol.ErrorMessage
			if err := json.Unmarshal(data, &errMsg); err != nil {
				continue
			}
			return fmt.Errorf("server error: %s - %s", errMsg.Code, errMsg.Message)
		}
	}
}

// writeToWebSocket writes a message to the WebSocket connection.
func (c *PTYClient) writeToWebSocket(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.conn.WriteJSON(v)
}

// Close closes the PTY client.
func (c *PTYClient) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.restoreTerminal()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// AttachToAgent is a convenience function that connects and runs a PTY session.
func AttachToAgent(ctx context.Context, endpoint, token, slug string) error {
	// Get terminal size
	cols, rows := 80, 24
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		c, r, err := term.GetSize(fd)
		if err == nil {
			cols, rows = c, r
		}
	}

	client := NewPTYClient(PTYClientConfig{
		Endpoint: endpoint,
		Token:    token,
		Slug:     slug,
		Cols:     cols,
		Rows:     rows,
	})

	if err := client.Connect(ctx); err != nil {
		return err
	}
	defer client.Close()

	return client.Run()
}

// BuildDirectAttachURL builds a URL for direct attachment to a runtime broker.
func BuildDirectAttachURL(hostEndpoint, slug string, cols, rows int) (string, error) {
	u, err := url.Parse(hostEndpoint)
	if err != nil {
		return "", err
	}

	// Convert to WebSocket scheme
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}

	u.Path = fmt.Sprintf("/api/v1/agents/%s/attach", slug)

	q := u.Query()
	q.Set("cols", fmt.Sprintf("%d", cols))
	q.Set("rows", fmt.Sprintf("%d", rows))
	u.RawQuery = q.Encode()

	return u.String(), nil
}

// IsWebSocketURL checks if a URL is a WebSocket URL.
func IsWebSocketURL(urlStr string) bool {
	return strings.HasPrefix(urlStr, "ws://") || strings.HasPrefix(urlStr, "wss://")
}
