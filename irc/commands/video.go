package commands

import (
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"

	meta "aibird/shared/meta"
)

// ParseAiVideo handles video commands (non-queued path)
func ParseAiVideo(irc state.State) bool {
	return parseAiVideoInternal(irc, meta.GPU4090)
}

// ParseAiVideoWithGPU handles video commands with explicit GPU selection (queued path)
func ParseAiVideoWithGPU(irc state.State, gpu meta.GPUType) bool {
	return parseAiVideoInternal(irc, gpu)
}

func parseAiVideoInternal(irc state.State, gpu meta.GPUType) bool {
	if !comfyui.WorkflowExists(irc.Action()) {
		return false
	}

	aiEnhancedPrompt, displayMessage := preparePromptAndDisplay(irc, gpu)

	response, err := comfyui.Process(irc, aiEnhancedPrompt, gpu)
	if err != nil {
		logger.Error("ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(irc, response, displayMessage, aiEnhancedPrompt)
}
