package commands

import (
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
)

func ParseTop(irc *state.State) {
	if irc.Channel == nil {
		irc.Send("Error: top can only be used in a channel.")
		return
	}

	hours := parseHoursFromMessage(irc.Message(), 24, 168)

	if irc.Message() == argHelp {
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
	for _, et := range []string{eventPrivmsg, eventAction, eventJoin, eventPart, eventQuit, eventKick} {
		if count, ok := stats.EventCounts[et]; ok && count > 0 {
			label := et
			switch et {
			case eventPrivmsg:
				label = "messages"
			case eventAction:
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
