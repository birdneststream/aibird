package commands

import (
	"fmt"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
)

func ParseRecall(irc state.State) {
	if irc.Channel == nil {
		irc.Send("Error: recall can only be used in a channel.")
		return
	}

	if irc.Message() == "" || irc.Message() == "--help" {
		irc.Send("Usage: !recall <keyword> — Searches recent channel history for a keyword.")
		irc.Send("Example: !recall bird, !recall \"good morning\"")
		return
	}

	keyword := strings.TrimSpace(irc.Message())
	if len(keyword) > 100 {
		keyword = keyword[:100]
	}

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.SearchChannelMessages(networkID, irc.Channel.Name, keyword, 8)
	if err != nil {
		logger.Error("Failed to search channel messages", "error", err, "keyword", keyword)
		irc.Send("Error searching channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No messages found matching \"%s\" in the last 7 days.", keyword))
		return
	}

	irc.Send(fmt.Sprintf("Found %d messages matching \"%s\":", len(messages), keyword))

	for _, msg := range messages {
		when := time.Unix(msg.Timestamp, 0).Format("Jan 02 15:04")
		switch msg.EventType {
		case "privmsg":
			irc.Send(fmt.Sprintf("[%s] <%s> %s", when, msg.Nickname, truncateMessage(msg.Message, 200)))
		case "action":
			irc.Send(fmt.Sprintf("[%s] * %s %s", when, msg.Nickname, truncateMessage(msg.Message, 200)))
		}
	}
}

func truncateMessage(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen] + "..."
}
