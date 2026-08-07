package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// wsReadDeadline is intentionally unused as a fixed value: gorilla/websocket
// documents that "after a read has timed out, the websocket connection state is
// corrupt and all future reads will return an error", so a deadline would kill
// healthy connections during quiet periods (model loading, video sampling) and
// during binary-preview-frame bursts. Cancellation is handled entirely by the
// watcher goroutine closing the connection on ctx.Done(), which unblocks a
// pending ReadMessage without corrupting the connection. If dead-peer detection
// is ever needed, use SetPongHandler + a ping ticker and refresh the deadline
// on pong instead.

// httpClientTimeout is the default timeout for non-streaming HTTP calls
// (QueuePrompt, Interrupt, History, Free). View uses a separate timeout-free
// client so large output artifacts (videos) can stream for as long as the
// caller's context allows.
const httpClientTimeout = 30 * time.Second

// wsChannelBuffer sizes the buffered message channel returned by Connect. A
// non-trivial buffer prevents the WebSocket reader from blocking on a
// momentarily-slow consumer, which is important for progress-heavy workflows.
const wsChannelBuffer = 32

// Client is a minimal ComfyUI REST/WebSocket client. It is safe for concurrent
// use after construction. The ClientID is generated once and reused across
// every /prompt and /ws connection so ComfyUI can flush pending messages to the
// same client on reconnect.
type Client struct {
	baseURL    string       // e.g. "http://127.0.0.1:8188"
	wsURL      string       // e.g. "ws://127.0.0.1:8188"
	httpClient *http.Client // for JSON endpoints (QueuePrompt/Interrupt/History/Free)
	viewClient *http.Client // timeout-free, for streaming View output bytes
	clientID   string
	dialer     *websocket.Dialer // overridable for tests
}

// Option configures a Client at construction time.
type Option func(*Client)

// WithHTTPClient injects a custom *http.Client (useful for test transports).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithDialer injects a custom gorilla/websocket Dialer (useful for tests).
func WithDialer(d *websocket.Dialer) Option {
	return func(c *Client) { c.dialer = d }
}

// WithClientID pins the client id (useful for tests and deterministic logging).
func WithClientID(id string) Option {
	return func(c *Client) { c.clientID = id }
}

// New constructs a Client for the given ComfyUI host and port. The client id is
// a freshly-generated UUID; supply WithClientID to override.
func New(host string, port int, opts ...Option) *Client {
	base := fmt.Sprintf("%s:%d", host, port)
	c := &Client{
		baseURL:    "http://" + base,
		wsURL:      "ws://" + base,
		httpClient: &http.Client{Timeout: httpClientTimeout},
		viewClient: &http.Client{}, // Timeout: 0 => no overall timeout; ctx governs View
		clientID:   uuid.NewString(),
		dialer:     websocket.DefaultDialer,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ClientID returns the stable client identifier used for /prompt and /ws.
func (c *Client) ClientID() string { return c.clientID }

// QueuePromptRequest is the body of POST /prompt.
type QueuePromptRequest struct {
	Prompt   map[string]any `json:"prompt"`
	ClientID string         `json:"client_id"`
}

// QueuePromptResponse is the body returned by POST /prompt on success.
type QueuePromptResponse struct {
	PromptID   string         `json:"prompt_id"`
	Number     int            `json:"number"`
	NodeErrors map[string]any `json:"node_errors"`
}

// QueuePrompt submits a workflow graph for execution. On a non-2xx response the
// body (which ComfyUI formats as {"error": ..., "node_errors": {...}}) is read
// and wrapped into the returned error so callers can surface validation faults.
func (c *Client) QueuePrompt(ctx context.Context, graph map[string]any) (string, error) {
	body, err := json.Marshal(QueuePromptRequest{Prompt: graph, ClientID: c.clientID})
	if err != nil {
		return "", fmt.Errorf("comfyui queue prompt: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/prompt", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("comfyui queue prompt: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("comfyui queue prompt: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("comfyui /prompt rejected (status %d): %s", resp.StatusCode, truncateErrBody(respBody))
	}

	var out QueuePromptResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("comfyui /prompt: decode response: %w", err)
	}
	return out.PromptID, nil
}

// truncateErrBody keeps error messages readable when ComfyUI returns large
// node_errors blobs (they can include full tracebacks).
func truncateErrBody(b []byte) string {
	const maxLen = 800
	s := string(b)
	if len(s) > maxLen {
		return s[:maxLen] + "...(truncated)"
	}
	return s
}

// Connect dials the /ws endpoint and returns a channel that emits parsed
// WebSocket messages plus a stop function. The channel is closed when the reader
// exits, so consumers can `for msg := range ch`. The stop function is idempotent
// and safe to call from any goroutine; calling it closes the underlying
// connection which unblocks the reader's pending ReadMessage.
//
// Callers MUST either defer the stop function OR cancel the context that was
// passed in. If a consumer breaks out of the range loop without doing either,
// the reader goroutine and the WS connection will leak (the reader blocks on
// the channel send once its buffer fills).
//
// Cancellation is leak-free: a watcher goroutine closes the connection on
// ctx.Done(), which unblocks the reader; the reader exits and the watcher
// exits with it. There is intentionally NO read deadline: gorilla/websocket
// documents that a timed-out read corrupts the connection, which would kill
// healthy streams during the quiet periods that real generation produces
// (model loading, long video sampling) and during binary-preview-frame bursts.
//
// Note: ComfyUI broadcasts WS messages for ALL prompts on a connection, so
// consumers MUST filter by PromptID (returned from QueuePrompt) to avoid
// cross-talk between concurrent prompts. Binary frames (latent previews) are
// discarded silently.
func (c *Client) Connect(ctx context.Context) (msgCh <-chan Message, stop func(), err error) {
	u := c.wsURL + "/ws?clientId=" + url.QueryEscape(c.clientID)
	conn, resp, derr := c.dialer.DialContext(ctx, u, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if derr != nil {
		return nil, nil, fmt.Errorf("comfyui ws dial: %w", derr)
	}

	// stop closes stopCh and the connection exactly once. It is safe to call
	// from the consumer (defer), the reader (defer), and the watcher (on
	// ctx.Done). Closing stopCh unblocks the reader's send-select even when it
	// is stuck on a full buffer (the abandoned-consumer case); closing the conn
	// unblocks a pending ReadMessage.
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() {
			close(stopCh)
			_ = conn.Close()
		})
	}

	ch := make(chan Message, wsChannelBuffer)
	readerDone := make(chan struct{})

	// Reader goroutine: parses text frames and forwards messages until the
	// connection breaks, the context is canceled, or stop() is called (all
	// observed via the send select). Closing the channel here is what unblocks
	// consumers' `for range` loops. Binary frames (latent previews) are skipped
	// without attempting to parse them.
	go func() {
		defer close(ch)
		defer close(readerDone)
		defer stop()
		for {
			msgType, data, rerr := conn.ReadMessage()
			if rerr != nil {
				// The connection was closed (by the server, by the watcher on
				// ctx.Done, or by an explicit stop). Either way, we're done.
				return
			}
			if msgType != websocket.TextMessage {
				continue // binary frame (e.g. latent preview); ignore
			}
			msg, perr := parseMessage(data)
			if perr != nil {
				continue // skip malformed/unparseable frame, keep the stream alive
			}
			select {
			case ch <- msg:
			case <-stopCh:
				return
			}
		}
	}()

	// Watcher goroutine: triggers stop() on context cancellation so both a
	// pending ReadMessage and a full-buffer send-select unblock. Exits
	// immediately once the reader has finished (normal close), so it never leaks.
	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-readerDone:
		}
	}()

	return ch, stop, nil
}

// View fetches the bytes of a single output artifact. All three parameters are
// required by ComfyUI's /view endpoint and come from the OutputItem fields
// delivered in an "executed" WebSocket message (or via History).
//
// The returned ReadCloser is the live response body; callers MUST Close it.
// The body is not buffered in memory, so large videos stream. View uses a
// timeout-free *http.Client; the caller's context is the sole deadline.
func (c *Client) View(ctx context.Context, filename, subfolder, fileType string) (io.ReadCloser, error) {
	q := url.Values{}
	q.Set("filename", filename)
	q.Set("subfolder", subfolder)
	q.Set("type", fileType)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/view?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("comfyui view: request: %w", err)
	}
	resp, err := c.viewClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("comfyui view: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("comfyui /view rejected (status %d)", resp.StatusCode)
	}
	return resp.Body, nil
}

// Interrupt asks ComfyUI to stop the currently-executing prompt. It is a no-op
// (and returns no error) if no prompt is currently running; if the target
// prompt is still queued but not yet executing, this call will not remove it
// from the queue.
func (c *Client) Interrupt(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/interrupt", http.NoBody)
	if err != nil {
		return fmt.Errorf("comfyui interrupt: request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("comfyui interrupt: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("comfyui /interrupt rejected (status %d)", resp.StatusCode)
	}
	return nil
}

// History retrieves the execution record for a prompt id. The shape is
// {prompt_id: {..., "outputs": {node_id: {"images"|"gifs"|"audio": [...]}}}}.
// It is used as a fallback for output discovery when "executed" messages are
// missed (e.g. a brief WS reconnect).
func (c *Client) History(ctx context.Context, promptID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/history/"+url.PathEscape(promptID), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("comfyui history: request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("comfyui history: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // not finished yet, or evicted from history
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("comfyui /history rejected (status %d)", resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("comfyui /history: decode: %w", err)
	}
	return out, nil
}

// Free unloads models and/or releases VRAM on the ComfyUI server. This mirrors
// the existing freeVram helper (POST /free) that already ran independently of
// comfy2go; it is centralized here for reuse by the direct client path.
//
// Pass unloadModels and freeMemory to control each flag independently; in
// practice callers set both to true after a generation completes.
func (c *Client) Free(ctx context.Context, unloadModels, freeMemory bool) error {
	payload := map[string]bool{
		"unload_models": unloadModels,
		"free_memory":   freeMemory,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("comfyui free: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/free", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("comfyui free: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("comfyui free: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("comfyui /free rejected (status %d)", resp.StatusCode)
	}
	return nil
}

// SplitBaseURL separates a "scheme://host:port" base into its host and port for
// callers that need to pass them to New. It is a small convenience used by the
// comfyui package when bridging from the existing (host, port) config fields.
// Returns ("", 0, error) if the input cannot be parsed. Bracketed IPv6 literals
// ([::1]:8188) are supported via net.SplitHostPort.
func SplitBaseURL(base string) (host string, port int, err error) {
	base = strings.TrimSpace(base)
	for _, scheme := range []string{"http://", "https://"} {
		base = strings.TrimPrefix(base, scheme)
	}
	host, portStr, err := net.SplitHostPort(base)
	if err != nil {
		return "", 0, fmt.Errorf("comfyui base url missing or invalid port: %q: %w", base, err)
	}
	port, err = strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("comfyui base url invalid port: %q", portStr)
	}
	return host, port, nil
}
