package settings

import (
	"aibird/irc/networks"
	"aibird/logger"
)

type (
	Config struct {
		Networks map[string]networks.Network `toml:"networks" validate:"required,min=1"`
		AiBird   AiBird                      `toml:"aibird" validate:"required"`
		LlamaCpp LlamaCppConfig              `toml:"llamacpp" validate:"required"`
		Glm      GlmConfig                   `toml:"glm"`
		ComfyUi  ComfyUiConfig               `toml:"comfyui" validate:"required"`
		Birdhole BirdholeConfig              `toml:"birdhole" validate:"required"`
		Logging  logger.Config               `toml:"logging" validate:"required"`
		Mcp      McpConfig                   `toml:"mcp"`
		YouTube  YouTubeConfig               `toml:"youtube"`
		Live     LiveConfig                  `toml:"live"`
	}

	AiBird struct {
		AsciiRecordingUrl    string    `toml:"recordingUrl" validate:"omitempty,url"`
		FloodThreshold       int       `toml:"floodThreshold" validate:"gte=0"`
		FloodIgnoreMinutes   int       `toml:"floodIgnoreMinutes" validate:"gte=0"`
		ActionTrigger        string    `toml:"actionTrigger" validate:"required"`
		DenyCommands         []string  `toml:"denyCommands"`
		AiChatContextLimit   int       `toml:"aiChatContextLimit" validate:"gte=0"`
		Support              []Support `toml:"support"`
		StatusUrl            string    `toml:"statusUrl" validate:"omitempty,url"`
		StatusApiKey         string    `toml:"statusApiKey"`
		Proxy                Proxy     `toml:"proxy"`
		KickRetryDelay       int       `toml:"kickRetryDelay" validate:"gte=0"`
		SummaryRetentionDays int       `toml:"summaryRetentionDays" validate:"gte=0"`
		SummaryMaxMessages   int       `toml:"summaryMaxMessages" validate:"gte=0"`
		SummaryDefaultHours  int       `toml:"summaryDefaultHours" validate:"gte=0"`
		VibeMaxMessages      int       `toml:"vibeMaxMessages" validate:"gte=0"`
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

	LlamaCppConfig struct {
		Url          string `toml:"url" validate:"required,url"`
		Port         string `toml:"port" validate:"required"`
		DefaultModel string `toml:"defaultModel"`
		ContextLimit int    `toml:"contextLimit" validate:"gte=0"`
	}

	GlmConfig struct {
		Url          string `toml:"url"`
		ApiKey       string `toml:"apiKey"`
		DefaultModel string `toml:"defaultModel"`
		ImageUrl     string `toml:"imageUrl"`
		ImageModel   string `toml:"imageModel"`
	}

	ComfyUiConfig struct {
		Url            string   `toml:"url" validate:"required"`
		Port           int      `toml:"port" validate:"required"`
		BadWords       []string `toml:"badWords"`
		BadWordsPrompt string   `toml:"badWordsPrompt"`
		MaxQueueSize   int      `toml:"maxQueueSize" validate:"gte=0"`
		RewritePrompts bool     `toml:"rewritePrompts"`
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

	// McpConfig holds configuration for MCP tool-calling integration.
	// The name "mcp" stands for "managed capability providers" — this is NOT
	// the Model Context Protocol, but a tool-calling integration layer that
	// gives the LLM access to external tools like web search and URL scraping.
	McpConfig struct {
		SearXNG       SearXNGConfig   `toml:"searxng"`
		Firecrawl     FirecrawlConfig `toml:"firecrawl"`
		Enabled       bool            `toml:"enabled"`
		MaxIterations int             `toml:"maxIterations"` // max tool calling iterations (default 5)
		TimeoutSecs   int             `toml:"timeoutSecs"`   // total loop timeout in seconds (default 120)
	}

	// SearXNGConfig holds connection details for a self-hosted SearXNG search instance.
	SearXNGConfig struct {
		Url  string `toml:"url" validate:"omitempty,url"`
		Port string `toml:"port"`
	}

	// FirecrawlConfig holds connection details for a self-hosted Firecrawl scrape instance.
	FirecrawlConfig struct {
		Url    string `toml:"url" validate:"omitempty,url"`
		Port   string `toml:"port"`
		ApiKey string `toml:"apiKey" json:"-"`
	}

	// YouTubeConfig configures the !live stream monitor. When ApiKey is empty,
	// stream-end auto-detection is disabled and !live only supports manual
	// announce/off. The key must never be logged or echoed to IRC.
	// PollIntervalSecs <= 0 means the 120s default; values 1-29 clamp to 30s.
	YouTubeConfig struct {
		ApiKey           string `toml:"apiKey"`
		PollIntervalSecs int    `toml:"pollIntervalSecs" validate:"gte=0"`
	}

	// LiveConfig configures the periodic in-stream promo broadcast. While a
	// !live stream is active in PromoNetwork/PromoChannel, the promo lines
	// are repeated every PromoIntervalMins. Zero values fall back to the
	// efnet #birdnest defaults with a 30 minute interval; a negative
	// PromoIntervalMins disables the promo entirely.
	LiveConfig struct {
		PromoNetwork      string `toml:"promoNetwork"`
		PromoChannel      string `toml:"promoChannel"`
		PromoIntervalMins int    `toml:"promoIntervalMins"`
	}
)
