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
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

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

// slugify converts a string to a URL-friendly slug
func slugify(input string) string {
	// Remove non-alphanumeric characters and replace with hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	slug := reg.ReplaceAllString(strings.TrimSpace(input), "-")
	// Remove leading/trailing hyphens and convert to lowercase
	slug = strings.Trim(strings.ToLower(slug), "-")
	// Limit length to 50 characters
	if len(slug) > 50 {
		slug = slug[:50]
	}
	return slug
}

// recordArt posts the IRC art to the recording URL and returns the result
func recordArt(recordingUrl, fileName, art string) (string, bool) {
	if recordingUrl == "" {
		logger.Debug("Recording URL not configured, not saving art")
		return "", false
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := strings.TrimRight(recordingUrl, "/") + "/" + fileName
	
	req, err := http.NewRequest("POST", url, strings.NewReader(art))
	if err != nil {
		logger.Error("Failed to create record art request", "error", err)
		return "failed to record art :(", false
	}
	
	req.Header.Set("Content-Type", "text/plain")
	
	res, err := client.Do(req)
	if err != nil || res.StatusCode != 200 {
		logger.Error("Failed to record art", "error", err, "status", res.StatusCode)
		return "failed to record art :(", false
	}
	
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Error("Failed to read record art response", "error", err)
		return "maybe failed to record art? try " + fileName + " :(", false
	}
	
	return "art saved to " + string(body), true
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
				logger.Debug("Processing aiscii command", "file", response)
				
				// Create a copy for IRC art processing (since Birdhole deletes the original)
				copyPath := response + "_copy"
				if err := copyFile(response, copyPath); err != nil {
					logger.Error("Failed to copy file for IRC art processing", "error", err)
					irc.SendError("Failed to copy image file: " + err.Error())
					return true
				}
				
				// Upload original to Birdhole (this will delete the original file)
				fields := []request.Fields{
					{Key: "panorama", Value: "false"},
					{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
					{Key: "meta_network", Value: irc.Network.NetworkName},
					{Key: "meta_nick", Value: irc.User.NickName},
					{Key: "meta_prompt", Value: message},
				}
				
				upload, err := birdhole.BirdHole(response, message, fields, irc.Config.Birdhole)
				if err != nil {
					logger.Error("Birdhole upload failed for aiscii", "error", err)
					// Clean up copy on error
					os.Remove(copyPath)
					irc.SendError("Failed to upload image: " + err.Error())
					return true
				}
				
				// Send the Birdhole link first
				irc.Send(fmt.Sprintf("🖼️ Original image: %s", upload))
				
				// Now convert to IRC art using the copy
				useHalfblocks := !irc.GetBoolArg("fullblocks") // Invert: default to halfblocks unless --fullblocks is specified
				ircArtLines, err := ircart.ConvertPNGToIRCArt(copyPath, useHalfblocks)
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
				irc.Send(fmt.Sprintf("🎨 IRC Art for '%s':", message))
				
				// Send each line of IRC art
				for _, line := range formattedLines {
					irc.Send(line)
				}

				// Record the art to the recording URL (unless --norecord is specified)
				if !irc.GetBoolArg("norecord") && irc.Config.AiBird.AsciiRecordingUrl != "" {
					// Use slugified prompt as filename, trimmed to 250 chars
					trimmedMessage := message
					if len(trimmedMessage) > 250 {
						trimmedMessage = trimmedMessage[:250]
					}
					fileName := slugify(trimmedMessage)
					if fileName == "" {
						fileName = "unnamed-art"
					}
					
					// Join all lines to create the full art string
					fullArt := strings.Join(ircArtLines, "\n")
					recordResult, success := recordArt(irc.Config.AiBird.AsciiRecordingUrl, fileName, fullArt)
					if success {
						irc.Send(recordResult)
					} else if recordResult != "" {
						irc.Send(recordResult)
					}
				}

				// Clean up the copy file after processing
				if err := os.Remove(copyPath); err != nil {
					logger.Debug("Failed to remove copy file", "file", copyPath, "error", err)
				}
				
				return true
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
				logger.Debug("Processing aiscii command in GPU function", "file", response)
				
				// Create a copy for IRC art processing (since Birdhole deletes the original)
				copyPath := response + "_copy"
				if err := copyFile(response, copyPath); err != nil {
					logger.Error("Failed to copy file for IRC art processing", "error", err)
					irc.SendError("Failed to copy image file: " + err.Error())
					return true
				}
				
				// Upload original to Birdhole (this will delete the original file)
				fields := []request.Fields{
					{Key: "panorama", Value: "false"},
					{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
					{Key: "meta_network", Value: irc.Network.NetworkName},
					{Key: "meta_nick", Value: irc.User.NickName},
					{Key: "meta_prompt", Value: message},
				}
				
				upload, err := birdhole.BirdHole(response, message, fields, irc.Config.Birdhole)
				if err != nil {
					logger.Error("Birdhole upload failed for aiscii", "error", err)
					// Clean up copy on error
					os.Remove(copyPath)
					irc.SendError("Failed to upload image: " + err.Error())
					return true
				}
				
				// Send the Birdhole link first
				irc.Send(fmt.Sprintf("🖼️ Original image: %s", upload))
				
				// Now convert to IRC art using the copy
				useHalfblocks := !irc.GetBoolArg("fullblocks") // Invert: default to halfblocks unless --fullblocks is specified
				ircArtLines, err := ircart.ConvertPNGToIRCArt(copyPath, useHalfblocks)
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
				irc.Send(fmt.Sprintf("🎨 IRC Art for '%s':", message))
				
				// Send each line of IRC art
				for _, line := range formattedLines {
					irc.Send(line)
				}

				// Record the art to the recording URL (unless --norecord is specified)
				if !irc.GetBoolArg("norecord") && irc.Config.AiBird.AsciiRecordingUrl != "" {
					// Use slugified prompt as filename, trimmed to 250 chars
					trimmedMessage := message
					if len(trimmedMessage) > 250 {
						trimmedMessage = trimmedMessage[:250]
					}
					fileName := slugify(trimmedMessage)
					if fileName == "" {
						fileName = "unnamed-art"
					}
					
					// Join all lines to create the full art string
					fullArt := strings.Join(ircArtLines, "\n")
					recordResult, success := recordArt(irc.Config.AiBird.AsciiRecordingUrl, fileName, fullArt)
					if success {
						irc.Send(recordResult)
					} else if recordResult != "" {
						irc.Send(recordResult)
					}
				}

				// Clean up the copy file after processing
				if err := os.Remove(copyPath); err != nil {
					logger.Debug("Failed to remove copy file", "file", copyPath, "error", err)
				}
				
				return true
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
