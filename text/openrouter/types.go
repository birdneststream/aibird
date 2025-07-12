package openrouter

import "aibird/text"

type (
	OpenRouterRequestBody struct {
		Model    string         `json:"model"`
		Messages []text.Message `json:"messages"`
	}

	OpenRouterChoice struct {
		FinishReason string       `json:"finish_reason"`
		Message      text.Message `json:"message"`
	}

	OpenRouterResponse struct {
		ID      string             `json:"id"`
		Choices []OpenRouterChoice `json:"choices"`
		Model   string             `json:"model"`
	}

	OpenRouterConfig struct {
		Url          string `toml:"url"`
		ApiKey       string `toml:"apiKey"`
		DefaultModel string `toml:"defaultModel"`
	}
)
