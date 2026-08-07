package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upgrader is shared by the fake WS server.
var upgrader = websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

// fakeServer is a minimal ComfyUI stand-in for deterministic tests.
type fakeServer struct {
	mu sync.Mutex
	// /prompt behavior
	promptStatus   int
	promptRespBody string
	lastPromptBody []byte

	// /view behavior
	viewStatus int
	viewBytes  []byte

	// /interrupt behavior
	interruptCalls int

	// ws behavior
	wsFrames []string // scripted text frames to send on connect
	wsClosed bool

	// endpoints registered
	server *httptest.Server
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{
		promptStatus:   http.StatusOK,
		promptRespBody: `{"prompt_id":"prompt-123","number":1,"node_errors":{}}`,
		viewStatus:     http.StatusOK,
		viewBytes:      []byte("PNG-BYTES"),
	}
	mux := http.NewServeMux()

	mux.HandleFunc("/prompt", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.lastPromptBody = body
		st := f.promptStatus
		resp := f.promptRespBody
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_, _ = fmt.Fprint(w, resp)
	})

	mux.HandleFunc("/view", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		st := f.viewStatus
		b := f.viewBytes
		f.mu.Unlock()
		if st != http.StatusOK {
			w.WriteHeader(st)
			return
		}
		w.WriteHeader(st)
		_, _ = w.Write(b)
	})

	mux.HandleFunc("/interrupt", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.interruptCalls++
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/free", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/history/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"prompt-123":{"outputs":{"9":{"images":[{"filename":"out.png","subfolder":"","type":"output"}]}}}}`)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("ws upgrade: %v", err)
			return
		}
		f.mu.Lock()
		frames := append([]string{}, f.wsFrames...)
		f.mu.Unlock()
		for _, fr := range frames {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(fr)); err != nil {
				_ = conn.Close()
				return
			}
		}
		// Block until the client disconnects (so frames are delivered first).
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				f.mu.Lock()
				f.wsClosed = true
				f.mu.Unlock()
				return
			}
		}
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// clientFor builds a Client pointed at the fake server. The http and ws bases
// are derived from the test server URL.
func (f *fakeServer) clientFor(t *testing.T, opts ...Option) *Client {
	t.Helper()
	addr := f.server.Listener.Addr().String() // "127.0.0.1:PORT"
	idx := strings.LastIndex(addr, ":")
	hostOnly := addr[:idx]
	portStr := addr[idx+1:]
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	// Override dialer so it points at ws://host:port (test server).
	dialer := &websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	allOpts := append([]Option{WithClientID("test-client-id"), WithDialer(dialer)}, opts...)
	return New(hostOnly, port, allOpts...)
}

func TestNew_GeneratesClientID(t *testing.T) {
	c := New("127.0.0.1", 8188)
	assert.NotEmpty(t, c.ClientID(), "client id must be generated")
	assert.Contains(t, c.baseURL, "127.0.0.1:8188")
	assert.Contains(t, c.wsURL, "ws://127.0.0.1:8188")
}

func TestNew_OptionsApply(t *testing.T) {
	custom := &http.Client{}
	c := New("h", 1, WithHTTPClient(custom), WithClientID("fixed-id"))
	assert.Equal(t, custom, c.httpClient)
	assert.Equal(t, "fixed-id", c.ClientID())
}

func TestQueuePrompt_Success(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)

	graph := map[string]any{"3": map[string]any{"class_type": "KSampler"}}
	id, err := c.QueuePrompt(context.Background(), graph)
	require.NoError(t, err)
	assert.Equal(t, "prompt-123", id)

	// Body must contain the client_id and the graph.
	var sent QueuePromptRequest
	require.NoError(t, json.Unmarshal(f.lastPromptBody, &sent))
	assert.Equal(t, "test-client-id", sent.ClientID)
	n3, ok := sent.Prompt["3"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "KSampler", n3["class_type"])
}

func TestQueuePrompt_RejectsOn400AndWrapsBody(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	f.promptStatus = http.StatusBadRequest
	f.promptRespBody = `{"error":"prompt outputs failed validation","node_errors":{"3":{"errors":["missing seed"]}}}`
	f.mu.Unlock()

	c := f.clientFor(t)
	_, err := c.QueuePrompt(context.Background(), map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
	assert.Contains(t, err.Error(), "validation")
}

func TestQueuePrompt_ContextCancel(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before sending
	_, err := c.QueuePrompt(ctx, map[string]any{})
	require.Error(t, err)
}

func TestView_ReturnsBytes(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	f.viewBytes = []byte("hello-image")
	f.mu.Unlock()
	c := f.clientFor(t)

	rc, err := c.View(context.Background(), "x.png", "", "output")
	require.NoError(t, err)
	defer rc.Close()
	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "hello-image", string(b))
}

func TestView_ErrorOnNon200(t *testing.T) {
	f := newFakeServer(t)
	f.mu.Lock()
	f.viewStatus = http.StatusNotFound
	f.mu.Unlock()
	c := f.clientFor(t)
	_, err := c.View(context.Background(), "missing.png", "", "output")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestHistory_ReturnsOutputs(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)
	out, err := c.History(context.Background(), "prompt-123")
	require.NoError(t, err)
	require.NotEmpty(t, out)
	root, ok := out["prompt-123"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, root, "outputs")
}

func TestInterrupt_NoError(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)
	require.NoError(t, c.Interrupt(context.Background()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.Equal(t, 1, f.interruptCalls)
}

func TestFree_NoError(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)
	require.NoError(t, c.Free(context.Background(), true, true))
}

func TestParseMessage_AllKnownTypes(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		check func(t *testing.T, m Message)
	}{
		{
			name: "execution_start",
			raw:  `{"type":"execution_start","data":{"prompt_id":"p1"}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, TypeExecutionStart, m.Type)
				assert.Equal(t, "p1", m.PromptID)
			},
		},
		{
			name: "executing_with_node",
			raw:  `{"type":"executing","data":{"prompt_id":"p1","node":"9"}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, "9", m.Node)
				assert.False(t, m.ExecutingDone)
			},
		},
		{
			name: "executing_done_null_node",
			raw:  `{"type":"executing","data":{"prompt_id":"p1","node":null}}`,
			check: func(t *testing.T, m Message) {
				assert.True(t, m.ExecutingDone)
				assert.Empty(t, m.Node)
			},
		},
		{
			name: "progress",
			raw:  `{"type":"progress","data":{"value":3,"max":10,"prompt_id":"p1","node":"3"}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, 3, m.Value)
				assert.Equal(t, 10, m.Max)
				assert.Equal(t, "p1", m.PromptID)
				assert.Equal(t, "3", m.Node)
			},
		},
		{
			name: "progress_state_running",
			raw:  `{"type":"progress_state","data":{"prompt_id":"p1","nodes":{"4":{"value":1.0,"max":1.0,"state":"finished","node_id":"4"},"3":{"value":2,"max":10,"state":"running","node_id":"3"}}}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, "p1", m.PromptID)
				require.Len(t, m.ProgressState, 2)
				assert.True(t, m.IsGenerating(), "a running node means generating")
				running := m.RunningNodeIDs()
				assert.ElementsMatch(t, []string{"3"}, running)
			},
		},
		{
			name: "progress_state_all_finished",
			raw:  `{"type":"progress_state","data":{"prompt_id":"p1","nodes":{"4":{"value":1.0,"max":1.0,"state":"finished","node_id":"4"},"9":{"value":1.0,"max":1.0,"state":"finished","node_id":"9"}}}}`,
			check: func(t *testing.T, m Message) {
				assert.False(t, m.IsGenerating(), "no running node means not generating")
				assert.Empty(t, m.RunningNodeIDs())
			},
		},
		{
			name: "executed_images",
			raw:  `{"type":"executed","data":{"prompt_id":"p1","node":"9","output":{"images":[{"filename":"a.png","subfolder":"","type":"output"}]}}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, "9", m.Node)
				require.Contains(t, m.Output, "images")
				require.Len(t, m.Output["images"], 1)
				assert.Equal(t, "a.png", m.Output["images"][0].Filename)
				assert.Equal(t, "output", m.Output["images"][0].Type)
			},
		},
		{
			name: "execution_error",
			raw:  `{"type":"execution_error","data":{"prompt_id":"p1","exception_type":"ValueError","exception_message":"bad seed"}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, "ValueError", m.ExceptionType)
				assert.Equal(t, "bad seed", m.ExceptionMessage)
				assert.True(t, m.IsTerminal())
			},
		},
		{
			name: "execution_success",
			raw:  `{"type":"execution_success","data":{"prompt_id":"p1"}}`,
			check: func(t *testing.T, m Message) {
				assert.True(t, m.IsTerminal())
			},
		},
		{
			name: "unknown_type_preserved",
			raw:  `{"type":"some_new_comfyui_event","data":{"prompt_id":"p1","weird":true}}`,
			check: func(t *testing.T, m Message) {
				assert.Equal(t, "some_new_comfyui_event", m.Type)
				assert.False(t, m.IsTerminal())
				assert.Contains(t, string(m.Raw), "weird")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseMessage([]byte(tc.raw))
			require.NoError(t, err)
			tc.check(t, m)
		})
	}
}

func TestParseMessage_InvalidEnvelope(t *testing.T) {
	_, err := parseMessage([]byte("not json"))
	require.Error(t, err)
}

func TestConnect_StreamAndTerminal(t *testing.T) {
	frames := []string{
		`{"type":"execution_start","data":{"prompt_id":"prompt-123"}}`,
		`{"type":"progress","data":{"value":1,"max":3}}`,
		`{"type":"executing","data":{"prompt_id":"prompt-123","node":"9"}}`,
		`{"type":"executed","data":{"prompt_id":"prompt-123","node":"9","output":{"images":[{"filename":"out.png","subfolder":"","type":"output"}]}}}`,
		`{"type":"execution_success","data":{"prompt_id":"prompt-123"}}`,
	}
	f := newFakeServer(t)
	f.mu.Lock()
	f.wsFrames = frames
	f.mu.Unlock()
	c := f.clientFor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	var msgs []Message
	for m := range ch {
		msgs = append(msgs, m)
		if m.IsTerminal() {
			stop() // stop the reader; channel closes
		}
	}
	require.NotEmpty(t, msgs)
	assert.Equal(t, "execution_success", msgs[len(msgs)-1].Type)
}

func TestConnect_FiltersForeignPromptIDsAtConsumer(t *testing.T) {
	// ComfyUI broadcasts messages for every prompt on the connection. The
	// consumer (not Connect) must filter by PromptID. This test documents that
	// contract: the channel delivers ALL messages and the caller is responsible
	// for filtering.
	frames := []string{
		`{"type":"progress","data":{"value":1,"max":2}}`, // no prompt_id, foreign
		`{"type":"execution_success","data":{"prompt_id":"mine"}}`,
	}
	f := newFakeServer(t)
	f.mu.Lock()
	f.wsFrames = frames
	f.mu.Unlock()
	c := f.clientFor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	sawProgress, sawSuccess := false, false
	for m := range ch {
		switch m.Type {
		case TypeProgress:
			sawProgress = true
		case TypeExecutionSuccess:
			sawSuccess = true
			stop()
		}
	}
	assert.True(t, sawProgress, "consumer sees all messages regardless of prompt_id")
	assert.True(t, sawSuccess)
}

func TestConnect_QuietLinkStaysOpenUntilStop(t *testing.T) {
	// Regression: a previous rolling-read-deadline design corrupted the
	// connection during quiet periods and closed the channel mid-stream. This
	// test proves a healthy-but-quiet connection delivers frames sent well
	// after a gap, as long as the context is alive.
	//
	// We script: send a frame, pause, send a terminal frame. Without the
	// stop()/ctx path driving shutdown, both frames must be delivered.
	frames := []string{
		`{"type":"executing","data":{"prompt_id":"p","node":"1"}}`,
		`{"type":"execution_success","data":{"prompt_id":"p"}}`,
	}
	f := newFakeServer(t)
	// Custom /ws handler that inserts a gap between frames.
	f.server.Close()
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			conn.WriteMessage(websocket.TextMessage, []byte(frames[0]))
			time.Sleep(300 * time.Millisecond) // quiet period (would have tripped a 5s+ design under load)
			conn.WriteMessage(websocket.TextMessage, []byte(frames[1]))
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	c := f.clientFor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	got := []string{}
	for m := range ch {
		got = append(got, m.Type)
		if m.IsTerminal() {
			stop()
		}
	}
	assert.Equal(t, []string{TypeExecuting, TypeExecutionSuccess}, got, "both frames across the quiet gap must arrive")
}

func TestConnect_BinaryFramesIgnored(t *testing.T) {
	// ComfyUI streams binary latent-preview frames. They must not corrupt the
	// stream or be parsed as text.
	f := newFakeServer(t)
	f.server.Close()
	frames := [][]byte{
		[]byte(`{"type":"executing","data":{"prompt_id":"p","node":"1"}}`),
	}
	// A binary preview frame is any non-text bytes; the server writes it raw.
	preview := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}
	terminal := []byte(`{"type":"execution_success","data":{"prompt_id":"p"}}`)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.WriteMessage(websocket.TextMessage, frames[0])
		conn.WriteMessage(websocket.BinaryMessage, preview) // binary frame: ignored
		conn.WriteMessage(websocket.TextMessage, terminal)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(f.server.Close)
	c := f.clientFor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	var got []string
	for m := range ch {
		got = append(got, m.Type)
		if m.IsTerminal() {
			stop()
		}
	}
	assert.Equal(t, []string{TypeExecuting, TypeExecutionSuccess}, got, "binary frame must be skipped, not parsed")
}

func TestConnect_PreCanceledContextErrors(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := c.Connect(ctx)
	require.Error(t, err)
}

func TestConnect_ContextCancelStopsReader_NoLeak(t *testing.T) {
	// Server stays open (no frames) so the only way the reader exits is via
	// context cancellation.
	f := newFakeServer(t)
	c := f.clientFor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Allow the reader to spin up before sampling the baseline.
	require.Eventually(t, func() bool { return true }, 50*time.Millisecond, 5*time.Millisecond)
	before := runtime.NumGoroutine()
	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	// Drain until closed (reader exits on ctx deadline -> watcher closes conn
	// -> reader returns -> channel closes).
	for range ch {
	}
	// Give the watcher goroutine a moment to observe readerDone and exit.
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= before+1
	}, time.Second, 20*time.Millisecond, "goroutine leak detected")
}

func TestConnect_StopReleasesAbandonedConsumer(t *testing.T) {
	// Regression: if a consumer breaks out of the range loop WITHOUT calling
	// stop() or canceling ctx, the reader blocks on the send once the buffer
	// fills, leaking a goroutine + WS connection. The documented contract is
	// "callers MUST defer stop()". This test verifies stop() releases both even
	// when the consumer abandons the loop early with a still-alive ctx.
	f := newFakeServer(t)
	// Flood more than wsChannelBuffer (32) frames so the reader's send would
	// block if stop() weren't called.
	frames := make([]string, 64)
	for i := range frames {
		frames[i] = `{"type":"progress","data":{"value":1,"max":2,"prompt_id":"p"}}`
	}
	f.mu.Lock()
	f.wsFrames = frames
	f.mu.Unlock()
	c := f.clientFor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)

	// Read exactly one message then abandon (do NOT cancel ctx).
	<-ch
	stop() // the mandatory release
	// The reader exits on stop(); its buffer still holds up to 32 messages,
	// so drain them and assert the channel closes promptly (no leak).
	closed := make(chan struct{})
	go func() {
		for range ch {
		}
		close(closed)
	}()
	select {
	case <-closed:
		// success: reader exited and channel closed
	case <-time.After(2 * time.Second):
		t.Fatal("channel did not close after stop(); reader/conn leaked")
	}
}

func TestConnect_InterruptCalledOnCancel(t *testing.T) {
	f := newFakeServer(t)
	c := f.clientFor(t)

	ctx, cancel := context.WithCancel(context.Background())
	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	cancel() // cancel immediately
	for range ch {
	}
	// Interrupt is a separate explicit call the execute loop performs on ctx.Done.
	require.NoError(t, c.Interrupt(context.Background()))
	f.mu.Lock()
	defer f.mu.Unlock()
	assert.GreaterOrEqual(t, f.interruptCalls, 1)
}

func TestSplitBaseURL(t *testing.T) {
	cases := []struct {
		in      string
		host    string
		port    int
		wantErr bool
	}{
		{"http://127.0.0.1:8188", "127.0.0.1", 8188, false},
		{"https://example.com:443", "example.com", 443, false},
		{"127.0.0.1:9000", "127.0.0.1", 9000, false},
		{"[::1]:8188", "::1", 8188, false},
		{"127.0.0.1", "", 0, true},
		{"127.0.0.1:notaport", "", 0, true},
		{"127.0.0.1:8188abc", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			h, p, err := SplitBaseURL(tc.in)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.host, h)
			assert.Equal(t, tc.port, p)
		})
	}
}

func TestMessageIsTerminal(t *testing.T) {
	assert.True(t, (Message{Type: TypeExecutionSuccess}).IsTerminal())
	assert.True(t, (Message{Type: TypeExecutionError}).IsTerminal())
	assert.True(t, (Message{Type: TypeExecutionInterrupted}).IsTerminal())
	assert.False(t, (Message{Type: TypeProgress}).IsTerminal())
}

func TestIsGenerating_NonProgressStateIsFalse(t *testing.T) {
	assert.False(t, (Message{Type: TypeProgress}).IsGenerating())
	assert.False(t, (Message{Type: TypeProgressState}).IsGenerating(), "nil ProgressState is not generating")
}

func TestUnknownType_ExtractsPromptID(t *testing.T) {
	// Unknown future message type should still surface a prompt_id if present,
	// so strict prompt_id filtering keeps working as ComfyUI evolves.
	m, err := parseMessage([]byte(`{"type":"future_event_v2","data":{"prompt_id":"p1","x":42}}`))
	require.NoError(t, err)
	assert.Equal(t, "future_event_v2", m.Type)
	assert.Equal(t, "p1", m.PromptID)
	assert.False(t, m.IsTerminal())
}
