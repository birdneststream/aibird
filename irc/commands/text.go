package commands

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/shared/meta"
	"aibird/status"
	"aibird/text/glm"
	"aibird/text/ollama"
	"aibird/text/openrouter"

	"github.com/lrstanley/girc"
)

const openrouterService = "openrouter"

func ParseAiText(irc state.State) bool {
	if irc.IsAction("ai") {
		if irc.GetBoolArg("info") {
			irc.ReplyTo(girc.Fmt(fmt.Sprintf("🧠 AI service: %s 🧠 AI model: %s 🧠 Base prompt: %s 🧠 Personality: %s",
				defaultIfEmpty(irc.User.GetAiService(), "openrouter"),
				defaultIfEmpty(irc.User.GetAiModel(), "default"),
				defaultIfEmpty(irc.User.GetBasePrompt(), "will use personality"),
				defaultIfEmpty(irc.User.GetPersonality(), "ai"))))
			return true
		}

		setPersonality, _ := irc.GetStringArg("setPersonality", "")
		if setPersonality != "" {
			irc.User.SetPersonality(setPersonality)
			irc.Send(girc.Fmt("🧠 Personality set to: " + setPersonality))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg("clearPersonality") {
			irc.User.SetPersonality("")
			irc.Send(girc.Fmt("🧠 Personality cleared"))
			irc.Network.Save()
			return true
		}

		setBasePrompt, _ := irc.GetStringArg("setBasePrompt", "")
		if setBasePrompt != "" {
			irc.User.SetBasePrompt(setBasePrompt)
			irc.Send(girc.Fmt("🧠 Base prompt set to: " + setBasePrompt))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg("clearBasePrompt") {
			irc.User.SetBasePrompt("")
			irc.Send(girc.Fmt("🧠 Base prompt cleared"))
			irc.Network.Save()
			return true
		}

		setAiModel, _ := irc.GetStringArg("setAiModel", "")
		if setAiModel != "" {
			// TODO: Check with models list
			irc.User.SetAiModel(setAiModel)
			irc.Send(girc.Fmt("🧠 AI model set to: " + setAiModel))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg("clearAiModel") {
			irc.User.SetAiModel("")
			irc.Send(girc.Fmt("🧠 AI model cleared"))
			irc.Network.Save()
			return true
		}

		setAiService, _ := irc.GetStringArg("setAiService", "")
		if setAiService != "" {
			if setAiService != "ollama" && setAiService != openrouterService {
				irc.SendError("🧠 AI service not found. Please choose between ollama or openrouter")
				return true
			}

			irc.User.SetAiService(setAiService)
			irc.Send(girc.Fmt("🧠 AI service set to: " + setAiService))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg("clearAiService") {
			irc.User.SetAiService("openrouter")
			irc.Send(girc.Fmt("🧠 AI service defaulting to openrouter"))
			irc.Network.Save()
			return true
		}

		service := irc.User.GetAiService()
		if service == "" {
			service = "openrouter"
		}

		switch service {
		case "openrouter":
			if irc.IsEmptyMessage() {
				return true
			}
			irc.ReplyTo(girc.Fmt("🧠 Processing AI request, please wait..."))
			response, err := callOpenRouterWithFallback(irc)
			if err != nil {
				logger.Error("Error processing AI request", "error", err)
				irc.SendError(fmt.Sprintf("🧠 Error processing AI request: %s", err))
			} else {
				logger.Debug("Calling handleAiResponse from OpenRouter path")
				handleAiResponse(irc, response)
			}
			return true

		case "ollama":
			if irc.IsEmptyMessage() {
				return true
			}

			// Try to check Ollama status, but proceed anyway if status service is offline
			systemStatus := status.NewClient(irc.Config.AiBird)
			isOllamaRunning, err := systemStatus.IsOllamaRunning()
			if err != nil {
				logger.Warn("Status service offline, proceeding with Ollama request anyway", "error", err)
				isOllamaRunning = true // Assume it's running and let the actual request fail if it's not
			}

			if !isOllamaRunning {
				irc.SendError("🧠 Ollama AI service is offline")
				return true
			}

			// dsqwen is 32b and uses all the 4090, so we need to check if it's available
			if irc.GetBoolArg("dsqwen") {
				isSteamRunning, err := systemStatus.IsSteamRunning()
				if err != nil {
					logger.Warn("Status service offline, cannot check Steam status for dsqwen", "error", err)
					// Proceed anyway - let the actual request fail if there's not enough VRAM
				} else if isSteamRunning {
					irc.SendError("🧠 Not enough VRAM to process request")
					return true
				}
			}

			irc.ReplyTo(girc.Fmt("🧠 Processing AI request, please wait..."))
			response, err := ollama.ChatRequest(irc)
			if err != nil {
				logger.Error("Error processing AI request", "error", err)
				irc.SendError(fmt.Sprintf("🧠 Error processing AI request: %s", err))
			} else {
				logger.Debug("Calling handleAiResponse from Ollama path")
				handleAiResponse(irc, response)
			}

			return true
		}
	}

	if irc.IsAction("glm") && irc.Config.Glm.ApiKey != "" {
		if irc.IsEmptyMessage() {
			return true
		}

		irc.ReplyTo(girc.Fmt("🧠 Processing GLM request, please wait..."))

		go func() {
			response, err := glm.Request(irc)
			if err != nil {
				logger.Error("GLM request failed", "error", err)
				irc.SendError(fmt.Sprintf("🧠 GLM error: %s", err))
			} else {
				irc.TextToBirdhole(response)
			}
		}()
		return true
	}

	if irc.IsAction("glm-img") && irc.Config.Glm.ApiKey != "" {
		if irc.IsEmptyMessage() {
			irc.Send("Usage: !glm-img <prompt> [--size=1024x1024]")
			return true
		}

		size, _ := irc.GetStringArg("size", "1024x1024")
		irc.ReplyTo(girc.Fmt("🎨 Generating image with CogView-4, please wait..."))

		go func() {
			imageUrl, err := glm.ImageRequest(irc.Message(), size, irc.Config.Glm)
			if err != nil {
				logger.Error("GLM image request failed", "error", err)
				irc.SendError(fmt.Sprintf("🎨 GLM image error: %s", err))
				return
			}

			// Download the image to a temp file
			tempFile := fmt.Sprintf("%s/glm-img-%d.png", os.TempDir(), time.Now().UnixNano())
			downloadReq := request.Request{
				Url:      imageUrl,
				FileName: tempFile,
			}
			if err := downloadReq.Download(); err != nil {
				logger.Error("Failed to download GLM image", "error", err)
				irc.SendError("Failed to download generated image")
				return
			}
			defer os.Remove(tempFile)

			// Upload to birdhole
			fields := []request.Fields{
				{Key: "tags", Value: "glm-img," + irc.Network.NetworkName},
				{Key: "meta_network", Value: irc.Network.NetworkName},
				{Key: "meta_nick", Value: irc.User.NickName},
				{Key: "meta_prompt", Value: irc.Message()},
			}
			upload, err := birdhole.BirdHole(tempFile, irc.Message(), fields, irc.Config.Birdhole)
			if err != nil {
				logger.Error("Birdhole upload failed", "error", err)
				irc.SendError("Failed to upload image: " + err.Error())
				return
			}

			irc.ReplyTo(fmt.Sprintf("🎨 %s", upload))
		}()
		return true
	}

	return false
}

func callOpenRouterWithFallback(irc state.State) (string, error) {
	response, err := openrouter.OpenRouterRequest(irc)
	if err != nil {
		logger.Error("OpenRouter request failed", "error", err)
		irc.Send(girc.Fmt("🧠 OpenRouter failed, falling back to ollama..."))

		systemStatus := status.NewClient(irc.Config.AiBird)
		isOllamaRunning, ollamaErr := systemStatus.IsOllamaRunning()
		if ollamaErr != nil {
			logger.Warn("Status service offline, proceeding with ollama fallback anyway", "error", ollamaErr)
			// Try ollama anyway - let it fail if it's actually offline
			return ollama.ChatRequest(irc)
		}
		if !isOllamaRunning {
			return "", fmt.Errorf("🧠 Ollama AI service is offline")
		}

		return ollama.ChatRequest(irc)
	}
	return response, nil
}

func handleAiResponse(irc state.State, response string) {
	hasTTS := irc.GetBoolArg("tts")
	voiceArg := irc.FindArgument("voice", "")
	logger.Debug("handleAiResponse called", "has_tts_flag", hasTTS, "voice_arg", voiceArg)

	if hasTTS || voiceArg != "" {
		originalMessage := irc.Message()
		irc.Command.Action = "tts"

		// Strip thinking content for TTS processing
		ttsResponse := stripThinkingContent(response)
		logger.Info("TTS content filtering applied", "original_length", len(response), "filtered_length", len(ttsResponse), "has_think_tags", strings.Contains(response, "<think>"))

		// Debug: Show first 200 chars of original response
		preview := response
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		logger.Debug("Original response preview", "content", preview)

		irc.SetMessage(ttsResponse)

		ProcessAndUploadAudio(irc, originalMessage, ttsResponse, meta.GPU4090)

		irc.Command.Action = "ai"
		irc.SetMessage(originalMessage)
	} else {
		irc.TextToBirdhole(response)
	}
}

// stripThinkingContent removes <think>...</think> blocks, emojis, and markdown from AI responses for TTS processing
func stripThinkingContent(text string) string {
	// Remove <think>...</think> blocks (case insensitive, multiline with dotall)
	re := regexp.MustCompile(`(?is)<think>.*?</think>`)
	cleaned := re.ReplaceAllString(text, "")

	// Remove markdown formatting
	cleaned = stripMarkdown(cleaned)

	// Remove emojis
	cleaned = stripEmojis(cleaned)

	// Clean up extra whitespace and newlines that might be left behind
	cleaned = strings.TrimSpace(cleaned)

	// Fix escaped quotes for natural speech
	cleaned = strings.ReplaceAll(cleaned, `\"`, `"`)

	// Remove all newlines for TTS (convert to spaces)
	cleaned = regexp.MustCompile(`\n+`).ReplaceAllString(cleaned, " ")

	// Clean up multiple spaces
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	return cleaned
}

// stripMarkdown removes common markdown formatting for cleaner TTS
func stripMarkdown(text string) string {
	// Remove bold/italic markers: **bold**, *italic*, __bold__, _italic_
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`__([^_]+)__`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(text, "$1")

	// Remove code blocks: ```code``` and `code`
	text = regexp.MustCompile("```[\\s\\S]*?```").ReplaceAllString(text, "")
	text = regexp.MustCompile("`([^`]+)`").ReplaceAllString(text, "$1")

	// Remove headers: # ## ### etc.
	text = regexp.MustCompile(`(?m)^#{1,6}\s*(.*)$`).ReplaceAllString(text, "$1")

	// Remove links: [text](url) -> text
	text = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(text, "$1")

	// Remove strikethrough: ~~text~~
	text = regexp.MustCompile(`~~([^~]+)~~`).ReplaceAllString(text, "$1")

	// Remove blockquotes: > text
	text = regexp.MustCompile(`(?m)^>\s*(.*)$`).ReplaceAllString(text, "$1")

	// Remove horizontal rules: --- or ***
	text = regexp.MustCompile(`(?m)^[-*]{3,}$`).ReplaceAllString(text, "")

	return text
}

// stripEmojis removes Unicode emoji characters for cleaner TTS
func stripEmojis(text string) string {
	// Remove emoji ranges (most common Unicode emoji blocks)
	// Emoticons: U+1F600-U+1F64F
	text = regexp.MustCompile(`[\x{1F600}-\x{1F64F}]`).ReplaceAllString(text, "")
	// Miscellaneous Symbols: U+1F300-U+1F5FF
	text = regexp.MustCompile(`[\x{1F300}-\x{1F5FF}]`).ReplaceAllString(text, "")
	// Transport and Map: U+1F680-U+1F6FF
	text = regexp.MustCompile(`[\x{1F680}-\x{1F6FF}]`).ReplaceAllString(text, "")
	// Additional Emoticons: U+1F910-U+1F96B
	text = regexp.MustCompile(`[\x{1F910}-\x{1F96B}]`).ReplaceAllString(text, "")
	// Additional Transport and Map: U+1F980-U+1F9E0
	text = regexp.MustCompile(`[\x{1F980}-\x{1F9E0}]`).ReplaceAllString(text, "")
	// Symbols and Pictographs: U+1F1E6-U+1F1FF (flags)
	text = regexp.MustCompile(`[\x{1F1E6}-\x{1F1FF}]`).ReplaceAllString(text, "")

	return text
}

// ShouldQueueOllamaAi checks if an !ai command needs to be queued.
// Returns true if it's an Ollama request that should be queued.
func ShouldQueueOllamaAi(irc state.State) bool {
	if !irc.IsAction("ai") {
		return false
	}

	// Don't queue info command
	if irc.GetBoolArg("info") {
		return false
	}

	// Don't queue settings commands
	if setPersonality, _ := irc.GetStringArg("setPersonality", ""); setPersonality != "" {
		return false
	}
	if irc.GetBoolArg("clearPersonality") {
		return false
	}
	if setBasePrompt, _ := irc.GetStringArg("setBasePrompt", ""); setBasePrompt != "" {
		return false
	}
	if irc.GetBoolArg("clearBasePrompt") {
		return false
	}
	if setAiModel, _ := irc.GetStringArg("setAiModel", ""); setAiModel != "" {
		return false
	}
	if irc.GetBoolArg("clearAiModel") {
		return false
	}
	if setAiService, _ := irc.GetStringArg("setAiService", ""); setAiService != "" {
		return false
	}
	if irc.GetBoolArg("clearAiService") {
		return false
	}

	// Don't queue empty messages
	if irc.IsEmptyMessage() {
		return false
	}

	// Only queue if using Ollama service
	service := irc.User.GetAiService()
	if service == "" {
		service = "openrouter"
	}

	return service == "ollama"
}

// CheckAiCanUse checks if the AI system is available (respects can_use flag)
func CheckAiCanUse(irc state.State) error {
	statusClient := status.NewClient(irc.Config.AiBird)
	_, err := statusClient.CheckCanUse()
	return err
}

// ProcessOllamaAiRequest handles Ollama AI requests from the queue.
// This is called by the queue processor, not directly by ParseAiText.
func ProcessOllamaAiRequest(irc state.State, gpu meta.GPUType) {
	// Try to check Ollama status, but proceed anyway if status service is offline
	systemStatus := status.NewClient(irc.Config.AiBird)
	isOllamaRunning, err := systemStatus.IsOllamaRunning()
	if err != nil {
		logger.Warn("Status service offline, proceeding with Ollama request anyway", "error", err)
		isOllamaRunning = true // Assume it's running
	}

	if !isOllamaRunning {
		irc.SendError("🧠 Ollama AI service is offline")
		return
	}

	// dsqwen check for VRAM
	if irc.GetBoolArg("dsqwen") {
		isSteamRunning, err := systemStatus.IsSteamRunning()
		if err != nil {
			logger.Warn("Status service offline, cannot check Steam status for dsqwen", "error", err)
		} else if isSteamRunning {
			irc.SendError("🧠 Not enough VRAM to process request")
			return
		}
	}

	irc.ReplyTo(girc.Fmt("🧠 Processing AI request, please wait..."))
	response, err := ollama.ChatRequest(irc)
	if err != nil {
		logger.Error("Error processing AI request", "error", err)
		irc.SendError(fmt.Sprintf("🧠 Error processing AI request: %s", err))
	} else {
		logger.Debug("Calling handleAiResponse from Ollama queue path")
		handleAiResponse(irc, response)
	}
}
