package commands

import (
	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/image"
	"aibird/image/comfyui"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text/ollama"
	"fmt"
	"strconv"
	"strings"

	meta "aibird/shared/meta"

	"github.com/lrstanley/girc"
)

func ParseAiImage(irc state.State) bool {
	if irc.IsAction("sd") {
		if irc.GetBoolArg("help") {
			irc.Send(girc.Fmt(help.FindHelp(irc)))
			return true
		}

		if irc.GetBoolArg("models") {
			irc.Send(girc.Fmt("📸 The following sd models are available: " + comfyui.GetWorkFlows(true)))
			return true
		}

	}

	if comfyui.WorkflowExists(irc.Action()) {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp(irc)))
			return true
		}
		var aiEnhancedPrompt string
		message := comfyui.CleanPrompt(irc.Message())

		imgArg, _ := irc.GetStringArg("img", "")

		if irc.IsAction("flux-img2img") && imgArg == "" {
			irc.SendError("--img argument is required for flux-img2img")
			return true
		}

		if imgArg != "" {
			if !image.IsImageURL(imgArg) {
				irc.SendError("Invalid image URL")
				return true
			}

			if !strings.Contains(irc.Command.Action, "img") && !irc.IsAction("kontext") {
				irc.SendError("Cannot use image for this model")
				return true
			}
		}

		if (strings.Contains(irc.Command.Action, "img") || irc.IsAction("img2ltx")) && imgArg == "" {
			irc.SendError("Image URL required for this model")
			return true
		}

		if irc.IsAction("img2ltx") && message == "" {
			irc.SendError("Prompt required for ltx model")
			return true
		}

		aiEnhancedPrompt = ""
		if (irc.IsAction("ltx") || irc.IsAction("img2ltx")) || irc.GetBoolArg("pe") {
			irc.Send("✨ Enhancing prompt with ai! ✨")
			aiEnhancedPrompt, _ = ollama.EnhancePrompt(message, irc.Config.Ollama)
		}

		//if (irc.IsAction("sdxxxl") || irc.IsAction("sd") || irc.IsAction("porn") || irc.IsAction("ponyrealism") || irc.IsAction("pony") || irc.IsAction("photon")) && irc.GetBoolArg("pe") {
		//	aiEnhancedPrompt, _ = ollama.SdPrompt(message)
		//}

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
				irc.SendError(err.Error())
			} else {
				irc.ReplyTo(upload + " - " + irc.GetActionTrigger() + irc.Action() + " " + message)

				return true
			}
		}

	}

	if irc.Channel.ImageDescribe {
		urls, err := image.ExtractURLs(irc.Event.Last())
		if err != nil {
			logger.Error("Image extraction error", "error", err)
			return true
		}

		for _, url := range urls {
			if request.IsImage(url) {
				logger.Info("Image URL detected but analysis service unavailable", "url", url)
				return true
			}
		}

	}

	return false
}

// ParseAiImageWithGPU handles image commands with explicit GPU selection
func ParseAiImageWithGPU(irc state.State, gpu meta.GPUType) bool {
	if irc.IsAction("sd") {
		if irc.GetBoolArg("help") {
			irc.Send(girc.Fmt(help.FindHelp(irc)))
			return true
		}

		if irc.GetBoolArg("models") {
			irc.Send(girc.Fmt("📸 The following sd models are available: " + comfyui.GetWorkFlows(true)))
			return true
		}
	}

	if comfyui.WorkflowExists(irc.Action()) {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp(irc)))
			return true
		}
		var aiEnhancedPrompt string
		message := comfyui.CleanPrompt(irc.Message())

		imgArg, _ := irc.GetStringArg("img", "")

		if irc.IsAction("flux-img2img") && imgArg == "" {
			irc.SendError("--img argument is required for flux-img2img")
			return true
		}

		if imgArg != "" {
			if !image.IsImageURL(imgArg) {
				irc.SendError("Invalid image URL")
				return true
			}

			if !strings.Contains(irc.Command.Action, "img") && !irc.IsAction("kontext") {
				irc.SendError("Cannot use image for this model")
				return true
			}
		}

		if (strings.Contains(irc.Command.Action, "img") || irc.IsAction("img2ltx")) && imgArg == "" {
			irc.SendError("Image URL required for this model")
			return true
		}

		if irc.IsAction("img2ltx") && message == "" {
			irc.SendError("Prompt required for ltx model")
			return true
		}

		aiEnhancedPrompt = ""
		if (irc.IsAction("ltx") || irc.IsAction("img2ltx")) || irc.GetBoolArg("pe") {
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
				irc.SendError(err.Error())
			} else {
				irc.ReplyTo(upload + " - " + irc.GetActionTrigger() + irc.Action() + " " + message)
				return true
			}
		}
	}

	return false
}
