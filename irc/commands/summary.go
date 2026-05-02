package commands

import (
	"fmt"
	"strconv"
	"strings"

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

	// Check that GLM is configured
	if irc.Config.Glm.ApiKey == "" {
		irc.Send("Error: AI summary requires GLM to be configured.")
		return
	}

	if irc.Message() == "--help" {
		irc.Send("Usage: !summary [hours] [persona] — Summarizes the last N hours of channel activity.")
		irc.Send("Examples: !summary, !summary 6h, !summary 12h from the view of a pirate, !summary a grumpy cat")
		irc.Send("Defaults: 24h, max: 168h (7 days). Persona: optional creative angle for the summary.")
		return
	}

	// Use config default hours if set, otherwise 24
	hours := irc.Config.AiBird.SummaryDefaultHours
	if hours <= 0 {
		hours = 24
	}

	persona := ""

	if irc.Message() != "" {
		msg := strings.TrimSpace(irc.Message())

		// Try to parse a leading number as hours (e.g., "6", "6h", "12h")
		// Remaining text after hours is treated as the persona argument
		hoursStr := msg
		rest := ""

		if idx := strings.IndexByte(msg, ' '); idx > 0 {
			hoursStr = msg[:idx]
			rest = strings.TrimSpace(msg[idx+1:])
		}

		// Try parsing hours
		cleaned := strings.TrimSuffix(hoursStr, "h")
		if parsed, err := strconv.Atoi(cleaned); err == nil && parsed > 0 {
			if parsed > 168 {
				parsed = 168
			}
			hours = parsed
			persona = rest
		} else {
			// No valid hours prefix — entire message is the persona
			persona = msg
		}
	}

	// Sanitize persona: strip newlines and control characters to prevent prompt injection
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

	go generateAndSendSummary(irc, messages, hours, persona)
}

// sanitizePersona strips newlines, carriage returns, and truncates to prevent prompt injection.
func sanitizePersona(input string) string {
	// Strip newlines and carriage returns
	s := strings.ReplaceAll(input, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)

	// Truncate to reasonable length
	if len(s) > 200 {
		s = s[:200]
	}

	return s
}

func generateAndSendSummary(irc state.State, messages []birdbase.ChannelMessage, hours int, persona string) {
	irc.Send(fmt.Sprintf("%s, generating summary of the last %dh (%d events)...", irc.User.NickName, hours, len(messages)))

	systemPrompt, err := text.GetPrompt("summary.md")
	if err != nil {
		logger.Error("Failed to load summary prompt", "error", err)
		irc.Send("Error: could not load summary prompt.")
		return
	}

	var logBuilder strings.Builder
	for _, msg := range messages {
		switch msg.EventType {
		case "privmsg":
			logBuilder.WriteString(fmt.Sprintf("<%s> %s\n", msg.Nickname, msg.Message))
		case "action":
			logBuilder.WriteString(fmt.Sprintf("* %s %s\n", msg.Nickname, msg.Message))
		case "join":
			logBuilder.WriteString(fmt.Sprintf("--> %s joined\n", msg.Nickname))
		case "part":
			partMsg := ""
			if msg.Message != "" {
				partMsg = fmt.Sprintf(" (%s)", msg.Message)
			}
			logBuilder.WriteString(fmt.Sprintf("<-- %s left%s\n", msg.Nickname, partMsg))
		case "quit":
			quitMsg := ""
			if msg.Message != "" {
				quitMsg = fmt.Sprintf(" (%s)", msg.Message)
			}
			logBuilder.WriteString(fmt.Sprintf("<-- %s quit%s\n", msg.Nickname, quitMsg))
		case "kick":
			logBuilder.WriteString(fmt.Sprintf("<-- %s was kicked by %s\n", msg.Nickname, msg.Message))
		}
	}

	userPrompt := fmt.Sprintf("Channel: %s | Time period: last %d hours\n\n%s", irc.Channel.Name, hours, logBuilder.String())

	// Append persona if provided
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
