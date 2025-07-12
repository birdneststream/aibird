package ollama

import "aibird/text"

type (
	OllamaRequestBody struct {
		Model     string         `json:"model"`
		Stream    bool           `json:"stream"`
		KeepAlive string         `json:"keep_alive"`
		Messages  []text.Message `json:"messages"`
		Options   OllamaOptions  `json:"options"`
	}

	OllamaOptions struct {
		RepeatPenalty    float64 `json:"repeat_penalty"`
		PresencePenalty  float64 `json:"presence_penalty"`
		FrequencyPenalty float64 `json:"frequency_penalty"`
	}

	OllamaResponse struct {
		Model              string       `json:"model"`
		CreatedAt          string       `json:"created_at"`
		Message            text.Message `json:"message"`
		Done               bool         `json:"done"`
		TotalDuration      int64        `json:"total_duration"`
		LoadDuration       int64        `json:"load_duration"`
		PromptEvalCount    int          `json:"prompt_eval_count"`
		PromptEvalDuration int64        `json:"prompt_eval_duration"`
		EvalCount          int          `json:"eval_count"`
		EvalDuration       int64        `json:"eval_duration"`
	}

	OllamaConfig struct {
		Url          string `toml:"url"`
		Port         string `toml:"port"`
		DefaultModel string `toml:"defaultModel"`
		ContextLimit int    `toml:"contextLimit"`
	}
)
