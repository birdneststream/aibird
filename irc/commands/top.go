package commands

import (
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
)

func ParseTop(irc state.State) {
	if irc.Channel == nil {
		irc.Send("Error: top can only be used in a channel.")
		return
	}

	hours := 24
	if irc.Message() != "" && irc.Message() != "--help" {
		msg := strings.TrimSpace(irc.Message())
		cleaned := strings.TrimSuffix(msg, "h")
		if parsed, err := parseIntSafe(cleaned); err == nil && parsed > 0 && parsed <= 168 {
			hours = parsed
		}
	}

	if irc.Message() == "--help" {
		irc.Send("Usage: !top [hours] — Shows channel activity stats.")
		irc.Send("Example: !top, !top 6h, !top 168h")
		return
	}

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	stats, err := birdbase.GetChannelStats(networkID, irc.Channel.Name, hours)
	if err != nil {
		logger.Error("Failed to get channel stats", "error", err)
		irc.Send("Error retrieving channel stats.")
		return
	}

	if stats.TotalMessages == 0 {
		irc.Send(fmt.Sprintf("No activity in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	// Build summary line
	var parts []string
	parts = append(parts, fmt.Sprintf("📊 %s activity (last %dh):", irc.Channel.Name, hours))
	parts = append(parts, fmt.Sprintf("%d total events", stats.TotalMessages))

	// Event breakdown
	var eventParts []string
	for _, et := range []string{"privmsg", "action", "join", "part", "quit", "kick"} {
		if count, ok := stats.EventCounts[et]; ok && count > 0 {
			label := et
			switch et {
			case "privmsg":
				label = "messages"
			case "action":
				label = "actions"
			}
			eventParts = append(eventParts, fmt.Sprintf("%d %s", count, label))
		}
	}
	if len(eventParts) > 0 {
		parts = append(parts, fmt.Sprintf("(%s)", strings.Join(eventParts, ", ")))
	}

	irc.Send(strings.Join(parts, " "))

	// Top chatters
	if len(stats.TopChatters) > 0 {
		var chatterParts []string
		limit := 5
		if len(stats.TopChatters) < limit {
			limit = len(stats.TopChatters)
		}
		for i := 0; i < limit; i++ {
			chatterParts = append(chatterParts, fmt.Sprintf("%s (%d)", stats.TopChatters[i].Nickname, stats.TopChatters[i].Count))
		}
		irc.Send(fmt.Sprintf("🏆 Top chatters: %s", strings.Join(chatterParts, ", ")))
	}
}

func parseIntSafe(s string) (int, error) {
	var result int
	for _, c := range strings.TrimSpace(s) {
		if c < '0' || c > '9' {
			break
		}
		result = result*10 + int(c-'0')
	}
	if result == 0 {
		return 0, fmt.Errorf("not a number")
	}
	return result, nil
}
