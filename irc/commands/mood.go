package commands

import (
	"fmt"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/shared/meta"
	"aibird/text"
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

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, hours, 200)
	if err != nil {
		logger.Error("Failed to get channel messages for mood", "error", err)
		irc.Send("Error retrieving channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No activity found in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	// Queue the AI generation — LlamaCpp uses the GPU and must be serialized
	enqueueAITask(irc, q, "mood", meta.GPU4090, func(s *state.State) {
		generateMood(s, messages, hours)
	})
}

func generateMood(irc *state.State, messages []birdbase.ChannelMessage, hours int) {
	irc.Send(fmt.Sprintf("%s, reading the room for the last %dh...", irc.User.NickName, hours))

	systemPrompt, err := text.GetPrompt("mood.md")
	if err != nil {
		logger.Error("Failed to load mood prompt", "error", err)
		irc.Send("Error: could not load mood prompt.")
		return
	}

	userPrompt := formatEventLog(irc.Channel.Name, hours, messages)

	answer, err := singleRequestWithFallback(systemPrompt, userPrompt, irc.Config)
	if err != nil {
		logger.Error("Failed to generate mood", "error", err)
		irc.Send("Error: failed to analyze the mood. Please try again later.")
		return
	}

	irc.Send(fmt.Sprintf("🌡️ %s", answer))
}
