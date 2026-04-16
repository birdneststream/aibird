package glm

import "aibird/text"

type (
	GlmThinking struct {
		Type string `json:"type"`
	}

	GlmRequestBody struct {
		Model    string         `json:"model"`
		Messages []text.Message `json:"messages"`
		Stream   bool           `json:"stream"`
		Thinking *GlmThinking   `json:"thinking,omitempty"`
	}

	GlmChoice struct {
		FinishReason string       `json:"finish_reason"`
		Message      text.Message `json:"message"`
	}

	GlmResponse struct {
		ID      string      `json:"id"`
		Choices []GlmChoice `json:"choices"`
		Model   string      `json:"model"`
	}

	// Image generation types
	GlmImageRequestBody struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Size   string `json:"size,omitempty"`
	}

	GlmImageData struct {
		URL string `json:"url"`
	}

	GlmImageResponse struct {
		Created int64          `json:"created"`
		Data    []GlmImageData `json:"data"`
		// Error fields for API errors
		Error *GlmError `json:"error,omitempty"`
	}

	GlmError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
)
