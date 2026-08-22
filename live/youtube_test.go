package live

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractVideoID(t *testing.T) {
	const id = "dQw4w9WgXcQ"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full watch URL", "https://www.youtube.com/watch?v=" + id, id},
		{"watch URL with extra params", "https://www.youtube.com/watch?v=" + id + "&list=PL123&t=42", id},
		{"scheme-less watch URL", "www.youtube.com/watch?v=" + id, id},
		{"youtu.be short link", "https://youtu.be/" + id, id},
		{"youtu.be with timestamp", "https://youtu.be/" + id + "?t=123", id},
		{"scheme-less youtu.be", "youtu.be/" + id, id},
		{"live path", "https://www.youtube.com/live/" + id, id},
		{"shorts path", "https://www.youtube.com/shorts/" + id, id},
		{"embed path", "https://www.youtube.com/embed/" + id, id},
		{"v path", "https://www.youtube.com/v/" + id, id},
		{"mobile watch", "https://m.youtube.com/watch?v=" + id, id},
		{"music watch", "https://music.youtube.com/watch?v=" + id, id},
		{"whitespace trimmed", "  https://www.youtube.com/watch?v=" + id + "  ", id},
		{"uppercase host", "https://WWW.YouTube.com/watch?v=" + id, id},

		{"bare video ID rejected", id, ""},
		{"empty string", "", ""},
		{"only spaces", "   ", ""},
		{"other host", "https://vimeo.com/" + id, ""},
		{"not youtube host with v param", "https://evil.example.com/watch?v=" + id, ""},
		{"id too short", "https://www.youtube.com/watch?v=short", ""},
		{"id too long", "https://www.youtube.com/watch?v=" + id + "x", ""},
		{"invalid id chars", "https://www.youtube.com/watch?v=abc def!ghi", ""},
		{"youtube homepage", "https://www.youtube.com/", ""},
		{"deep non-video path", "https://www.youtube.com/channel/UCxyz", ""},
		{"youtu.be with no id", "https://youtu.be/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ExtractVideoID(tt.input))
		})
	}
}

// newCheckerServer spins up an httptest server answering videos.list-shaped
// requests with the given handler and returns a checker pointed at it plus a
// pointer to the recorded API keys.
func newCheckerServer(t *testing.T, handler http.HandlerFunc) (checker *YouTubeChecker, seenKeys *[]string) {
	t.Helper()

	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.URL.Query().Get("key"))
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	c := &YouTubeChecker{
		apiKey:  "test-key",
		BaseURL: server.URL,
		client:  server.Client(),
	}
	return c, &keys
}

func TestYouTubeChecker_IsLive(t *testing.T) {
	const id = "dQw4w9WgXcQ"

	tests := []struct {
		name        string
		body        string
		status      int
		wantLive    bool
		wantErr     bool
		keyAsserted bool
	}{
		{
			name:        "live broadcast",
			body:        `{"items":[{"id":"` + id + `","snippet":{"liveBroadcastContent":"live"}}]}`,
			status:      http.StatusOK,
			wantLive:    true,
			keyAsserted: true,
		},
		{
			name:        "upcoming broadcast counts as live",
			body:        `{"items":[{"snippet":{"liveBroadcastContent":"upcoming"}}]}`,
			status:      http.StatusOK,
			wantLive:    true,
			keyAsserted: true,
		},
		{
			name:        "ended via actualEndTime in top-level liveStreamingDetails",
			body:        `{"items":[{"snippet":{"liveBroadcastContent":"live"},"liveStreamingDetails":{"actualEndTime":"2026-08-21T00:00:00Z"}}]}`,
			status:      http.StatusOK,
			wantLive:    false,
			keyAsserted: true,
		},
		{
			name:        "ordinary video is not live",
			body:        `{"items":[{"snippet":{"liveBroadcastContent":"none"}}]}`,
			status:      http.StatusOK,
			wantLive:    false,
			keyAsserted: true,
		},
		{
			name:        "missing liveStreamingDetails is fine",
			body:        `{"items":[{"snippet":{"liveBroadcastContent":"none"}}]}`,
			status:      http.StatusOK,
			wantLive:    false,
			keyAsserted: true,
		},
		{
			name:        "empty items means finished not error",
			body:        `{"items":[]}`,
			status:      http.StatusOK,
			wantLive:    false,
			keyAsserted: true,
		},
		{
			// Critical: YouTube quota/key errors come back as valid JSON with
			// a non-200 status. That must surface as an error, never decode
			// to empty items and read as "stream ended".
			name:        "403 JSON error body is an error",
			body:        `{"error":{"code":403,"message":"quotaExceeded"}}`,
			status:      http.StatusForbidden,
			wantLive:    false,
			wantErr:     true,
			keyAsserted: true,
		},
		{
			name:        "400 bad request is an error",
			body:        `{"error":{"code":400,"message":"keyInvalid"}}`,
			status:      http.StatusBadRequest,
			wantLive:    false,
			wantErr:     true,
			keyAsserted: true,
		},
		{
			name:     "malformed JSON is an error",
			body:     `not json at all`,
			status:   http.StatusOK,
			wantLive: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, keys := newCheckerServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			})

			live, err := c.IsLive(context.Background(), id)

			if tt.wantErr {
				require.Error(t, err)
				assert.False(t, live)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantLive, live)
			}

			if tt.keyAsserted {
				require.NotEmpty(t, *keys, "request should have reached the server")
				assert.Equal(t, "test-key", (*keys)[0], "API key should be sent as the key param")
			}
		})
	}
}

func TestYouTubeChecker_IsLive_RequestCancelled(t *testing.T) {
	c, _ := newCheckerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// Handler never runs: the request is canceled before connecting.
		fmt.Fprint(w, `{}`)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.IsLive(ctx, "dQw4w9WgXcQ")
	assert.Error(t, err, "canceled context should produce an error, not a false 'finished'")
}

func TestNewYouTubeChecker_Defaults(t *testing.T) {
	c := NewYouTubeChecker("some-key")
	assert.Equal(t, defaultYouTubeAPIBase, c.BaseURL)
	assert.NotNil(t, c.client)
}
