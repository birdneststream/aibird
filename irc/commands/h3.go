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

// h3Workflow is the ComfyUI workflow file (!h3 action "h3" maps to m3.json).
// MiniMax H3 is the video model; "m3" is the on-disk workflow filename.
const h3Workflow = "m3"

// ParseH3 handles the !h3 command (non-queued entry point, unused by the queue
// dispatch path but kept for symmetry with the other Parse* commands).
func ParseH3(irc *state.State) bool {
	return parseH3Internal(*irc, meta.GPU4090)
}

// ParseH3WithGPU handles !h3 with an explicit GPU selection (queued path).
func ParseH3WithGPU(irc state.State, gpu meta.GPUType) bool {
	return parseH3Internal(irc, gpu)
}

// parseH3Internal expands the user's video idea into a full MiniMax H3 cinematic
// prompt via llama.cpp, then runs the ComfyUI m3.json workflow with that prompt as
// the positive prompt. Access control is enforced by the m3 workflow's aibird_meta
// accessLevel inside comfyui.Process.
func parseH3Internal(irc state.State, gpu meta.GPUType) bool {
	// The m3 workflow may not be deployed yet — fail fast with a clear message.
	if !comfyui.WorkflowExists(h3Workflow) {
		irc.SendError("H3 video workflow (m3) is not available yet")
		return false
	}

	// Prompt enhancement needs llama.cpp; bail early if it's offline.
	if !checkLlamaCppAvailable(&irc) {
		return false
	}

	message := comfyui.CleanPrompt(irc.Message())
	if message == "" {
		irc.Send("Usage: !h3 <your video idea>")
		return false
	}

	enhanced, err := llamacpp.EnhancePromptH3(message, irc.Config.LlamaCpp)
	if err != nil {
		logger.Error("H3 prompt enhancement failed", "error", err)
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

	// Override the action so comfyui.Process loads m3.json (the MiniMax H3 workflow).
	// irc is a value copy local to this function; mutating it here does not affect the
	// caller's dispatch routing (which already matched on "h3").
	irc.Command.Action = h3Workflow

	response, err := comfyui.Process(&irc, enhanced, gpu)
	if err != nil {
		logger.Error("H3 ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(&irc, response, displayMessage, enhanced)
}
