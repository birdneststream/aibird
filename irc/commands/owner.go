package commands

import (
	"fmt"
	"os"
	"path/filepath"

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
	// Get database stats from Bitcask
	stats, err := birdbase.GetDatabaseStats()
	if err != nil {
		irc.ReplyTo(fmt.Sprintf("Error getting database stats: %v", err))
		return
	}

	// Get file system size of database directory
	dbPath := "bird.db"
	var totalSize int64
	err = filepath.Walk(dbPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		irc.ReplyTo(fmt.Sprintf("Error calculating disk usage: %v", err))
		return
	}

	// Format size in human readable format
	sizeStr := formatBytes(totalSize)
	sizeVal, ok := stats["size"].(int64)
	if !ok {
		sizeVal = 0
	}
	bitcaskSize := formatBytes(sizeVal)

	response := fmt.Sprintf("Database Status: %d keys | %d datafiles | Bitcask size: %s | Disk usage: %s",
		stats["keys"], stats["datafiles"], bitcaskSize, sizeStr)

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
