package commands

import (
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/queue"
)

func ParseSummary(irc *state.State, q *queue.ProcessingQueue) {
	if irc.Channel == nil {
		irc.Send("Error: summary can only be used in a channel.")
		return
	}

	if !hasTextProviderConfig(irc.Config) {
		irc.Send("Error: no AI provider available. Configure LlamaCpp or GLM.")
		return
	}

	if irc.Message() == argHelp {
		irc.Send("Usage: !summary [hours] [--persona <angle>] — Summarizes the last N hours of channel activity.")
		irc.Send("Examples: !summary, !summary 6h, !summary 12 --persona \"a western cowboy\"")
		irc.Send("Defaults: 24h, max: 168h (7 days). Up to 1000 chat messages (text + /me actions only; no joins/parts/quits/kicks). --persona: optional creative angle.")
		return
	}

	defaultHours := irc.Config.AiBird.SummaryDefaultHours
	if defaultHours <= 0 {
		defaultHours = 24
	}

	hours := defaultHours
	persona := ""

	// Check for --persona flag
	if flagPersona, ok := irc.GetStringArg("persona", ""); ok && flagPersona != "" {
		persona = sanitizePersona(flagPersona)
	}

	if irc.Message() != "" {
		msg := strings.TrimSpace(irc.Message())

		// Try to parse a leading number as hours (e.g., "6", "6h", "12h")
		// Remaining text after hours is treated as the persona argument (unless --persona was used)
		hoursStr := msg
		rest := ""

		if idx := strings.IndexByte(msg, ' '); idx > 0 {
			hoursStr = msg[:idx]
			rest = strings.TrimSpace(msg[idx+1:])
		}

		cleaned := strings.TrimSuffix(hoursStr, "h")
		if parsed, err := parseIntSafe(cleaned); err == nil && parsed > 0 {
			if parsed > 168 {
				parsed = 168
			}
			hours = parsed
			// If no --persona flag was set, use remaining text as freeform persona
			if persona == "" {
				persona = sanitizePersona(rest)
			}
		} else {
			// No valid hours prefix — entire message is the persona (unless --persona was used)
			if persona == "" {
				persona = sanitizePersona(msg)
			}
		}
	}

	maxMessages := irc.Config.AiBird.SummaryMaxMessages
	if maxMessages <= 0 {
		maxMessages = 1000
	}

	// Summary only considers actual conversation — joins/parts/quits and kicks
	// are filtered out at the database level so the 1000-event cap applies to
	// chat messages rather than noisy presence events.
	fetchHistoryAndEnqueue(irc, q, "summary", hours, maxMessages, chatEventTypes, func(s *state.State, messages []birdbase.ChannelMessage, h int) {
		generateChannelAIResponse(s, messages, h, channelAIOpts{
			statusMsg:    fmt.Sprintf("generating summary of the last %dh (%d events)...", h, len(messages)),
			promptFile:   "summary.md",
			personaLabel: "Persona/Angle for this summary: ",
			persona:      persona,
			errorContext: "summary",
		})
	})
}
