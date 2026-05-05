// Package commands implements IRC command dispatching and handler registration.
package commands

import (
	"context"
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/shared/meta"
	"aibird/text"
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
// Truncation is rune-aware to avoid splitting multi-byte UTF-8 characters.
func sanitizePersona(input string) string {
	s := strings.ReplaceAll(input, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) > 200 {
		runes = runes[:200]
	}
	return string(runes)
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

// enqueueAITask wraps the common queue-enqueue pattern for AI text commands.
// gpu parameter allows callers to specify GPU routing (all current callers use meta.GPU4090).
// The closure captures state.State by value at enqueue time, so mutations to the
// original *state.State after enqueue do not affect the queued item.
func enqueueAITask(irc *state.State, q *queue.ProcessingQueue, model string, gpu meta.GPUType, generateFn func(*state.State)) {
	if q == nil {
		generateFn(irc)
		return
	}
	queueItem := queue.QueueItem{
		Item: queue.Item{
			State: *irc,
			Function: func(_ context.Context, s state.State, _ meta.GPUType) {
				generateFn(&s)
			},
		},
		Model: model,
		User:  irc.User,
		GPU:   gpu,
	}
	msg, err := q.Enqueue(queueItem)
	if err != nil {
		irc.SendError(err.Error())
	} else if msg != "" {
		irc.Send(msg)
	}
}

// fetchHistoryAndEnqueue encapsulates the shared tail pattern used by AI channel commands
// (summary, mood, story): resolve networkID → fetch channel messages → check for empty
// result → enqueue AI task. Each command keeps its own parsing logic and passes a
// generateFn closure that captures parsed parameters (hours, persona, messages).
func fetchHistoryAndEnqueue(irc *state.State, q *queue.ProcessingQueue, model string, hours, maxMessages int, generateFn func(*state.State, []birdbase.ChannelMessage, int)) {
	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, hours, maxMessages)
	if err != nil {
		logger.Error("Failed to get channel messages for "+model, "error", err)
		irc.Send("Error retrieving channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No activity found in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	enqueueAITask(irc, q, model, meta.GPU4090, func(s *state.State) {
		generateFn(s, messages, hours)
	})
}

// channelAIOpts configures the shared AI channel response generation.
// This groups the command-specific parameters that vary between mood/story/summary,
// keeping the function signature clean and self-documenting.
type channelAIOpts struct {
	statusMsg    string // Pre-formatted status message sent before processing
	promptFile   string // System prompt filename (e.g., "mood.md")
	personaLabel string // Label prepended to persona text (e.g., "Genre/Style for this story: "); empty disables persona
	persona      string // User-provided persona/style text (already sanitized)
	emojiPrefix  string // Prepended to the AI response when non-empty (e.g., "🌡️ ")
	errorContext string // Command name for error messages (e.g., "mood", "story")
}

// generateChannelAIResponse encapsulates the shared generate pattern used by AI channel
// commands: send status message → load system prompt → format event log → append optional
// persona suffix → call AI with fallback → send response with optional emoji prefix.
func generateChannelAIResponse(irc *state.State, messages []birdbase.ChannelMessage, hours int, opts channelAIOpts) {
	irc.Send(fmt.Sprintf("%s, %s", irc.User.NickName, opts.statusMsg))

	systemPrompt, err := text.GetPrompt(opts.promptFile)
	if err != nil {
		logger.Error("Failed to load "+opts.promptFile, "error", err)
		irc.Send(fmt.Sprintf("Error: could not load %s prompt.", opts.errorContext))
		return
	}

	userPrompt := formatEventLog(irc.Channel.Name, hours, messages)

	if opts.persona != "" && opts.personaLabel != "" {
		userPrompt += fmt.Sprintf("\n\n%s%s", opts.personaLabel, opts.persona)
	}

	answer, err := singleRequestWithFallback(systemPrompt, userPrompt, irc.Config)
	if err != nil {
		logger.Error("Failed to generate "+opts.errorContext, "error", err)
		irc.Send(fmt.Sprintf("Error: failed to generate %s. Please try again later.", opts.errorContext))
		return
	}

	if opts.emojiPrefix != "" {
		irc.Send(fmt.Sprintf("%s%s", opts.emojiPrefix, answer))
	} else {
		irc.Send(answer)
	}
}

// truncateString truncates a string to the specified length and adds ellipsis if needed.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
