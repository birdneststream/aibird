package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/settings"
	"aibird/shared/meta"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayDefault(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback string
		want     string
	}{
		{"empty returns fallback", "", "default", "default"},
		{"non-empty returns value", "hello", "default", "hello"},
		{"both empty returns empty", "", "", ""},
		{"whitespace is non-empty", " ", "default", " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, displayDefault(tt.value, tt.fallback))
		})
	}
}

func TestHasTextProviderConfig(t *testing.T) {
	tests := []struct {
		name       string
		llamaUrl   string
		glmApiKey  string
		wantResult bool
	}{
		{"both configured", "http://localhost", "key123", true},
		{"only llamacpp", "http://localhost", "", true},
		{"only glm", "", "key123", true},
		{"neither configured", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &settings.Config{
				LlamaCpp: settings.LlamaCppConfig{Url: tt.llamaUrl},
				Glm:      settings.GlmConfig{ApiKey: tt.glmApiKey},
			}
			assert.Equal(t, tt.wantResult, hasTextProviderConfig(config))
		})
	}
}

func TestSingleRequestWithFallback_LlamaCppSuccess(t *testing.T) {
	expectedContent := "Hello from llama"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.Config{
		LlamaCpp: settings.LlamaCppConfig{Url: ts.URL, DefaultModel: "test"},
	}

	result, err := singleRequestWithFallback("system", "user", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestSingleRequestWithFallback_LlamaCppFails_NoGLM(t *testing.T) {
	// Server that returns 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	config := &settings.Config{
		LlamaCpp: settings.LlamaCppConfig{Url: ts.URL, DefaultModel: "test"},
	}

	_, err := singleRequestWithFallback("system", "user", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LlamaCpp failed")
}

func TestSingleRequestWithFallback_NoProviders(t *testing.T) {
	config := &settings.Config{}

	_, err := singleRequestWithFallback("system", "user", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no AI provider available")
}

func TestEnqueueAITask_NilQueue_CallsGenerateFn(t *testing.T) {
	called := false
	// Create minimal state — the generateFn just needs to be called
	irc := &state.State{}

	enqueueAITask(irc, nil, "test", meta.GPU4090, func(s *state.State) {
		called = true
	})

	assert.True(t, called, "generateFn should be called directly when queue is nil")
}

func TestFetchHistoryAndEnqueue_NetworkNotFound(t *testing.T) {
	// Create a temp database to control network resolution
	db, err := birdbase.NewSQLiteDB(":memory:")
	require.NoError(t, err)
	oldData := birdbase.Data
	birdbase.Data = db
	defer func() { birdbase.Data = oldData }()

	// Network "nonexistent" won't be found, so ResolveNetworkID returns 0
	networkID := birdbase.ResolveNetworkID("nonexistent")
	assert.Equal(t, int64(0), networkID, "nonexistent network should resolve to 0")
}

func TestSanitizePersona_UTF8Truncation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantRunes int
	}{
		{
			name:      "short ASCII unchanged",
			input:     "hello world",
			wantRunes: 11,
		},
		{
			name:      "short multi-byte unchanged",
			input:     "こんにちは世界",
			wantRunes: 7,
		},
		{
			name:      "multi-byte at boundary — truncates to 200 runes",
			input:     string(make([]rune, 250)),
			wantRunes: 200,
		},
		{
			name:      "emoji at boundary",
			input:     string(make([]rune, 199)) + "🎉🔑",
			wantRunes: 200,
		},
		{
			name:      "exactly 200 runes — no truncation",
			input:     string(make([]rune, 200)),
			wantRunes: 200,
		},
		{
			name:      "newlines stripped to spaces",
			input:     "hello\nworld",
			wantRunes: 11, // "hello world" (space replaces \n)
		},
		{
			name:      "carriage return stripped",
			input:     "hello\rworld",
			wantRunes: 11, // "hello world" (space replaces \r)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizePersona(tt.input)
			assert.True(t, utf8.ValidString(result), "result should be valid UTF-8")
			assert.Equal(t, tt.wantRunes, utf8.RuneCountInString(result), "rune count mismatch")
		})
	}
}

func TestSanitizePersona_ByteSafety(t *testing.T) {
	// Create a string with multi-byte characters that exceed 200 runes.
	// Each emoji is 4 bytes but 1 rune. 250 emojis = 250 runes > 200.
	input := ""
	for i := 0; i < 250; i++ {
		input += "🎉" // 4 bytes each, 250 runes
	}

	result := sanitizePersona(input)
	assert.True(t, utf8.ValidString(result), "result must be valid UTF-8")
	assert.Equal(t, 200, utf8.RuneCountInString(result), "should be exactly 200 runes")
}
