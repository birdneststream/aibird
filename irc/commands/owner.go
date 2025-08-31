package commands

import (
	"fmt"
	"os"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/state"
)

func ParseOwner(irc state.State) {
	if irc.User.IsOwner {
		switch irc.Command.Action {
		case "save":
			irc.Network.Save()
			irc.ReplyTo("Saved databases")
		case "ip":
			ip, _ := helpers.GetIp()
			irc.ReplyTo(ip)
		case "raw":
			_ = irc.Client.Cmd.SendRaw(irc.Command.Message)
		case "dbstats":
			handleDbStats(irc)
		}
	}
}

func handleDbStats(irc state.State) {
	// Get database stats from SQLite
	stats, err := birdbase.GetDatabaseStats()
	if err != nil {
		irc.ReplyTo(fmt.Sprintf("Error getting database stats: %v", err))
		return
	}

	// Get file system size of SQLite database file
	dbPath := "bird.db"
	fileInfo, err := os.Stat(dbPath)
	var fileSize int64
	if err != nil {
		irc.ReplyTo(fmt.Sprintf("Error getting database file size: %v", err))
		return
	}
	fileSize = fileInfo.Size()

	// Format size in human readable format
	fileSizeStr := formatBytes(fileSize)

	// Get internal SQLite size calculation
	sizeVal, ok := stats["size"].(int64)
	if !ok {
		sizeVal = 0
	}
	sqliteSize := formatBytes(sizeVal)

	response := fmt.Sprintf("Database Status: %d keys | SQLite internal: %s | File size: %s",
		stats["keys"], sqliteSize, fileSizeStr)

	irc.ReplyTo(response)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
