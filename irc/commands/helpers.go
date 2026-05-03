// Package commands implements IRC command dispatching and handler registration.
package commands

import (
	"fmt"
	"strings"

	"aibird/birdbase"
)

// Event type constants for message storage and formatting.
const (
	eventPrivmsg = "privmsg"
	eventAction  = "action"
	eventJoin    = "join"
	eventPart    = "part"
	eventKick    = "kick"
	eventQuit    = "quit"
)

// Argument constants.
const argHelp = "--help"

// displayDefault returns the value if non-empty, otherwise the fallback.
// Used for formatting user-facing display strings where empty values
// should show a descriptive placeholder instead.
func displayDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
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
	fmt.Fprintf(&logBuilder, "Channel: %s | Time period: last %d hours\n\n", channelName, hours)

	for _, msg := range messages {
		switch msg.EventType {
		case eventPrivmsg:
			fmt.Fprintf(&logBuilder, "<%s> %s\n", msg.Nickname, msg.Message)
		case eventAction:
			fmt.Fprintf(&logBuilder, "* %s %s\n", msg.Nickname, msg.Message)
		case eventJoin:
			fmt.Fprintf(&logBuilder, "--> %s joined\n", msg.Nickname)
		case eventPart:
			fmt.Fprintf(&logBuilder, "<-- %s left\n", msg.Nickname)
		case eventQuit:
			fmt.Fprintf(&logBuilder, "<-- %s quit\n", msg.Nickname)
		case eventKick:
			fmt.Fprintf(&logBuilder, "<-- %s was kicked by %s\n", msg.Nickname, msg.Message)
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

// parseIntSafe parses a leading integer from a string without using strconv.
// Stops at the first non-digit character.
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
