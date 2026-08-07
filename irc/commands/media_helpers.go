package commands

import (
	"fmt"
	"strconv"

	"aibird/http/request"
	"aibird/http/uploaders/birdhole"
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"

	meta "aibird/shared/meta"
)

// preparePromptAndDisplay handles the common pre-processing steps for media commands:
// cleaning the prompt, checking bad words, and sending the "processing" status
// message. It returns the display message (the cleaned prompt, or the bad-words
// replacement when triggered).
func preparePromptAndDisplay(irc *state.State, gpu meta.GPUType) string {
	message := comfyui.CleanPrompt(irc.Message())

	displayMessage := message
	if comfyui.BadWordsCheck(message, &irc.Config.ComfyUi) {
		displayMessage = irc.Config.ComfyUi.BadWordsPrompt
	}

	if irc.User.CanSkipQueue() {
		irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx %s🔥 processing '%s'... please wait.", irc.User.NickName, gpu, displayMessage))
	} else {
		irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, displayMessage))
	}

	return displayMessage
}

// uploadAndReply handles the common post-processing steps for media commands:
// building upload fields, uploading to BirdHole, and sending the IRC reply.
// Returns true on success.
func uploadAndReply(irc *state.State, response, displayMessage string) bool {
	fields := []request.Fields{
		{Key: "panorama", Value: strconv.FormatBool(irc.IsAction("panorama"))},
	}
	fields = append(fields, irc.BuildUploadFields()...)

	upload, err := birdhole.BirdHole(response, displayMessage, fields, &irc.Config.Birdhole)
	if err != nil {
		logger.Error("Birdhole error", "error", err)
		irc.SendError(err.Error())
		return false
	}

	irc.ReplyTo(upload + " - " + irc.GetActionTrigger() + irc.Action() + " " + displayMessage)
	return true
}
