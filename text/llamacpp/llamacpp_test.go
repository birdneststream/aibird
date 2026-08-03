package llamacpp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aibird/logger"
	"aibird/settings"
	"aibird/text"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})

	// Change to project root so text.GetPrompt can find text/prompts/lyrics.md
	_, filename, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..")
	_ = os.Chdir(projectRoot)

	m.Run()
}

func TestIsLlamaCppRunning_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	running, err := IsLlamaCppRunning(ts.URL, "")
	require.NoError(t, err)
	assert.True(t, running)
}

func TestIsLlamaCppRunning_NotOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	running, err := IsLlamaCppRunning(ts.URL, "")
	require.NoError(t, err)
	assert.False(t, running)
}

func TestIsLlamaCppRunning_ConnectionError(t *testing.T) {
	// Use a URL that will refuse connections
	running, err := IsLlamaCppRunning("http://127.0.0.1:1", "")
	require.NoError(t, err)
	assert.False(t, running)
}

func TestSingleRequest_Success(t *testing.T) {
	expectedContent := "Hello from llama.cpp"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		resp := LlamaCppResponse{
			Choices: []struct {
				Index        int             `json:"index"`
				Message      LlamaCppMessage `json:"message"`
				FinishReason string          `json:"finish_reason"`
			}{
				{
					Message: LlamaCppMessage{Content: expectedContent},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := settings.LlamaCppConfig{Url: ts.URL, Port: "", DefaultModel: "test-model"}
	result, err := SingleRequest("user message", "system prompt", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestSingleRequest_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := LlamaCppResponse{Choices: []struct {
			Index        int             `json:"index"`
			Message      LlamaCppMessage `json:"message"`
			FinishReason string          `json:"finish_reason"`
		}{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := settings.LlamaCppConfig{Url: ts.URL, Port: "", DefaultModel: "test-model"}
	_, err := SingleRequest("user message", "system prompt", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices returned")
}

func TestSingleRequest_EmptyContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := LlamaCppResponse{
			Choices: []struct {
				Index        int             `json:"index"`
				Message      LlamaCppMessage `json:"message"`
				FinishReason string          `json:"finish_reason"`
			}{
				{Message: LlamaCppMessage{Content: ""}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := settings.LlamaCppConfig{Url: ts.URL, Port: "", DefaultModel: "test-model"}
	_, err := SingleRequest("user message", "system prompt", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no content found")
}

func TestSingleRequest_HTTPError(t *testing.T) {
	// Server that returns an error response code without valid JSON
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer ts.Close()

	config := settings.LlamaCppConfig{Url: ts.URL, Port: "", DefaultModel: "test-model"}
	_, err := SingleRequest("user message", "system prompt", config)
	require.Error(t, err)
}

func TestEnhancePrompt_Success(t *testing.T) {
	expectedContent := "expanded prompt with vivid details"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := LlamaCppResponse{
			Choices: []struct {
				Index        int             `json:"index"`
				Message      LlamaCppMessage `json:"message"`
				FinishReason string          `json:"finish_reason"`
			}{
				{Message: LlamaCppMessage{Content: expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := settings.LlamaCppConfig{Url: ts.URL, Port: "", DefaultModel: "test-model"}
	result, err := EnhancePrompt("a cat sitting on a mat", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestGenerateLyrics_Success(t *testing.T) {
	expectedContent := "la la la, song lyrics here"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := LlamaCppResponse{
			Choices: []struct {
				Index        int             `json:"index"`
				Message      LlamaCppMessage `json:"message"`
				FinishReason string          `json:"finish_reason"`
			}{
				{Message: LlamaCppMessage{Content: expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := settings.LlamaCppConfig{Url: ts.URL, Port: "", DefaultModel: "test-model"}
	result, err := GenerateLyrics("love and peace", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestGenerateLyrics_MissingPromptFile(t *testing.T) {
	// When lyrics.md doesn't exist (simulate by using a bad config path isn't possible,
	// so we test the error path by checking that GetPrompt fails for a nonexistent file).
	// Since GenerateLyrics calls text.GetPrompt("lyrics.md") which reads from disk,
	// and lyrics.md exists in-repo, we test the error path at the GetPrompt level
	// by directly testing text.GetPrompt instead.
	_, err := text.GetPrompt("nonexistent_lyrics_file_xyz.md")
	require.Error(t, err)
}

// ChatRequest requires a fully constructed *state.State (with girc.Client, Network, User, etc.)
// which is impractical in unit tests. It is tested indirectly via the SingleRequest tests above
// which exercise the same HTTP call and response handling logic.

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no fence", "just some text", "just some text"},
		{"no fence multiline", "line1\nline2\nline3", "line1\nline2\nline3"},
		{"fenced no lang", "```\nsome content\n```", "some content"},
		{"fenced with lang", "```json\n{\"a\": 1}\n```", "{\"a\": 1}"},
		{"fenced single line content", "```\ncontent\n```", "content"},
		{"only opening fence", "```", ""},
		{"fence with inner backticks", "```\n VISUAL STYLE\nfoo\n```", "VISUAL STYLE\nfoo"},
		{"preserves newlines and brackets", "```\n[0s-5s] Shot 1\nAUDIO layer\n```", "[0s-5s] Shot 1\nAUDIO layer"},
		{"whitespace padded", "  ```\ncontent\n```  ", "content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripCodeFences(tt.in))
		})
	}
}

func TestEnhancePromptWithTemplate_LoadsH3(t *testing.T) {
	// Verify the H3 template is readable and carries its MiniMax H3 role
	// definition. Thinking is disabled at the model level, so the template no
	// longer carries a /no_think directive. EnhancePromptWithTemplate itself
	// needs a live llama.cpp server; we test the template load path (the only
	// pure part) at the GetPrompt level.
	prompt, err := text.GetPrompt("h3.md")
	require.NoError(t, err)
	require.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "MiniMax H3", "h3.md must define its MiniMax H3 role")
}
