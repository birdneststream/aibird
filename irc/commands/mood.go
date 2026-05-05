package commands

import (
	"fmt"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/queue"
)

func ParseMood(irc *state.State, q *queue.ProcessingQueue) {
	if irc.Channel == nil {
		irc.Send("Error: mood can only be used in a channel.")
		return
	}

	if !hasTextProviderConfig(irc.Config) {
		irc.Send("Error: no AI provider available. Configure LlamaCpp or GLM.")
		return
	}

	if irc.Message() == argHelp {
		irc.Send("Usage: !mood [hours] — Analyzes the current mood/vibe of the channel.")
		irc.Send("Example: !mood, !mood 6h")
		return
	}

	hours := 24
	if irc.Message() != "" {
		hours = parseHoursFromMessage(irc.Message(), 24, 168)
	}

	fetchHistoryAndEnqueue(irc, q, "mood", hours, 200, func(s *state.State, messages []birdbase.ChannelMessage, h int) {
		generateChannelAIResponse(s, messages, h, channelAIOpts{
			statusMsg:    fmt.Sprintf("reading the room for the last %dh...", h),
			promptFile:   "mood.md",
			emojiPrefix:  "🌡️ ",
			errorContext: "mood",
		})
	})
}
