package commands

import (
	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/image/comfyui"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/status"
	"aibird/text"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	meta "aibird/shared/meta"

	"github.com/lrstanley/girc"
)

func convertAudioToMp3AndAmplify(inputFile string) (string, error) {
	outputFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".mp3"
	// Validate input file path to prevent command injection
	if !filepath.IsAbs(inputFile) && strings.Contains(inputFile, "..") {
		return "", fmt.Errorf("invalid file path: %s", inputFile)
	}
	cmd := exec.Command("ffmpeg", "-i", inputFile, "-af", "volume=1.1", "-y", outputFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to convert audio: %w. Output: %s", err, string(output))
	}
	return outputFile, nil
}

func ProcessAndUploadAudio(irc state.State, message, response string) {
	audioFile, err := comfyui.Process(irc, "", meta.GPU4090)
	if err != nil {
		logger.Error("Failed to process comfyui request", "error", err)
		irc.SendError(err.Error())
		return
	}
	defer os.Remove(audioFile)

	var finalFile string
	if filepath.Ext(audioFile) == ".mp3" {
		finalFile = audioFile
	} else {
		convertedFile, err := convertAudioToMp3AndAmplify(audioFile)
		if err != nil {
			logger.Error("Failed to convert audio file", "error", err)
			irc.SendError("Failed to process audio file.")
			return
		}
		finalFile = convertedFile
		defer os.Remove(finalFile)
	}

	fields := []request.Fields{
		{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
		{Key: "meta_network", Value: irc.Network.NetworkName},
		{Key: "meta_channel", Value: irc.Channel.Name},
		{Key: "meta_user", Value: irc.User.NickName},
		{Key: "meta_ident", Value: irc.User.Ident},
		{Key: "meta_host", Value: irc.User.Host},
	}

	upload, err := birdhole.BirdHole(finalFile, message, fields, irc.Config.Birdhole)
	if err != nil {
		logger.Error("Failed to upload to birdhole", "error", err)
		irc.SendError(err.Error())
	} else {
		irc.ReplyTo(upload + " - " + response)
	}
}

func ParseAiSound(irc state.State) bool {
	if irc.IsAction("tts") {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp("tts", irc.Config.AiBird)))
			return true
		}

		systemStatus := status.NewClient(irc.Config.AiBird)
		voice, _ := irc.GetStringArg("voice", "woman")
		validVoices, err := systemStatus.GetWavs()
		if err != nil {
			irc.SendError("could not fetch voice list from api")
			return true
		}

		isValid := false
		for _, v := range validVoices {
			if v == voice {
				isValid = true
				break
			}
		}

		if !isValid {
			irc.SendError(fmt.Sprintf("invalid voice '%s'. Valid voices are: %s", voice, strings.Join(validVoices, ", ")))
			return true
		}

		message := text.AppendFullStop(irc.Message())

		irc.ReplyTo("🔊 Processing TTS request, please wait...")
		ProcessAndUploadAudio(irc, message, message)

		return true
	}

	if irc.IsAction("tts-add") {
		if !irc.User.IsAdmin && irc.User.AccessLevel < 4 {
			irc.SendError("⛔️ Sorry pal you must at least be Golden Toucans tier on Patreon to use this, check out !support for more info.")
			return true
		}

		url, _ := irc.GetStringArg("url", "")
		name, _ := irc.GetStringArg("name", "")
		start, _ := irc.GetStringArg("start", "")
		duration, _ := irc.GetStringArg("duration", "")

		if irc.GetBoolArg("help") || url == "" || name == "" {
			irc.Send(girc.Fmt(help.FindHelp("tts-add", irc.Config.AiBird)))
			return true
		}

		irc.ReplyTo(fmt.Sprintf("Adding new voice '%s' from %s. Please wait...", name, url))

		statusClient := status.NewClient(irc.Config.AiBird)
		message, err := statusClient.AddVoice(url, name, start, duration)
		if err != nil {
			irc.SendError(fmt.Sprintf("Failed to add voice: %s", err.Error()))
			return true
		}

		irc.ReplyTo(message)
		return true
	}

	if irc.IsAction("music") {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp("music", irc.Config.AiBird)))
			return true
		}

		if steps, ok := irc.GetIntArg("steps", 30); ok {
			if steps < 15 || steps > 45 {
				irc.SendError("steps must be between 15 and 45")
				return true
			}
		}

		if cfgStr, _ := irc.GetStringArg("cfg", "4.0"); cfgStr != "" {
			if cfg, err := strconv.ParseFloat(cfgStr, 64); err == nil {
				if cfg < 1.0 || cfg > 8.0 {
					irc.SendError("cfg must be between 1.0 and 8.0")
					return true
				}
			} else {
				irc.SendError("Invalid value for cfg, must be a number.")
				return true
			}
		}

		if genType, _ := irc.GetStringArg("type", "quality"); genType != "quality" && genType != "speed" {
			irc.SendError("type must be 'quality' or 'speed'")
			return true
		}

		irc.ReplyTo("🎵 Processing music request, please wait...")
		ProcessAndUploadAudio(irc, irc.Message(), irc.Message())

		return true
	}

	if irc.IsAction("sound") {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp("sound", irc.Config.AiBird)))
			return true
		}

		irc.ReplyTo("🔊 Processing sound request, please wait...")
		ProcessAndUploadAudio(irc, irc.Message(), irc.Message())

		return true
	}

	return false
}

// ProcessAndUploadAudioWithGPU handles audio processing with explicit GPU selection
func ProcessAndUploadAudioWithGPU(irc state.State, message, response string, gpu meta.GPUType) {
	audioFile, err := comfyui.Process(irc, "", gpu)
	if err != nil {
		logger.Error("Failed to process comfyui request", "error", err)
		irc.SendError(err.Error())
		return
	}
	defer os.Remove(audioFile)

	var finalFile string
	if filepath.Ext(audioFile) == ".mp3" {
		finalFile = audioFile
	} else {
		convertedFile, err := convertAudioToMp3AndAmplify(audioFile)
		if err != nil {
			logger.Error("Failed to convert audio file", "error", err)
			irc.SendError("Failed to process audio file.")
			return
		}
		finalFile = convertedFile
		defer os.Remove(finalFile)
	}

	fields := []request.Fields{
		{Key: "tags", Value: irc.Action() + "," + irc.Network.NetworkName},
		{Key: "meta_network", Value: irc.Network.NetworkName},
		{Key: "meta_channel", Value: irc.Channel.Name},
		{Key: "meta_user", Value: irc.User.NickName},
		{Key: "meta_ident", Value: irc.User.Ident},
		{Key: "meta_host", Value: irc.User.Host},
	}

	upload, err := birdhole.BirdHole(finalFile, message, fields, irc.Config.Birdhole)
	if err != nil {
		logger.Error("Failed to upload to birdhole", "error", err)
		irc.SendError(err.Error())
	} else {
		irc.ReplyTo(upload + " - " + response)
	}
}

// ParseAiSoundWithGPU handles sound commands with explicit GPU selection
func ParseAiSoundWithGPU(irc state.State, gpu meta.GPUType) bool {
	if irc.IsAction("tts") {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp("tts", irc.Config.AiBird)))
			return true
		}

		systemStatus := status.NewClient(irc.Config.AiBird)
		voice, _ := irc.GetStringArg("voice", "woman")
		validVoices, err := systemStatus.GetWavs()
		if err != nil {
			irc.SendError("could not fetch voice list from api")
			return true
		}

		isValid := false
		for _, v := range validVoices {
			if v == voice {
				isValid = true
				break
			}
		}

		if !isValid {
			irc.SendError(fmt.Sprintf("invalid voice '%s'. Valid voices are: %s", voice, strings.Join(validVoices, ", ")))
			return true
		}

		irc.ReplyTo("🔊 Processing TTS request, please wait...")
		ProcessAndUploadAudioWithGPU(irc, irc.Message(), irc.Message(), gpu)

		return true
	}

	if irc.IsAction("tts-add") {
		if !irc.User.IsAdmin && irc.User.AccessLevel < 4 {
			irc.SendError("⛔️ Sorry pal you must at least be Golden Toucans tier on Patreon to use this, check out !support for more info.")
			return true
		}

		url, _ := irc.GetStringArg("url", "")
		name, _ := irc.GetStringArg("name", "")
		start, _ := irc.GetStringArg("start", "")
		duration, _ := irc.GetStringArg("duration", "")

		if irc.GetBoolArg("help") || url == "" || name == "" {
			irc.Send(girc.Fmt(help.FindHelp("tts-add", irc.Config.AiBird)))
			return true
		}

		irc.ReplyTo(fmt.Sprintf("Adding new voice '%s' from %s. Please wait...", name, url))

		statusClient := status.NewClient(irc.Config.AiBird)
		message, err := statusClient.AddVoice(url, name, start, duration)
		if err != nil {
			irc.SendError(fmt.Sprintf("Failed to add voice: %s", err.Error()))
			return true
		}

		irc.ReplyTo(message)
		return true
	}

	if irc.IsAction("music") {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp("music", irc.Config.AiBird)))
			return true
		}

		if steps, ok := irc.GetIntArg("steps", 30); ok {
			if steps < 15 || steps > 45 {
				irc.SendError("steps must be between 15 and 45")
				return true
			}
		}

		if cfgStr, _ := irc.GetStringArg("cfg", "4.0"); cfgStr != "" {
			if cfg, err := strconv.ParseFloat(cfgStr, 64); err == nil {
				if cfg < 1.0 || cfg > 8.0 {
					irc.SendError("cfg must be between 1.0 and 8.0")
					return true
				}
			} else {
				irc.SendError("Invalid value for cfg, must be a number.")
				return true
			}
		}

		if genType, _ := irc.GetStringArg("type", "quality"); genType != "quality" && genType != "speed" {
			irc.SendError("type must be 'quality' or 'speed'")
			return true
		}

		irc.ReplyTo("🎵 Processing music request, please wait...")
		ProcessAndUploadAudioWithGPU(irc, irc.Message(), irc.Message(), gpu)

		return true
	}

	if irc.IsAction("sound") {
		if irc.GetBoolArg("help") || irc.IsEmptyMessage() {
			irc.Send(girc.Fmt(help.FindHelp("sound", irc.Config.AiBird)))
			return true
		}

		irc.ReplyTo("🔊 Processing sound request, please wait...")
		ProcessAndUploadAudioWithGPU(irc, irc.Message(), irc.Message(), gpu)

		return true
	}

	return false
}
