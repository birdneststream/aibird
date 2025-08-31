package commands

import (
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
)

func ParseLeaderboard(irc state.State) {
	switch irc.Command.Action {
	case "leaderboard":
		handleLeaderboard(irc)
	}
}

func handleLeaderboard(irc state.State) {
	// Get the argument to determine which leaderboard to show
	arg := strings.TrimSpace(irc.Command.Message)
	
	switch arg {
	case "global":
		if !irc.User.IsOwnerUser() {
			irc.SendError("Only owners can view the global leaderboard")
			return
		}
		handleGlobalLeaderboard(irc)
	case "":
		// Default: show network-specific leaderboard
		handleNetworkLeaderboard(irc)
	default:
		// Check if it's a specific command leaderboard (owner only)
		if !irc.User.IsOwnerUser() {
			irc.SendError("Only owners can view command-specific leaderboards")
			return
		}
		handleCommandLeaderboard(irc, arg)
	}
}

func handleNetworkLeaderboard(irc state.State) {
	entries, err := birdbase.GetNetworkLeaderboard(irc.Network.NetworkName, 10)
	if err != nil {
		logger.Error("Failed to get network leaderboard", "error", err, "network", irc.Network.NetworkName)
		irc.SendError("Failed to retrieve leaderboard data")
		return
	}

	if len(entries) == 0 {
		irc.Send("📊 No command usage data available for " + irc.Network.NetworkName)
		return
	}

	// Create table header
	response := fmt.Sprintf("📊 Top %d users on %s:", len(entries), irc.Network.NetworkName)
	irc.Send(response)
	
	// Table header
	irc.Send("┌────┬─────────────────┬─────────────────┬───────┐")
	irc.Send("│ #  │ Nickname        │ Command         │ Count │")
	irc.Send("├────┼─────────────────┼─────────────────┼───────┤")
	
	// Table rows
	for i, entry := range entries {
		rank := fmt.Sprintf("%2d", i+1)
		nickname := fmt.Sprintf("%-15s", truncateString(entry.Nickname, 15))
		command := fmt.Sprintf("%-15s", truncateString(entry.Command, 15))
		count := fmt.Sprintf("%5d", entry.Count)
		
		row := fmt.Sprintf("│ %s │ %s │ %s │ %s │", rank, nickname, command, count)
		irc.Send(row)
	}
	
	// Table footer
	irc.Send("└────┴─────────────────┴─────────────────┴───────┘")
}

func handleGlobalLeaderboard(irc state.State) {
	entries, err := birdbase.GetGlobalLeaderboard(10)
	if err != nil {
		logger.Error("Failed to get global leaderboard", "error", err)
		irc.SendError("Failed to retrieve global leaderboard data")
		return
	}

	if len(entries) == 0 {
		irc.Send("📊 No command usage data available globally")
		return
	}

	// Create table header
	response := fmt.Sprintf("🌍 Global Top %d users:", len(entries))
	irc.Send(response)
	
	// Table header
	irc.Send("┌────┬─────────────────┬─────────────────┬─────────────────┬───────┐")
	irc.Send("│ #  │ Network         │ Nickname        │ Command         │ Count │")
	irc.Send("├────┼─────────────────┼─────────────────┼─────────────────┼───────┤")
	
	// Table rows
	for i, entry := range entries {
		rank := fmt.Sprintf("%2d", i+1)
		network := fmt.Sprintf("%-15s", truncateString(entry.Network, 15))
		nickname := fmt.Sprintf("%-15s", truncateString(entry.Nickname, 15))
		command := fmt.Sprintf("%-15s", truncateString(entry.Command, 15))
		count := fmt.Sprintf("%5d", entry.Count)
		
		row := fmt.Sprintf("│ %s │ %s │ %s │ %s │ %s │", rank, network, nickname, command, count)
		irc.Send(row)
	}
	
	// Table footer
	irc.Send("└────┴─────────────────┴─────────────────┴─────────────────┴───────┘")
}

func handleCommandLeaderboard(irc state.State, command string) {
	entries, err := birdbase.GetCommandLeaderboard(command, 10)
	if err != nil {
		logger.Error("Failed to get command leaderboard", "error", err, "command", command)
		irc.SendError("Failed to retrieve command leaderboard data")
		return
	}

	if len(entries) == 0 {
		irc.Send(fmt.Sprintf("📊 No usage data available for command: %s", command))
		return
	}

	// Create table header
	response := fmt.Sprintf("🎯 Top %d users for command '%s':", len(entries), command)
	irc.Send(response)
	
	// Table header
	irc.Send("┌────┬─────────────────┬─────────────────┬───────┐")
	irc.Send("│ #  │ Network         │ Nickname        │ Count │")
	irc.Send("├────┼─────────────────┼─────────────────┼───────┤")
	
	// Table rows
	for i, entry := range entries {
		rank := fmt.Sprintf("%2d", i+1)
		network := fmt.Sprintf("%-15s", truncateString(entry.Network, 15))
		nickname := fmt.Sprintf("%-15s", truncateString(entry.Nickname, 15))
		count := fmt.Sprintf("%5d", entry.Count)
		
		row := fmt.Sprintf("│ %s │ %s │ %s │ %s │", rank, network, nickname, count)
		irc.Send(row)
	}
	
	// Table footer
	irc.Send("└────┴─────────────────┴─────────────────┴───────┘")
}

// truncateString truncates a string to the specified length and adds ellipsis if needed
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}