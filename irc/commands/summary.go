package commands

import (
	"fmt"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text"
	"aibird/text/glm"
)

func ParseSummary(irc state.State) {
	if irc.Channel == nil {
		irc.Send("Error: summary can only be used in a channel.")
		return
	}

	if irc.Config.Glm.ApiKey == "" {
		irc.Send("Error: AI summary requires GLM to be configured.")
		return
	}

	if irc.Message() == "--help" {
		irc.Send("Usage: !summary [hours] [--persona <angle>] — Summarizes the last N hours of channel activity.")
		irc.Send("Examples: !summary, !summary 6h, !summary 12 --persona \"a western cowboy\"")
		irc.Send("Defaults: 24h, max: 168h (7 days). --persona: optional creative angle for the summary.")
		return
	}

	defaultHours := irc.Config.AiBird.SummaryDefaultHours
	if defaultHours <= 0 {
		defaultHours = 24
	}
	hours := parseHoursFromMessage(irc.Message(), defaultHours, 168)
	persona, _ := irc.GetStringArg("persona", "")
	persona = sanitizePersona(persona)

	maxMessages := irc.Config.AiBird.SummaryMaxMessages
	if maxMessages <= 0 {
		maxMessages = 200
	}

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, hours, maxMessages)
	if err != nil {
		logger.Error("Failed to get channel messages for summary", "error", err)
		irc.Send("Error retrieving channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No activity found in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	go generateSummary(irc, messages, hours, persona)
}

func generateSummary(irc state.State, messages []birdbase.ChannelMessage, hours int, persona string) {
	irc.Send(fmt.Sprintf("%s, generating summary of the last %dh (%d events)...", irc.User.NickName, hours, len(messages)))

	systemPrompt, err := text.GetPrompt("summary.md")
	if err != nil {
		logger.Error("Failed to load summary prompt", "error", err)
		irc.Send("Error: could not load summary prompt.")
		return
	}

	userPrompt := formatEventLog(irc.Channel.Name, hours, messages)

	if persona != "" {
		userPrompt += fmt.Sprintf("\n\nPersona/Angle for this summary: %s", persona)
	}

	answer, err := glm.SingleRequestWithSystem(systemPrompt, userPrompt, irc.Config.Glm)
	if err != nil {
		logger.Error("Failed to generate summary", "error", err)
		irc.Send("Error: failed to generate summary. Please try again later.")
		return
	}

	irc.Send(answer)
}
