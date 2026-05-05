package participant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aibird/settings"
	"aibird/text"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateParticipantMessage_Success(t *testing.T) {
	expectedResponse := "Hello there, how's it going?"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request structure
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req ParticipantRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-model", req.Model)
		assert.False(t, req.Stream)
		assert.Greater(t, len(req.Messages), 0)

		// First message should be system prompt
		assert.Equal(t, "system", req.Messages[0].Role)

		resp := ParticipantResponse{
			Choices: []ParticipantChoice{
				{Message: text.Message{Content: expectedResponse}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, ApiKey: "test-key", DefaultModel: "test-model"}
	ctx := &MessageContext{
		Type:            modeReactive,
		TimeOfDay:       "morning",
		ChannelActivity: "active",
		PersonalityMode: "friendly",
		RecentMessages:  []string{"user1: hey everyone"},
	}

	result, err := GenerateParticipantMessage(ctx, config)
	require.NoError(t, err)
	assert.Equal(t, expectedResponse, result)
}

func TestGenerateParticipantMessage_EmptyChoices(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ParticipantResponse{Choices: []ParticipantChoice{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, ApiKey: "test-key", DefaultModel: "test-model"}
	ctx := &MessageContext{Type: modeReactive, PersonalityMode: "friendly"}

	_, err := GenerateParticipantMessage(ctx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no response choices")
}

func TestGenerateParticipantMessage_Unconfigured(t *testing.T) {
	config := &settings.GlmConfig{Url: "", ApiKey: ""}
	ctx := &MessageContext{Type: modeReactive}

	_, err := GenerateParticipantMessage(ctx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGenerateParticipantMessage_APIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	config := &settings.GlmConfig{Url: ts.URL, ApiKey: "test-key", DefaultModel: "test-model"}
	ctx := &MessageContext{Type: modeReactive, PersonalityMode: "friendly"}

	_, err := GenerateParticipantMessage(ctx, config)
	require.Error(t, err)
}

func TestCleanParticipantResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"basic text", "Hello there", "Hello there"},
		{"removes asterisks", "*waves* hello", "waves hello"},
		{"removes AI phrases", "As an AI I think this is cool", "I think this is cool"},
		{"removes quotes around entire message", `"Hello world"`, "Hello world"},
		{"trims whitespace", "  hello  ", "hello"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cleanParticipantResponse(tt.input))
		})
	}
}
