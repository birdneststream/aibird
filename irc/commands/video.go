package commands

import (
	"context"

	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
)

// ParseAiVideo handles video commands. ctx allows cancellation on queue
// timeout/shutdown; non-queued callers pass context.Background().
func ParseAiVideo(ctx context.Context, irc state.State) bool {
	if !comfyui.WorkflowExists(irc.Action()) {
		return false
	}

	displayMessage := preparePromptAndDisplay(&irc)

	response, err := comfyui.Process(ctx, &irc, "")
	if err != nil {
		logger.Error("ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(&irc, response, displayMessage)
}
