package commands

import (
	"fmt"
	"strings"

	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text/llamacpp"

	meta "aibird/shared/meta"
)

// premiumVideoWorkflow is the ComfyUI workflow all enhanced-video style commands
// run. The MiniMax H3 model is "m3" on disk (comfyuijson/m3.json).
const premiumVideoWorkflow = "m3"

// freeAccessChannel is the IRC channel where premium commands are free for
// everyone (a community/test channel). In all other channels the commands
// require Patreon-level access (see checkPremiumAccess).
const freeAccessChannel = "#h4x"

// enhancedVideoTemplates maps each enhanced-video command name to the prompt
// template file under text/prompts/ used to expand the user's idea via llama.cpp
// before running the m3 workflow. This is the single source of truth for which
// commands are premium enhanced-video commands and which template each loads.
var enhancedVideoTemplates = map[string]string{
	"h3":        "h3.md",
	"sienfield": "sienfield.md",
	"garfield":  "garfield.md",
	"spongebob": "spongebob.md",
}

// enhancedVideoTemplate returns the prompt template file for an enhanced-video
// command (case-insensitive) and whether the action is one of these commands.
func enhancedVideoTemplate(action string) (string, bool) {
	tpl, ok := enhancedVideoTemplates[strings.ToLower(action)]
	return tpl, ok
}

// isEnhancedVideoCommand reports whether action is one of the premium
// enhanced-video style commands (h3, sienfield, garfield, spongebob).
func isEnhancedVideoCommand(action string) bool {
	_, ok := enhancedVideoTemplate(action)
	return ok
}

// hasPremiumAccess reports whether the current user may run a Patreon-gated
// command. Access is granted in the free community channel (#h4x) for everyone,
// or anywhere when the user holds premium access (access level >= 2 / admin /
// owner). This is the pure boolean form (no IRC side effects) for testability.
func hasPremiumAccess(irc *state.State) bool {
	if irc.Channel != nil && strings.EqualFold(irc.Channel.Name, freeAccessChannel) {
		return true
	}
	return irc.User != nil && irc.User.CanUsePremiumGPU()
}

// checkPremiumAccess is the guard used by premium commands. Returns true when
// access is allowed; otherwise sends an explanatory error to the channel/user
// and returns false.
func checkPremiumAccess(irc *state.State) bool {
	if hasPremiumAccess(irc) {
		return true
	}
	irc.SendError("✨ This command is for Patreon supporters (Swan Squadron+). Try it in #h4x or see !support")
	return false
}

// ParseEnhancedVideo handles an enhanced-video style command (non-queued entry
// point, kept for symmetry with the other Parse* commands).
func ParseEnhancedVideo(irc *state.State) bool {
	tpl, ok := enhancedVideoTemplate(irc.Action())
	if !ok {
		return false
	}
	return parseEnhancedVideoCommand(*irc, tpl, meta.GPU4090)
}

// ParseEnhancedVideoWithGPU handles an enhanced-video command with an explicit
// GPU selection (queued path).
func ParseEnhancedVideoWithGPU(irc state.State, gpu meta.GPUType) bool {
	tpl, ok := enhancedVideoTemplate(irc.Action())
	if !ok {
		return false
	}
	return parseEnhancedVideoCommand(irc, tpl, gpu)
}

// parseEnhancedVideoCommand expands the user's video idea using the named
// template via llama.cpp, then runs the ComfyUI m3 workflow with the expanded
// prompt. Patreon access (#h4x-or-premium) is checked before any expensive work.
// Workflow-level accessLevel inside comfyui.Process acts as a final safety net.
func parseEnhancedVideoCommand(irc state.State, templateFile string, gpu meta.GPUType) bool {
	// The m3 workflow may not be deployed yet — fail fast with a clear message.
	if !comfyui.WorkflowExists(premiumVideoWorkflow) {
		irc.SendError("The video workflow (m3) is not available yet")
		return false
	}

	// Patreon gate (with #h4x free exception) — before the LLM call.
	if !checkPremiumAccess(&irc) {
		return false
	}

	// Prompt enhancement needs llama.cpp; bail early if it's offline.
	if !checkLlamaCppAvailable(&irc) {
		return false
	}

	message := comfyui.CleanPrompt(irc.Message())
	if message == "" {
		irc.Send("Usage: !<command> <your video idea>")
		return false
	}

	enhanced, err := llamacpp.EnhancePromptWithTemplate(message, templateFile, irc.Config.LlamaCpp)
	if err != nil {
		logger.Error("Enhanced video prompt enhancement failed", "template", templateFile, "error", err)
		irc.SendError("Failed to enhance prompt: " + err.Error())
		return false
	}
	if strings.TrimSpace(enhanced) == "" {
		irc.SendError("Prompt enhancement returned an empty result")
		return false
	}

	// Build the display message (what the user sees) honoring the bad-words filter.
	displayMessage := message
	if comfyui.BadWordsCheck(message, &irc.Config.ComfyUi) {
		displayMessage = irc.Config.ComfyUi.BadWordsPrompt
	}

	// Notify the user that processing has started (before the GPU work).
	if irc.User.CanUsePremiumGPU() {
		irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx %s🔥 processing '%s'... please wait.", irc.User.NickName, gpu, displayMessage))
	} else {
		irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, displayMessage))
	}

	// Override the action so comfyui.Process loads m3.json (the MiniMax workflow).
	// irc is a value copy local to this function; mutating it here does not affect
	// the caller's dispatch routing (which already matched on the command name).
	irc.Command.Action = premiumVideoWorkflow

	response, err := comfyui.Process(&irc, enhanced, gpu)
	if err != nil {
		logger.Error("Enhanced video ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(&irc, response, displayMessage, enhanced)
}
