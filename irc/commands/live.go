package commands

import (
	"fmt"
	"strings"
	"time"

	"aibird/irc/state"
	"aibird/live"

	"github.com/lrstanley/girc"
)

// liveAnnounce is the channel announcement sent when a stream starts.
const liveAnnounce = "🔴 BIRDNEST IS STARTING SOON, TUNE IN!"

const (
	// liveCmdOff and liveCmdEnd manually end the on-air state.
	liveCmdOff = "off"
	liveCmdEnd = "end"

	// highlightChunkMaxBytes is the byte budget for one mass-highlight
	// message; IRC lines cap at 512 bytes, leaving room for framing.
	highlightChunkMaxBytes = 450

	// highlightPace is the delay between highlight chunks so flood-throttled
	// networks don't drop the messages or kick the bot.
	highlightPace = 300 * time.Millisecond

	// liveAutoDetectOff is the status suffix shown when monitoring is disabled.
	liveAutoDetectOff = "off"

	// liveCallToActionFormat is sent after the highlight chunks so the final
	// message of the announcement burst carries the stream URL (%s = URL).
	liveCallToActionFormat = "BIRDNEST IS LIVE NOW CHECK IT %s"
)

// gircNotifier adapts a girc client to the live.Notifier interface.
type gircNotifier struct {
	client *girc.Client
}

func (g gircNotifier) SetTopic(channel, topic string) {
	g.client.Cmd.Topic(channel, topic)
}

func (g gircNotifier) MessageChannel(channel, message string) {
	g.client.Cmd.Message(channel, message)
}

// liveAction categorizes the message part of a !live command.
type liveAction int

const (
	liveActionStatus liveAction = iota
	liveActionOff
	liveActionStart
	liveActionInvalid
)

// classifyLiveMessage routes a !live message body to its handler logic.
// Pure function: no IRC or manager interaction, so it stays trivially testable.
func classifyLiveMessage(message string) liveAction {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "":
		return liveActionStatus
	case liveCmdOff, liveCmdEnd:
		return liveActionOff
	}

	if live.ExtractVideoID(message) == "" {
		return liveActionInvalid
	}
	return liveActionStart
}

// ParseLive implements the admin !live command: announce a YouTube stream,
// mass-highlight the channel, and manage the ON AIR / WAS LIVE topic.
func ParseLive(irc *state.State) {
	if irc.Event.IsFromUser() {
		irc.SendError("Must be used in a channel")
		return
	}

	switch classifyLiveMessage(irc.Message()) {
	case liveActionStatus:
		sendLiveStatus(irc)
	case liveActionOff:
		stopLive(irc)
	case liveActionStart:
		startLive(irc)
	default:
		irc.SendError("Usage: " + irc.GetActionTrigger() + "live <youtube url> | " +
			irc.GetActionTrigger() + "live off | " + irc.GetActionTrigger() + "live")
	}
}

// sendLiveStatus reports the on-air state of the current channel.
func sendLiveStatus(irc *state.State) {
	m := live.Default()
	if s, ok := m.Status(irc.Network.NetworkName, irc.Channel.Name); ok {
		auto := liveAutoDetectOff
		if m.MonitoringEnabled() {
			auto = "on"
		}
		irc.Send(fmt.Sprintf("📡 ON AIR: %s (started %s ago, auto-detect: %s)",
			s.URL, time.Since(s.StartedAt).Round(time.Minute), auto))
		return
	}
	irc.Send("⚫ Not on air. Use " + irc.GetActionTrigger() + "live <youtube url> to go live.")
}

// stopLive ends the on-air state via !live off.
func stopLive(irc *state.State) {
	if live.Default().Stop(irc.Network.NetworkName, irc.Channel.Name) {
		irc.SendSuccess("Stream ended — topic updated")
	} else {
		irc.SendWarning("No active stream in this channel")
	}
}

// startLive announces a stream, sets the ON AIR topic, and highlights the channel.
func startLive(irc *state.State) {
	videoID := live.ExtractVideoID(irc.Message())
	if videoID == "" {
		irc.SendError("Invalid YouTube URL")
		return
	}

	// Canonicalize so the topic always carries a clean, predictable URL.
	canonicalURL := "https://www.youtube.com/watch?v=" + videoID

	m := live.Default()
	m.Start(irc.Network.NetworkName, irc.Channel.Name, videoID, canonicalURL, gircNotifier{client: irc.Client})

	irc.Send(liveAnnounce + " " + canonicalURL)

	if !m.MonitoringEnabled() {
		irc.Send("ℹ️ Auto-detect off — configure [youtube] apiKey; use " +
			irc.GetActionTrigger() + "live off when done.")
	}

	sendLiveHighlights(irc, canonicalURL)
}

// sendLiveHighlights mass-highlights everyone currently in the channel so they
// get pinged about the stream, then follows with a call-to-action so the last
// message of the burst carries the URL. Nicks come from girc's live channel
// state; if names aren't known yet the highlight and CTA are skipped silently
// (the announcement message already contains the URL).
func sendLiveHighlights(irc *state.State, streamURL string) {
	if irc.Client == nil {
		return
	}

	ch := irc.Client.LookupChannel(irc.Channel.Name)
	if ch == nil {
		return
	}

	chunks := BuildHighlightChunks(ch.UserList, irc.Network.Nick, highlightChunkMaxBytes)
	for i, chunk := range chunks {
		if i > 0 {
			time.Sleep(highlightPace)
		}
		irc.Client.Cmd.Message(irc.Channel.Name, chunk)
	}

	// Client notification previews should land on the URL, not a wall of
	// nicks, so the CTA trails the highlight burst.
	if len(chunks) > 0 {
		time.Sleep(highlightPace)
		irc.Client.Cmd.Message(irc.Channel.Name,
			fmt.Sprintf(liveCallToActionFormat, streamURL))
	}
}

// BuildHighlightChunks packs nick names into space-separated highlight
// messages of at most maxBytes bytes (IRC lines cap at 512). Duplicates
// (case-insensitive) and the bot's own nick are dropped. Returns nil when
// there is nothing to send. A single nick longer than maxBytes is skipped
// rather than emitting an oversized message.
func BuildHighlightChunks(nicks []string, excludeNick string, maxBytes int) []string {
	if maxBytes <= 0 {
		return nil
	}

	exclude := strings.ToLower(excludeNick)
	seen := make(map[string]bool, len(nicks))
	unique := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		nick = strings.TrimSpace(nick)
		if nick == "" {
			continue
		}
		key := strings.ToLower(nick)
		if key == exclude || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, nick)
	}

	var chunks []string
	var current strings.Builder
	for _, nick := range unique {
		additional := len(nick)
		if current.Len() > 0 {
			additional++ // separating space
		}

		// Flush only when something is accumulated: an overlong nick at the
		// head of a chunk must not emit an empty message.
		if current.Len() > 0 && current.Len()+additional > maxBytes {
			chunks = append(chunks, current.String())
			current.Reset()
			additional = len(nick)
		}

		// An overlong single nick can't fit any chunk; skip it.
		if additional > maxBytes {
			continue
		}

		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(nick)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}

	return chunks
}
