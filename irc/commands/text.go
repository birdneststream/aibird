package commands

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"aibird/helpers"
	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/shared/meta"
	"aibird/status"
	"aibird/text/glm"
	"aibird/text/llamacpp"

	"github.com/lrstanley/girc"
)

// checkLlamaCppAvailable checks if llama.cpp is running. Returns true if
// available (or if health check fails, assuming it's running). Returns false
// and sends an error message if confirmed offline.
// AI service name constants.
const (
	serviceGLM      = "glm"
	serviceLlamacpp = "llamacpp"
)

// AI settings flag name constants (used in both ParseAiText and aiSettingsFlags).
const (
	flagInfo             = "info"
	flagSetPersonality   = "setPersonality"
	flagClearPersonality = "clearPersonality"
	flagSetBasePrompt    = "setBasePrompt"
	flagClearBasePrompt  = "clearBasePrompt"
	flagSetAiModel       = "setAiModel"
	flagClearAiModel     = "clearAiModel"
	flagSetAiService     = "setAiService"
	flagClearAiService   = "clearAiService"
)

func checkLlamaCppAvailable(irc *state.State) bool {
	isRunning, err := llamacpp.IsLlamaCppRunning(irc.Config.LlamaCpp.Url, irc.Config.LlamaCpp.Port)
	if err != nil {
		logger.Warn("Health check failed, proceeding with llama.cpp request anyway", "error", err)
		return true // Assume it's running and let the actual request fail if not
	}

	if !isRunning {
		irc.SendError("🧠 llama.cpp AI service is offline")
		return false
	}

	return true
}

var (
	reThinkingBlock   = regexp.MustCompile(`(?is)<think.*?</think >`)
	reNewlines        = regexp.MustCompile(`\n+`)
	reWhitespace      = regexp.MustCompile(`\s+`)
	reBold            = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic          = regexp.MustCompile(`\*([^*]+)\*`)
	reUnderlineBold   = regexp.MustCompile(`__([^_]+)__`)
	reUnderlineItalic = regexp.MustCompile(`_([^_]+)_`)
	reCodeBlock       = regexp.MustCompile("```[\\s\\S]*?```")
	reInlineCode      = regexp.MustCompile("`([^`]+)`")
	reHeaders         = regexp.MustCompile(`(?m)^#{1,6}\s*(.*)$`)
	reLinks           = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reStrikethrough   = regexp.MustCompile(`~~([^~]+)~~`)
	reBlockquotes     = regexp.MustCompile(`(?m)^>\s*(.*)$`)
	reHorizontalRules = regexp.MustCompile(`(?m)^[-*]{3,}$`)
	reEmoticons       = regexp.MustCompile(`[\x{1F600}-\x{1F64F}]`)
	reSymbols         = regexp.MustCompile(`[\x{1F300}-\x{1F5FF}]`)
	reTransport       = regexp.MustCompile(`[\x{1F680}-\x{1F6FF}]`)
	reAddlEmoticons   = regexp.MustCompile(`[\x{1F910}-\x{1F96B}]`)
	reAddlTransport   = regexp.MustCompile(`[\x{1F980}-\x{1F9E0}]`)
	reFlags           = regexp.MustCompile(`[\x{1F1E6}-\x{1F1FF}]`)
)

func ParseAiText(irc *state.State) bool {
	if irc.IsAction("ai") {
		if irc.GetBoolArg(flagInfo) {
			irc.ReplyTo(girc.Fmt(fmt.Sprintf("🧠 AI service: %s 🧠 AI model: %s 🧠 Base prompt: %s 🧠 Personality: %s",
				displayDefault(irc.User.GetAiService(), "llamacpp"),
				displayDefault(irc.User.GetAiModel(), "default"),
				displayDefault(irc.User.GetBasePrompt(), "will use personality"),
				displayDefault(irc.User.GetPersonality(), "ai"))))
			return true
		}

		setPersonality, _ := irc.GetStringArg(flagSetPersonality, "")
		if setPersonality != "" {
			irc.User.SetPersonality(setPersonality)
			irc.Send(girc.Fmt("🧠 Personality set to: " + setPersonality))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg(flagClearPersonality) {
			irc.User.SetPersonality("")
			irc.Send(girc.Fmt("🧠 Personality cleared"))
			irc.Network.Save()
			return true
		}

		setBasePrompt, _ := irc.GetStringArg(flagSetBasePrompt, "")
		if setBasePrompt != "" {
			irc.User.SetBasePrompt(setBasePrompt)
			irc.Send(girc.Fmt("🧠 Base prompt set to: " + setBasePrompt))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg(flagClearBasePrompt) {
			irc.User.SetBasePrompt("")
			irc.Send(girc.Fmt("🧠 Base prompt cleared"))
			irc.Network.Save()
			return true
		}

		setAiModel, _ := irc.GetStringArg(flagSetAiModel, "")
		if setAiModel != "" {
			// TODO: Check with models list
			irc.User.SetAiModel(setAiModel)
			irc.Send(girc.Fmt("🧠 AI model set to: " + setAiModel))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg(flagClearAiModel) {
			irc.User.SetAiModel("")
			irc.Send(girc.Fmt("🧠 AI model cleared"))
			irc.Network.Save()
			return true
		}

		setAiService, _ := irc.GetStringArg(flagSetAiService, "")
		if setAiService != "" {
			if setAiService != serviceLlamacpp && setAiService != serviceGLM {
				irc.SendError("🧠 AI service not found. Please choose between llamacpp or glm")
				return true
			}

			if setAiService == serviceGLM && irc.Config.Glm.ApiKey == "" {
				irc.SendError("🧠 GLM is not configured. Please set up GLM before using it as AI service.")
				return true
			}

			irc.User.SetAiService(setAiService)
			irc.Send(girc.Fmt("🧠 AI service set to: " + setAiService))
			irc.Network.Save()
			return true
		}

		if irc.GetBoolArg(flagClearAiService) {
			irc.User.SetAiService(serviceLlamacpp)
			irc.Send(girc.Fmt("🧠 AI service defaulting to llamacpp"))
			irc.Network.Save()
			return true
		}

		service := irc.User.GetAiService()
		if service == "" {
			service = serviceLlamacpp
		}

		switch service {
		case serviceGLM:
			if irc.Config.Glm.ApiKey == "" {
				irc.SendError("🧠 GLM is not configured")
				return true
			}
			if irc.IsEmptyMessage() {
				return true
			}
			irc.ReplyTo(girc.Fmt("🧠 Processing AI request, please wait..."))
			helpers.GoSafe(func() {
				response, err := glm.Request(irc)
				if err != nil {
					logger.Error("Error processing AI request", "error", err)
					irc.SendError(fmt.Sprintf("🧠 Error processing AI request: %s", err))
				} else {
					logger.Debug("Calling handleAiResponse from GLM path")
					handleAiResponse(irc, response)
				}
			})
			return true

		case serviceLlamacpp:
			handleLlamaCppRequest(irc)
			return true

		default:
			// Fallback for unknown/legacy service values → use llamacpp
			logger.Info("Unknown AI service, defaulting to llamacpp", "service", service)
			handleLlamaCppRequest(irc)
			return true
		}
	}

	if irc.IsAction("glm") && irc.Config.Glm.ApiKey != "" {
		if irc.IsEmptyMessage() {
			return true
		}

		irc.ReplyTo(girc.Fmt("🧠 Processing GLM request, please wait..."))

		helpers.GoSafe(func() {
			response, err := glm.Request(irc)
			if err != nil {
				logger.Error("GLM request failed", "error", err)
				irc.SendError(fmt.Sprintf("🧠 GLM error: %s", err))
			} else {
				irc.TextToBirdhole(response)
			}
		})
		return true
	}

	if irc.IsAction("glm-img") && irc.Config.Glm.ApiKey != "" {
		if irc.IsEmptyMessage() {
			irc.Send("Usage: !glm-img <prompt> [--size=1024x1024]")
			return true
		}

		size, _ := irc.GetStringArg("size", "1024x1024")
		irc.ReplyTo(girc.Fmt("🎨 Generating image with CogView-4, please wait..."))

		helpers.GoSafe(func() {
			imageUrl, err := glm.ImageRequest(irc.Message(), size, &irc.Config.Glm)
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
			if dlErr := downloadReq.Download(); dlErr != nil {
				logger.Error("Failed to download GLM image", "error", dlErr)
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
			upload, err := birdhole.BirdHole(tempFile, irc.Message(), fields, &irc.Config.Birdhole)
			if err != nil {
				logger.Error("Birdhole upload failed", "error", err)
				irc.SendError("Failed to upload image: " + err.Error())
				return
			}

			irc.ReplyTo(fmt.Sprintf("🎨 %s", upload))
		})
		return true
	}

	return false
}

// handleLlamaCppRequest processes an AI request using llama.cpp.
func handleLlamaCppRequest(irc *state.State) {
	if irc.IsEmptyMessage() {
		return
	}

	if !checkLlamaCppAvailable(irc) {
		return
	}

	irc.ReplyTo(girc.Fmt("🧠 Processing AI request, please wait..."))
	response, err := llamacpp.ChatRequest(irc)
	if err != nil {
		logger.Error("Error processing AI request", "error", err)
		irc.SendError(fmt.Sprintf("🧠 Error processing AI request: %s", err))
	} else {
		logger.Debug("Calling handleAiResponse from llama.cpp path")
		handleAiResponse(irc, response)
	}
}

func handleAiResponse(irc *state.State, response string) {
	hasTTS := irc.GetBoolArg("tts")
	voiceArg := irc.FindArgument("voice", "")
	logger.Debug("handleAiResponse called", "has_tts_flag", hasTTS, "voice_arg", voiceArg)

	if hasTTS || voiceArg != "" {
		originalMessage := irc.Message()

		// Strip thinking content for TTS processing
		ttsResponse := stripThinkingContent(response)
		logger.Info("TTS content filtering applied", "original_length", len(response), "filtered_length", len(ttsResponse), "has_think_tags", strings.Contains(response, "<think"))

		// Debug: Show first 200 chars of original response
		preview := response
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		logger.Debug("Original response preview", "content", preview)

		// Switch action to "tts" so ComfyUI.Process loads the TTS workflow.
		// State is a value type — this mutation only affects our local copy.
		irc.Command.Action = "tts"
		irc.SetMessage(ttsResponse)

		ProcessAndUploadAudio(irc, originalMessage, ttsResponse, meta.GPU4090)
	} else {
		irc.TextToBirdhole(response)
	}
}

// stripThinkingContent removes <think>...</think> blocks, emojis, and markdown from AI responses for TTS processing
func stripThinkingContent(text string) string {
	// Remove <think...</think > blocks (case insensitive, multiline with dotall)
	cleaned := reThinkingBlock.ReplaceAllString(text, "")

	// Remove markdown formatting
	cleaned = stripMarkdown(cleaned)

	// Remove emojis
	cleaned = stripEmojis(cleaned)

	// Clean up extra whitespace and newlines that might be left behind
	cleaned = strings.TrimSpace(cleaned)

	// Fix escaped quotes for natural speech
	cleaned = strings.ReplaceAll(cleaned, `\"`, `"`)

	// Remove all newlines for TTS (convert to spaces)
	cleaned = reNewlines.ReplaceAllString(cleaned, " ")

	// Clean up multiple spaces
	cleaned = reWhitespace.ReplaceAllString(cleaned, " ")

	return cleaned
}

// stripMarkdown removes common markdown formatting for cleaner TTS
func stripMarkdown(text string) string {
	// Remove bold/italic markers: **bold**, *italic*, __bold__, _italic_
	text = reBold.ReplaceAllString(text, "$1")
	text = reItalic.ReplaceAllString(text, "$1")
	text = reUnderlineBold.ReplaceAllString(text, "$1")
	text = reUnderlineItalic.ReplaceAllString(text, "$1")

	// Remove code blocks: ```code``` and `code`
	text = reCodeBlock.ReplaceAllString(text, "")
	text = reInlineCode.ReplaceAllString(text, "$1")

	// Remove headers: # ## ### etc.
	text = reHeaders.ReplaceAllString(text, "$1")

	// Remove links: [text](url) -> text
	text = reLinks.ReplaceAllString(text, "$1")

	// Remove strikethrough: ~~text~~
	text = reStrikethrough.ReplaceAllString(text, "$1")

	// Remove blockquotes: > text
	text = reBlockquotes.ReplaceAllString(text, "$1")

	// Remove horizontal rules: --- or ***
	text = reHorizontalRules.ReplaceAllString(text, "")

	return text
}

// stripEmojis removes Unicode emoji characters for cleaner TTS
func stripEmojis(text string) string {
	text = reEmoticons.ReplaceAllString(text, "")
	text = reSymbols.ReplaceAllString(text, "")
	text = reTransport.ReplaceAllString(text, "")
	text = reAddlEmoticons.ReplaceAllString(text, "")
	text = reAddlTransport.ReplaceAllString(text, "")
	text = reFlags.ReplaceAllString(text, "")
	return text
}

// aiSettingsFlags defines all AI settings flag names and their types.
// This is the single source of truth for isAiSettingsCommand — new settings
// added here are automatically detected as non-queueable.
var aiSettingsFlags = []struct {
	name     string
	isString bool // true = GetStringArg with non-empty check, false = GetBoolArg
}{
	{flagInfo, false},
	{flagSetPersonality, true},
	{flagClearPersonality, false},
	{flagSetBasePrompt, true},
	{flagClearBasePrompt, false},
	{flagSetAiModel, true},
	{flagClearAiModel, false},
	{flagSetAiService, true},
	{flagClearAiService, false},
}

// isAiSettingsCommand checks if the !ai command is a settings/info request
// that should not be queued for GPU processing.
func isAiSettingsCommand(irc *state.State) bool {
	for _, flag := range aiSettingsFlags {
		if flag.isString {
			if v, _ := irc.GetStringArg(flag.name, ""); v != "" {
				return true
			}
		} else {
			if irc.GetBoolArg(flag.name) {
				return true
			}
		}
	}
	return false
}

// ShouldQueueLlamaCppAi checks if an !ai command needs to be queued.
// Returns true if it's a llama.cpp request that should be queued.
func ShouldQueueLlamaCppAi(irc *state.State) bool {
	if !irc.IsAction("ai") {
		return false
	}

	if isAiSettingsCommand(irc) {
		return false
	}

	if irc.IsEmptyMessage() {
		return false
	}

	// Queue all non-GLM requests (llamacpp, default, and any legacy/unknown services)
	service := irc.User.GetAiService()
	if service == "" {
		service = serviceLlamacpp
	}

	return service != serviceGLM
}

// CheckAiCanUse checks if the AI system is available (respects can_use flag)
func CheckAiCanUse(irc *state.State) error {
	statusClient := status.NewClient(&irc.Config.AiBird)
	_, err := statusClient.CheckCanUse()
	return err
}

// ProcessLlamaCppAiRequest handles llama.cpp AI requests from the queue.
// This is called by the queue processor, not directly by ParseAiText.
// The ctx parameter allows cancellation on timeout or queue shutdown.
func ProcessLlamaCppAiRequest(ctx context.Context, irc state.State, gpu meta.GPUType) {
	if !checkLlamaCppAvailable(&irc) {
		return
	}

	irc.ReplyTo(girc.Fmt("🧠 Processing AI request, please wait..."))
	response, err := llamacpp.ChatRequest(&irc)
	if err != nil {
		logger.Error("Error processing AI request", "error", err)
		irc.SendError(fmt.Sprintf("🧠 Error processing AI request: %s", err))
	} else {
		logger.Debug("Calling handleAiResponse from llama.cpp queue path")
		handleAiResponse(&irc, response)
	}
}
