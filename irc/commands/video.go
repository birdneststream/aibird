package commands

import (
	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text/ollama"
	"fmt"
	"strconv"

	meta "aibird/shared/meta"
)

func ParseAiVideo(irc state.State) bool {
	if comfyui.WorkflowExists(irc.Action()) {
		var aiEnhancedPrompt string
		message := comfyui.CleanPrompt(irc.Message())

		aiEnhancedPrompt = ""
		if irc.GetBoolArg("pe") {
			irc.Send("✨ Enhancing prompt with ai! ✨")
			aiEnhancedPrompt, _ = ollama.EnhancePrompt(message, irc.Config.Ollama)
		}

		if irc.User.CanUse4090() {
			irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx 4090🔥 processing '%s'... please wait.", irc.User.NickName, message))
		} else {
			irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, message))
		}

		response, err := comfyui.Process(irc, aiEnhancedPrompt, meta.GPU4090)
		if err != nil {
			logger.Error("ComfyUI request failed", "error", err)
			irc.SendError(err.Error())
		} else {

			fields := []request.Fields{
				{Key: "panorama", Value: strconv.FormatBool(irc.IsAction("panorama"))},
				{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
				{Key: "meta_network", Value: irc.Network.NetworkName},
				{Key: "meta_channel", Value: irc.Channel.Name},
				{Key: "meta_user", Value: irc.User.NickName},
				{Key: "meta_ident", Value: irc.User.Ident},
				{Key: "meta_host", Value: irc.User.Host},
			}

			if aiEnhancedPrompt != "" {
				fields = append(fields, request.Fields{Key: "message", Value: aiEnhancedPrompt})
			}

			upload, err := birdhole.BirdHole(response, message, fields, irc.Config.Birdhole)

			if err != nil {
				logger.Error("Birdhole error", "error", err)
			} else {
				irc.ReplyTo(upload + " - " + irc.GetActionTrigger() + irc.Action() + " " + message)

				return true
			}
		}
	}
	return false
}

// ParseAiVideoWithGPU handles video commands with explicit GPU selection
func ParseAiVideoWithGPU(irc state.State, gpu meta.GPUType) bool {
	if comfyui.WorkflowExists(irc.Action()) {
		var aiEnhancedPrompt string
		message := comfyui.CleanPrompt(irc.Message())

		aiEnhancedPrompt = ""
		if irc.GetBoolArg("pe") {
			irc.Send("✨ Enhancing prompt with ai! ✨")
			aiEnhancedPrompt, _ = ollama.EnhancePrompt(message, irc.Config.Ollama)
		}

		// Send processing message before starting the actual processing
		if irc.User.CanUse4090() {
			irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx 4090🔥 processing '%s'... please wait.", irc.User.NickName, message))
		} else {
			irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, message))
		}

		// Use the provided GPU parameter instead of hardcoded GPU4090
		response, err := comfyui.Process(irc, aiEnhancedPrompt, gpu)
		if err != nil {
			logger.Error("ComfyUI request failed", "error", err)
			irc.SendError(err.Error())
		} else {
			fields := []request.Fields{
				{Key: "panorama", Value: strconv.FormatBool(irc.IsAction("panorama"))},
				{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
				{Key: "meta_network", Value: irc.Network.NetworkName},
				{Key: "meta_channel", Value: irc.Channel.Name},
				{Key: "meta_user", Value: irc.User.NickName},
				{Key: "meta_ident", Value: irc.User.Ident},
				{Key: "meta_host", Value: irc.User.Host},
			}

			if aiEnhancedPrompt != "" {
				fields = append(fields, request.Fields{Key: "message", Value: aiEnhancedPrompt})
			}

			upload, err := birdhole.BirdHole(response, message, fields, irc.Config.Birdhole)

			if err != nil {
				logger.Error("Birdhole error", "error", err)
			} else {
				irc.ReplyTo(upload + " - " + irc.GetActionTrigger() + irc.Action() + " " + message)
				return true
			}
		}
	}

	return false
}
