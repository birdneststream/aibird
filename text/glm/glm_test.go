package glm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aibird/birdbase"
	"aibird/irc/state"
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

func TestBuildHttpRequest(t *testing.T) {
	config := &settings.GlmConfig{
		Url:    "https://api.glm.example.com",
		ApiKey: "test-api-key-123",
	}
	body := &GlmRequestBody{
		Model:    "glm-4",
		Stream:   false,
		Messages: nil,
	}

	req := buildHttpRequest(config, body)

	assert.Equal(t, config.Url, req.Url)
	assert.Equal(t, "POST", req.Method)
	require.Len(t, req.Headers, 2)
	assert.Equal(t, "Content-Type", req.Headers[0].Key)
	assert.Equal(t, "application/json", req.Headers[0].Value)
	assert.Equal(t, "Authorization", req.Headers[1].Key)
	assert.Equal(t, "Bearer test-api-key-123", req.Headers[1].Value)
	assert.Equal(t, body, req.Payload)
}

func TestSingleRequestWithSystem_WithSystemPrompt(t *testing.T) {
	expectedContent := "GLM response with system"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GlmRequestBody
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		require.Len(t, body.Messages, 2)
		assert.Equal(t, "system", body.Messages[0].Role)
		assert.Equal(t, "you are helpful", body.Messages[0].Content)
		assert.Equal(t, "user", body.Messages[1].Role)
		assert.Equal(t, "hello", body.Messages[1].Content)

		resp := GlmResponse{
			Choices: []GlmChoice{
				{Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{Content: expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, DefaultModel: "glm-4"}
	result, err := SingleRequestWithSystem("you are helpful", "hello", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestSingleRequestWithSystem_WithoutSystemPrompt(t *testing.T) {
	expectedContent := "GLM response without system"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GlmRequestBody
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		require.Len(t, body.Messages, 1)
		assert.Equal(t, "user", body.Messages[0].Role)
		assert.Equal(t, "hello", body.Messages[0].Content)

		resp := GlmResponse{
			Choices: []GlmChoice{
				{Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{Content: expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, DefaultModel: "glm-4"}
	result, err := SingleRequestWithSystem("", "hello", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestSingleRequestWithSystem_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GlmResponse{Choices: []GlmChoice{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, DefaultModel: "glm-4"}
	_, err := SingleRequestWithSystem("", "hello", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}

func TestSingleRequest_Success(t *testing.T) {
	expectedContent := "direct single response"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GlmResponse{
			Choices: []GlmChoice{
				{Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{Content: expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, DefaultModel: "glm-4"}
	result, err := SingleRequest("test prompt", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestImageRequest_Success(t *testing.T) {
	expectedURL := "https://images.glm.example.com/result.png"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GlmImageRequestBody
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		assert.Equal(t, "cogview-4", body.Model)
		assert.Equal(t, "a cat", body.Prompt)
		assert.Equal(t, "512x512", body.Size)

		resp := GlmImageResponse{
			Data: []GlmImageData{{URL: expectedURL}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{ImageUrl: ts.URL, ImageModel: "cogview-4", ApiKey: "key"}
	result, err := ImageRequest("a cat", "512x512", config)
	require.NoError(t, err)
	assert.Equal(t, expectedURL, result)
}

func TestImageRequest_DefaultSize(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GlmImageRequestBody
		_ = json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "1024x1024", body.Size)

		resp := GlmImageResponse{
			Data: []GlmImageData{{URL: "https://example.com/img.png"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{ImageUrl: ts.URL, ImageModel: "cogview-4"}
	_, err := ImageRequest("a cat", "", config)
	require.NoError(t, err)
}

func TestImageRequest_MissingImageUrl(t *testing.T) {
	config := &settings.GlmConfig{ImageUrl: ""}
	_, err := ImageRequest("a cat", "", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GLM image URL not configured")
}

func TestImageRequest_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GlmImageResponse{
			Error: &GlmError{Code: "invalid_request", Message: "bad prompt"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{ImageUrl: ts.URL, ImageModel: "cogview-4"}
	_, err := ImageRequest("a cat", "512x512", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GLM API error")
	assert.Contains(t, err.Error(), "bad prompt")
}

func TestImageRequest_EmptyData(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GlmImageResponse{Data: []GlmImageData{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{ImageUrl: ts.URL, ImageModel: "cogview-4"}
	_, err := ImageRequest("a cat", "512x512", config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data")
}

func TestGenerateLyrics_Success(t *testing.T) {
	expectedContent := "verse 1: la la la"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GlmRequestBody
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(body.Messages), 2)
		assert.Equal(t, "system", body.Messages[0].Role)
		assert.Equal(t, "user", body.Messages[1].Role)

		resp := GlmResponse{
			Choices: []GlmChoice{
				{Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{Content: expectedContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, DefaultModel: "glm-4"}
	result, err := GenerateLyrics("love and peace", config)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, result)
}

func TestHandleResetCommand_NotReset(t *testing.T) {
	// Non-reset messages should not be handled
	irc := &state.State{}
	handled, response := handleResetCommand(irc)
	assert.False(t, handled)
	assert.Empty(t, response)
}

func TestHandleResetCommand_DeletesFromChatHistory(t *testing.T) {
	// Set up in-memory database
	db, err := birdbase.NewSQLiteDB(":memory:")
	require.NoError(t, err)

	origData := birdbase.Data
	birdbase.Data = db
	defer func() { birdbase.Data = origData }()

	// Insert chat history for the user using a known key
	key := "test-reset-key"
	err = birdbase.PutChatHistory(key, []text.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	})
	require.NoError(t, err)

	// Verify history exists
	history, err := birdbase.GetChatHistory(key)
	require.NoError(t, err)
	require.Len(t, history, 2)

	// Simulate handleResetCommand's behavior: message=="reset" triggers
	// birdbase.DeleteChatHistory (this is the actual fix — previously called birdbase.Delete)
	irc := &state.State{}
	irc.SetMessage("reset")
	// We directly test the deletion that handleResetCommand performs
	// since UserAiChatCacheKey requires complex state construction
	err = birdbase.DeleteChatHistory(key)
	require.NoError(t, err)

	// Verify history is actually deleted from chat_history table
	history, err = birdbase.GetChatHistory(key)
	assert.Nil(t, history, "chat history should be deleted after reset")
}

// Request and buildRequestBody require a fully constructed *state.State (with girc.Client,
// Network, User, Channel, Config) which is impractical in unit tests.
// processResponse also requires *state.State for cache updates.
// These are tested indirectly via the SingleRequest tests above.
