package commands

import (
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text"
	"aibird/text/glm"
)

func ParseMood(irc state.State) {
	if irc.Channel == nil {
		irc.Send("Error: mood can only be used in a channel.")
		return
	}

	if irc.Config.Glm.ApiKey == "" {
		irc.Send("Error: AI mood requires GLM to be configured.")
		return
	}

	if irc.Message() == "--help" {
		irc.Send("Usage: !mood [hours] — Analyzes the current mood/vibe of the channel.")
		irc.Send("Example: !mood, !mood 6h")
		return
	}

	hours := 24
	if irc.Message() != "" {
		hours = parseHoursFromMessage(irc.Message(), 24, 168)
	}

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, hours, 200)
	if err != nil {
		logger.Error("Failed to get channel messages for mood", "error", err)
		irc.Send("Error retrieving channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No activity found in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	go generateMood(irc, messages, hours)
}

func generateMood(irc state.State, messages []birdbase.ChannelMessage, hours int) {
	irc.Send(fmt.Sprintf("%s, reading the room for the last %dh...", irc.User.NickName, hours))

	systemPrompt, err := text.GetPrompt("mood.md")
	if err != nil {
		logger.Error("Failed to load mood prompt", "error", err)
		irc.Send("Error: could not load mood prompt.")
		return
	}

	userPrompt := formatEventLog(irc.Channel.Name, hours, messages)

	answer, err := glm.SingleRequestWithSystem(systemPrompt, userPrompt, irc.Config.Glm)
	if err != nil {
		logger.Error("Failed to generate mood", "error", err)
		irc.Send("Error: failed to analyze the mood. Please try again later.")
		return
	}

	irc.Send(fmt.Sprintf("🌡️ %s", answer))
}

// sanitizePersona strips newlines, carriage returns, and truncates to prevent prompt injection.
func sanitizePersona(input string) string {
	s := strings.ReplaceAll(input, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// formatEventLog builds a conversation log from messages for AI consumption.
func formatEventLog(channelName string, hours int, messages []birdbase.ChannelMessage) string {
	var logBuilder strings.Builder
	logBuilder.WriteString(fmt.Sprintf("Channel: %s | Time period: last %d hours\n\n", channelName, hours))

	for _, msg := range messages {
		switch msg.EventType {
		case "privmsg":
			logBuilder.WriteString(fmt.Sprintf("<%s> %s\n", msg.Nickname, msg.Message))
		case "action":
			logBuilder.WriteString(fmt.Sprintf("* %s %s\n", msg.Nickname, msg.Message))
		case "join":
			logBuilder.WriteString(fmt.Sprintf("--> %s joined\n", msg.Nickname))
		case "part":
			logBuilder.WriteString(fmt.Sprintf("<-- %s left\n", msg.Nickname))
		case "quit":
			logBuilder.WriteString(fmt.Sprintf("<-- %s quit\n", msg.Nickname))
		case "kick":
			logBuilder.WriteString(fmt.Sprintf("<-- %s was kicked by %s\n", msg.Nickname, msg.Message))
		}
	}

	return logBuilder.String()
}

// parseHoursFromMessage extracts a leading number from a message, with min/max bounds.
func parseHoursFromMessage(msg string, defaultHours, maxHours int) int {
	msg = strings.TrimSpace(msg)
	firstToken := msg
	if idx := strings.IndexByte(msg, ' '); idx > 0 {
		firstToken = msg[:idx]
	}
	cleaned := strings.TrimSuffix(firstToken, "h")
	if parsed, err := parseIntSafe(cleaned); err == nil && parsed > 0 {
		if parsed > maxHours {
			return maxHours
		}
		return parsed
	}
	return defaultHours
}
