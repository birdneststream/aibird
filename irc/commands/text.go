package commands

import (
	"aibird/irc/state"
	"aibird/logger"
	"aibird/status"
	"aibird/text/gemini"
	"aibird/text/ollama"
	"aibird/text/openrouter"
	"fmt"
	"regexp"
	"strings"

	"github.com/lrstanley/girc"
)

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
			if setAiService != "ollama" && setAiService != "openrouter" {
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

			systemStatus := status.NewClient(irc.Config.AiBird)
			isOllamaRunning, err := systemStatus.IsOllamaRunning()
			if err != nil || !isOllamaRunning {
				irc.SendError("🧠 Ollama AI service is offline")
				return true
			}

			// dsqwen is 32b and uses all the 4090, so we need to check if it's available
			if irc.GetBoolArg("dsqwen") {
				isSteamRunning, err := systemStatus.IsSteamRunning()
				if err != nil {
					logger.Error("Error checking Steam status", "error", err)
					return true
				}

				if isSteamRunning {
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

	if (irc.IsAction("bard") || irc.IsAction("gemini")) && irc.Config.Gemini.ApiKey != "" {
		if irc.IsEmptyMessage() {
			return true
		}

		irc.ReplyTo(girc.Fmt("🧠 Processing Google Gemini request, please wait..."))

		response, err := gemini.Request(irc)
		if err != nil {
			logger.Error("Gemini request failed", "error", err)
		} else {
			if irc.GetBoolArg("tts") || irc.FindArgument("voice", "") != "" {
				originalMessage := irc.Message()
				irc.Command.Action = "tts"
				irc.SetMessage(response)

				ProcessAndUploadAudio(irc, originalMessage, response)

				irc.Command.Action = "bard"
				irc.SetMessage(originalMessage)

				return true
			} else {
				irc.TextToBirdhole(response)
			}
		}
		return true
	}

	if irc.IsAction("anal") {
		if irc.IsEmptyMessage() {
			return true
		}

		// HuggingBird sentiment analysis removed - service no longer available
		irc.Send(girc.Fmt("🧠 Sentiment analysis service is currently unavailable"))
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
		if ollamaErr != nil || !isOllamaRunning {
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

		ProcessAndUploadAudio(irc, originalMessage, ttsResponse)

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
