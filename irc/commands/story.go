package commands

import (
	"context"
	"fmt"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/shared/meta"
	"aibird/text"
)

func ParseStory(irc state.State, q *queue.ProcessingQueue) {
	if irc.Channel == nil {
		irc.Send("Error: story can only be used in a channel.")
		return
	}

	if !hasTextProviderConfig(irc.Config) {
		irc.Send("Error: no AI provider available. Configure LlamaCpp or GLM.")
		return
	}

	if irc.Message() == "--help" {
		irc.Send("Usage: !story [hours] [--persona <genre>] — Turns channel events into a short story.")
		irc.Send("Example: !story, !story 6h --persona \"fantasy\", !story --persona \"film noir\"")
		return
	}

	hours := parseHoursFromMessage(irc.Message(), 24, 168)
	persona, _ := irc.GetStringArg("persona", "")
	persona = sanitizePersona(persona)

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, hours, 200)
	if err != nil {
		logger.Error("Failed to get channel messages for story", "error", err)
		irc.Send("Error retrieving channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No activity found in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	// Queue the AI generation — LlamaCpp uses the GPU and must be serialized
	queueItem := queue.QueueItem{
		Item: queue.Item{
			State: irc,
			Function: func(ctx context.Context, s state.State, gpu meta.GPUType) {
				generateStory(s, messages, hours, persona)
			},
		},
		Model: "story",
		User:  irc.User,
		GPU:   meta.GPU4090,
	}

	msg, err := q.Enqueue(queueItem)
	if err != nil {
		irc.SendError(err.Error())
	} else if msg != "" {
		irc.Send(msg)
	}
}

func generateStory(irc state.State, messages []birdbase.ChannelMessage, hours int, persona string) {
	irc.Send(fmt.Sprintf("%s, writing a story from the last %dh (%d events)...", irc.User.NickName, hours, len(messages)))

	systemPrompt, err := text.GetPrompt("story.md")
	if err != nil {
		logger.Error("Failed to load story prompt", "error", err)
		irc.Send("Error: could not load story prompt.")
		return
	}

	userPrompt := formatEventLog(irc.Channel.Name, hours, messages)

	if persona != "" {
		userPrompt += fmt.Sprintf("\n\nGenre/Style for this story: %s", persona)
	}

	answer, err := singleRequestWithFallback(systemPrompt, userPrompt, irc.Config)
	if err != nil {
		logger.Error("Failed to generate story", "error", err)
		irc.Send("Error: failed to write the story. Please try again later.")
		return
	}

	irc.Send(fmt.Sprintf("📖 %s", answer))
}
