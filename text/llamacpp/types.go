package llamacpp

import "aibird/text"

type (
	// LlamaCppRequest represents an OpenAI-compatible chat completion request for llama.cpp.
	LlamaCppRequest struct {
		Model            string         `json:"model"`
		Messages         []text.Message `json:"messages"`
		Stream           bool           `json:"stream"`
		RepeatPenalty    float64        `json:"repeat_penalty,omitempty"`
		PresencePenalty  float64        `json:"presence_penalty,omitempty"`
		FrequencyPenalty float64        `json:"frequency_penalty,omitempty"`
	}

	// LlamaCppMessage extends text.Message with reasoning_content from llama.cpp
	// when --reasoning-format deepseek is used (the default).
	LlamaCppMessage struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content,omitempty"`
	}

	// LlamaCppResponse represents an OpenAI-compatible chat completion response from llama.cpp.
	LlamaCppResponse struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int             `json:"index"`
			Message      LlamaCppMessage `json:"message"`
			FinishReason string          `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
)
