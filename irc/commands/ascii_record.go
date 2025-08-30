package commands

import (
	"aibird/asciistore"
	"aibird/irc/state"
	"aibird/logger"
)

// ParseRecordCommand handles the !record command to save ASCII art to the recording service
func ParseRecordCommand(irc state.State) bool {
	logger.Debug("Processing record command", "user", irc.User.NickName, "network", irc.Network.NetworkName, "channel", irc.Channel.Name)

	// Get the recording URL from config
	recordingUrl := irc.Config.AiBird.AsciiRecordingUrl

	// Use the ASCII store manager to record the art
	response, err := asciistore.GetManager().RecordToService(
		irc.User.NickName,
		irc.Network.NetworkName,
		irc.Channel.Name,
		recordingUrl,
	)

	if err != nil {
		logger.Error("Failed to record ASCII art", "error", err, "user", irc.User.NickName)
		irc.SendError(response)
		return true
	}

	// Send success message to user
	irc.Send(response)
	logger.Info("Successfully processed record command", "user", irc.User.NickName, "response", response)

	return true
}
