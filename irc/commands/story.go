package commands

import (
	"fmt"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/queue"
)

func ParseStory(irc *state.State, q *queue.ProcessingQueue) {
	if irc.Channel == nil {
		irc.Send("Error: story can only be used in a channel.")
		return
	}

	if !hasTextProviderConfig(irc.Config) {
		irc.Send("Error: no AI provider available. Configure LlamaCpp or GLM.")
		return
	}

	if irc.Message() == argHelp {
		irc.Send("Usage: !story [hours] [--persona <genre>] — Turns channel events into a short story.")
		irc.Send("Example: !story, !story 6h --persona \"fantasy\", !story --persona \"film noir\"")
		return
	}

	hours := parseHoursFromMessage(irc.Message(), 24, 168)
	persona, _ := irc.GetStringArg("persona", "")
	persona = sanitizePersona(persona)

	fetchHistoryAndEnqueue(irc, q, "story", hours, 200, nil, func(s *state.State, messages []birdbase.ChannelMessage, h int) {
		generateChannelAIResponse(s, messages, h, channelAIOpts{
			statusMsg:    fmt.Sprintf("writing a story from the last %dh (%d events)...", h, len(messages)),
			promptFile:   "story.md",
			personaLabel: "Genre/Style for this story: ",
			persona:      persona,
			emojiPrefix:  "📖 ",
			errorContext: "story",
		})
	})
}
