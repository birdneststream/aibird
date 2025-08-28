package commands

import (
	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/image"
	"aibird/image/comfyui"
	"aibird/image/ircart"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text/ollama"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	meta "aibird/shared/meta"

	"github.com/lrstanley/girc"
)

// copyFile creates a copy of the source file at the destination path
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// convertToDetailURL converts a direct image URL to a Birdhole detail page URL
// Example: https://domain.com/abc123.jpg -> https://domain.com/detail/abc123.jpg
func convertToDetailURL(imageURL string) string {
	// Find the last slash and insert "/detail" before the filename
	lastSlash := strings.LastIndex(imageURL, "/")
	if lastSlash == -1 {
		return imageURL // Return original if no slash found
	}

	return imageURL[:lastSlash] + "/detail" + imageURL[lastSlash:]
}

// processAisciiCommand handles the aiscii command processing for both ParseAiImage and ParseAiImageWithGPU
func processAisciiCommand(irc state.State, response, message string) bool {
	logger.Debug("Processing aiscii command", "file", response)

	// Create a copy for IRC art processing (since Birdhole deletes the original)
	copyPath := response + "_copy"
	if err := copyFile(response, copyPath); err != nil {
		logger.Error("Failed to copy file for IRC art processing", "error", err)
		irc.SendError("Failed to copy image file: " + err.Error())
		return true
	}

	// Upload original to Birdhole (this will delete the original file)
	// Use BirdHolePNG to preserve PNG metadata for aiscii
	fields := []request.Fields{
		{Key: "panorama", Value: "false"},
		{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
		{Key: "meta_network", Value: irc.Network.NetworkName},
		{Key: "meta_nick", Value: irc.User.NickName},
		{Key: "meta_prompt", Value: message},
	}

	upload, err := birdhole.BirdHolePNG(response, message, fields, irc.Config.Birdhole)
	if err != nil {
		logger.Error("Birdhole upload failed for aiscii", "error", err)
		// Clean up copy on error
		os.Remove(copyPath)
		irc.SendError("Failed to upload image: " + err.Error())
		return true
	}

	// Now convert to IRC art using the copy
	useHalfblocks := !irc.GetBoolArg("fullblocks") // Invert: default to halfblocks unless --fullblocks is specified
	ircArtLines, err := ircart.ExtractOrConvertIRCArt(copyPath, useHalfblocks)
	if err != nil {
		logger.Error("IRC art conversion failed", "error", err)
		// Clean up copy on error
		os.Remove(copyPath)
		irc.SendError("Failed to convert image to IRC art: " + err.Error())
		return true
	}

	// Format the IRC art for sending
	formattedLines := ircart.FormatIRCArtForIRC(ircArtLines)

	// Send a header message
	detailURL := convertToDetailURL(upload)
	irc.Send(fmt.Sprintf("🎨 IRC Art for '%s' download the ascii here %s:", message, detailURL))

	// Send each line of IRC art
	for _, line := range formattedLines {
		irc.Client.Cmd.SendRaw(fmt.Sprintf("PRIVMSG %s :%s", irc.Channel.Name, line))
	}

	// Clean up the copy file after processing
	if err := os.Remove(copyPath); err != nil {
		logger.Debug("Failed to remove copy file", "file", copyPath, "error", err)
	}

	return true
}

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

			// Special handling for aiscii command - convert to IRC art instead of uploading
			logger.Debug("Checking aiscii action", "action", irc.Action(), "isAiscii", irc.IsAction("aiscii"))
			if irc.IsAction("aiscii") {
				return processAisciiCommand(irc, response, message)
			}

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

			// Special handling for aiscii command - convert to IRC art instead of uploading
			logger.Debug("Checking aiscii action in GPU function", "action", irc.Action(), "isAiscii", irc.IsAction("aiscii"))
			if irc.IsAction("aiscii") {
				return processAisciiCommand(irc, response, message)
			}

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
