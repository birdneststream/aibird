package commands

import (
	"fmt"
	"strconv"

	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text/llamacpp"

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
	if comfyui.WorkflowExists(irc.Action()) {
		var aiEnhancedPrompt string
		message := comfyui.CleanPrompt(irc.Message())

		if irc.GetBoolArg("pe") {
			irc.Send("✨ Enhancing prompt with ai! ✨")
			aiEnhancedPrompt, _ = llamacpp.EnhancePrompt(message, irc.Config.LlamaCpp)
		}

		// Use a safe display message to prevent leaking blocked prompts in IRC output
		displayMessage := message
		if comfyui.BadWordsCheck(message, irc.Config.ComfyUi) {
			displayMessage = irc.Config.ComfyUi.BadWordsPrompt
		}

		if irc.User.CanUsePremiumGPU() {
			irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx %s🔥 processing '%s'... please wait.", irc.User.NickName, gpu, displayMessage))
		} else {
			irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, displayMessage))
		}

		response, err := comfyui.Process(irc, aiEnhancedPrompt, gpu)
		if err != nil {
			logger.Error("ComfyUI request failed", "error", err)
			irc.SendError(err.Error())
		} else {
			fields := []request.Fields{
				{Key: "panorama", Value: strconv.FormatBool(irc.IsAction("panorama"))},
			}
			fields = append(fields, irc.BuildUploadFields()...)

			if aiEnhancedPrompt != "" {
				fields = append(fields, request.Fields{Key: "message", Value: displayMessage})
			}

			upload, err := birdhole.BirdHole(response, displayMessage, fields, irc.Config.Birdhole)

			if err != nil {
				logger.Error("Birdhole error", "error", err)
				irc.SendError(err.Error())
			} else {
				irc.ReplyTo(upload + " - " + irc.GetActionTrigger() + irc.Action() + " " + displayMessage)
				return true
			}
		}
	}
	return false
}
