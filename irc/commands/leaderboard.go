package commands

import (
	"fmt"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
)

func ParseLeaderboard(irc *state.State) {
	switch irc.Command.Action {
	case cmdLeaderboard:
		handleLeaderboard(irc)
	}
}

func handleLeaderboard(irc *state.State) {
	arg := irc.Command.Message

	switch arg {
	case "global":
		if !irc.User.IsOwnerUser() {
			irc.SendError("Only owners can view the global leaderboard")
			return
		}
		handleGlobalLeaderboard(irc)
	case "":
		handleNetworkLeaderboard(irc)
	default:
		if !irc.User.IsOwnerUser() {
			irc.SendError("Only owners can view command-specific leaderboards")
			return
		}
		handleCommandLeaderboard(irc, arg)
	}
}

// leaderboard header constants
const (
	hdrRank     = "#"
	hdrNickname = "Nickname"
	hdrNetwork  = "Network"
	hdrCommand  = "Command"
	hdrCount    = "Count"
)

func handleNetworkLeaderboard(irc *state.State) {
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

	irc.Send(fmt.Sprintf("📊 Top %d users on %s:", len(entries), irc.Network.NetworkName))

	rows := make([][]string, len(entries))
	for i, entry := range entries {
		rows[i] = []string{
			fmt.Sprintf("%2d", i+1),
			truncateString(entry.Nickname, 15),
			truncateString(entry.Command, 15),
			fmt.Sprintf("%5d", entry.Count),
		}
	}

	for _, line := range formatLeaderboardTable(
		[]int{4, 17, 17, 7},
		[]string{hdrRank, hdrNickname, hdrCommand, hdrCount},
		rows,
	) {
		irc.Send(line)
	}
}

func handleGlobalLeaderboard(irc *state.State) {
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

	irc.Send(fmt.Sprintf("🌍 Global Top %d users:", len(entries)))

	rows := make([][]string, len(entries))
	for i, entry := range entries {
		rows[i] = []string{
			fmt.Sprintf("%2d", i+1),
			truncateString(entry.Network, 15),
			truncateString(entry.Nickname, 15),
			truncateString(entry.Command, 15),
			fmt.Sprintf("%5d", entry.Count),
		}
	}

	for _, line := range formatLeaderboardTable(
		[]int{4, 17, 17, 17, 7},
		[]string{hdrRank, hdrNetwork, hdrNickname, hdrCommand, hdrCount},
		rows,
	) {
		irc.Send(line)
	}
}

func handleCommandLeaderboard(irc *state.State, command string) {
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

	irc.Send(fmt.Sprintf("🎯 Top %d users for command '%s':", len(entries), command))

	rows := make([][]string, len(entries))
	for i, entry := range entries {
		rows[i] = []string{
			fmt.Sprintf("%2d", i+1),
			truncateString(entry.Network, 15),
			truncateString(entry.Nickname, 15),
			fmt.Sprintf("%5d", entry.Count),
		}
	}

	for _, line := range formatLeaderboardTable(
		[]int{4, 17, 17, 7},
		[]string{hdrRank, hdrNetwork, hdrNickname, hdrCount},
		rows,
	) {
		irc.Send(line)
	}
}

// leaderboardColumn describes a column in the leaderboard table.
type leaderboardColumn struct {
	header string
	width  int // total column width including padding
}

// formatLeaderboardTable builds a formatted table with border lines.
// colWidths is the total width of each column (including 1-space padding on each side).
// headers and each row must have the same number of elements as colWidths.
// Returns lines in order: header border, header row, separator, data rows, footer border.
func formatLeaderboardTable(colWidths []int, headers []string, rows [][]string) []string {
	if len(colWidths) != len(headers) {
		return nil
	}

	cols := make([]leaderboardColumn, len(colWidths))
	for i, w := range colWidths {
		cols[i] = leaderboardColumn{header: headers[i], width: w}
	}

	var lines []string

	// Header border: ┌────┬──────┐
	lines = append(lines, buildBorder(cols, "┌", "┬", "┐"))

	// Header row: │ #  │ Name │
	lines = append(lines, buildRow(cols, func(i int) string {
		return centerText(headers[i], cols[i].width)
	}))

	// Separator: ├────┼──────┤
	lines = append(lines, buildBorder(cols, "├", "┼", "┤"))

	// Data rows
	for _, row := range rows {
		lines = append(lines, buildRow(cols, func(i int) string {
			if i < len(row) {
				return centerText(row[i], cols[i].width)
			}
			return centerText("", cols[i].width)
		}))
	}

	// Footer border: └────┴──────┘
	lines = append(lines, buildBorder(cols, "└", "┴", "┘"))

	return lines
}

// buildBorder creates a horizontal border line.
func buildBorder(cols []leaderboardColumn, left, mid, right string) string {
	s := left
	for i, col := range cols {
		for j := 0; j < col.width; j++ {
			s += "─"
		}
		if i < len(cols)-1 {
			s += mid
		}
	}
	return s + right
}

// buildRow creates a data row line.
func buildRow(cols []leaderboardColumn, getCell func(int) string) string {
	s := "│"
	for i := range cols {
		s += getCell(i)
		s += "│"
	}
	return s
}

// centerText pads text to the given width with spaces on both sides.
func centerText(text string, width int) string {
	// Left pad 1 space, right pad to fill
	padded := " " + text
	for len(padded) < width-1 {
		padded += " "
	}
	padded += " "
	if len(padded) > width {
		padded = padded[:width]
	}
	return padded
}
