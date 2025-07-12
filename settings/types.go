package settings

import (
	"aibird/irc/networks"
	"aibird/logger"
)

type (
	Config struct {
		Networks   map[string]networks.Network `toml:"networks" validate:"required,min=1"`
		AiBird     AiBird                      `toml:"aibird" validate:"required"`
		OpenRouter OpenRouterConfig            `toml:"openrouter" validate:"required"`
		Gemini     GeminiConfig                `toml:"gemini"`
		Ollama     OllamaConfig                `toml:"ollama" validate:"required"`
		ComfyUi    ComfyUiConfig               `toml:"comfyui" validate:"required"`
		Birdhole   BirdholeConfig              `toml:"birdhole" validate:"required"`
		Logging    logger.Config               `toml:"logging" validate:"required"`
	}

	AiBird struct {
		AsciiRecordingUrl  string    `toml:"recordingUrl" validate:"omitempty,url"`
		FloodThreshold     int       `toml:"floodThreshold" validate:"gte=0"`
		FloodIgnoreMinutes int       `toml:"floodIgnoreMinutes" validate:"gte=0"`
		ActionTrigger      string    `toml:"actionTrigger" validate:"required"`
		AiChatContextLimit int       `toml:"aiChatContextLimit" validate:"gte=0"`
		Support            []Support `toml:"support"`
		StatusUrl          string    `toml:"statusUrl" validate:"omitempty,url"`
		StatusApiKey       string    `toml:"statusApiKey"`
		Proxy              Proxy     `toml:"proxy"`
		KickRetryDelay     int       `toml:"kickRetryDelay" validate:"gte=0"`
	}

	Support struct {
		Name  string `toml:"name" validate:"required"`
		Value string `toml:"value" validate:"required"`
	}

	Proxy struct {
		User string `toml:"user"`
		Pass string `toml:"pass"`
		Host string `toml:"host"`
		Port string `toml:"port"`
	}

	OpenRouterConfig struct {
		Url          string `toml:"url" validate:"required,url"`
		ApiKey       string `toml:"apiKey" validate:"required"`
		DefaultModel string `toml:"defaultModel"`
	}

	GeminiConfig struct {
		ApiKey string `toml:"apiKey"`
	}

	OllamaConfig struct {
		Url          string `toml:"url" validate:"required,url"`
		Port         string `toml:"port" validate:"required"`
		DefaultModel string `toml:"defaultModel"`
		ContextLimit int    `toml:"contextLimit" validate:"gte=0"`
	}

	ComfyUiConfig struct {
		Url            string        `toml:"url" validate:"required"`
		Ports          []ComfyUiPort `toml:"ports" validate:"required,min=1,dive"`
		BadWords       []string      `toml:"badWords"`
		BadWordsPrompt string        `toml:"badWordsPrompt"`
		MaxQueueSize   int           `toml:"maxQueueSize" validate:"gte=0"`
		RewritePrompts bool          `toml:"rewritePrompts"`
	}

	ComfyUiPort struct {
		Name string `toml:"name" validate:"required"`
		Port int    `toml:"port" validate:"required"`
	}

	BirdholeConfig struct {
		Host        string `toml:"host" validate:"required"`
		Port        string `toml:"port" validate:"required"`
		EndPoint    string `toml:"endPoint" validate:"required"`
		Key         string `toml:"key" validate:"required"`
		UrlLen      int    `toml:"urlLen"`
		Expiry      int    `toml:"expiry"`
		Description string `toml:"description"`
	}
)
