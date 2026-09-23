package main

import (
	"fmt"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/irc/commands"
	"aibird/irc/networks"
	"aibird/irc/participant"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

func handlePrivMsg(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config, q *queue.ProcessingQueue) { //nolint:gocritic // girc library mandates Event by value
	// Ignore ZNC buffer playback
	if isZncBufferPlayback(e, network, config) {
		return
	}

	// Troll problem user: any user whose ident is "anonymous" is censored.
	// Reply with a canned message and skip all processing (commands and chat).
	if e.Source != nil && isCensoredIdent(e.Source.Ident) {
		logger.Info("Censored ident, refusing to process", "network", network.NetworkName, "nick", e.Source.Name, "ident", e.Source.Ident)
		c.Cmd.Reply(e, "lol censored !!!")
		return
	}

	// Known spam bot: kick immediately and stop processing the message.
	if kickSpamBotIfMatch(c, e, network) {
		return
	}

	// Check if this is a command (starts with trigger)
	isCommand := strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger)

	if !isCommand {
		storeChannelEvent(e, network)
		// Ordinary chat counts as activity too. Without this, LatestActivity is
		// only ever refreshed when someone invokes a bot command, so a user who
		// just talks in the channel looks weeks stale to anything that reads it.
		touchChatSender(network, e)
		// Not a command - check if we should handle it as a chat message for participant system
		participant.HandleChatMessage(c, e, network, config)
		return
	}

	// It is a command, so we initialize state and then perform checks.
	irc := state.Init(c, e, network, config)

	// Wire the command validator so Verify() can check against the command registry.
	irc.ValidateCommand = commands.IsValidCommand

	// Verify will parse the command, check for a trigger, and run the validator.
	if err := irc.Verify(); err == nil {
		logger.Info(
			"Command received",
			"command", irc.Action(),
			"user", irc.User.NickName,
			"channel", irc.Channel.Name,
			"network", irc.Network.Name,
		)

		// Track command usage for leaderboards
		if err := birdbase.IncrementCommandUsage(irc.Network.NetworkName, irc.User.NickName, irc.Action()); err != nil {
			logger.Warn("Failed to track command usage", "error", err, "network", irc.Network.NetworkName, "user", irc.User.NickName, "command", irc.Action())
		} else {
			logger.Debug("Command usage tracked", "network", irc.Network.NetworkName, "user", irc.User.NickName, "command", irc.Action())
		}

		// Only check for flooding on messages that are commands.
		checkFlood(&irc)

		dispatchCommand(&irc, q)
	}
}

// censoredIdent is the ident of a known troll problem user. Matching is
// case-insensitive so casing tweaks ("Anonymous", "ANONYMOUS") don't evade it.
const censoredIdent = "anonymous"

// isCensoredIdent reports whether the sender's ident is on the censored list.
// Censored users get a canned reply and no bot processing of any kind.
// A leading "~" is stripped first: IRC servers prepend it when the user has
// no identd, so the troll shows up as "~anonymous" (see admin.go's lookup
// fallback for the same pattern).
func isCensoredIdent(ident string) bool {
	return strings.EqualFold(strings.TrimPrefix(ident, "~"), censoredIdent)
}

// spamBotSignature is the normalized fingerprint of a known spam-bot
// message. Matching is done on a normalized form (lowercase, punctuation and
// apostrophe variants stripped, whitespace collapsed) so quote-style and
// casing tweaks don't evade it.
const spamBotSignature = "hi guys its madeleine czura just thought id leave my number here in case youre lonely"

// normalizeForSpam lowers the text, keeps ASCII alphanumerics and spaces,
// drops everything else (apostrophes, punctuation, unicode), and collapses
// runs of whitespace to single spaces.
func normalizeForSpam(s string) string {
	lowered := strings.ToLower(s)

	var b strings.Builder
	b.Grow(len(lowered))
	for _, r := range lowered {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(' ')
		}
		// Everything else (punctuation, apostrophes, unicode) is dropped.
	}

	return strings.Join(strings.Fields(b.String()), " ")
}

// isSpamBotMessage reports whether the text contains the known spam-bot
// fingerprint (normalized substring match).
func isSpamBotMessage(msg string) bool {
	return strings.Contains(normalizeForSpam(msg), spamBotSignature)
}

// kickSpamBotIfMatch kicks the sender when a channel message matches the
// known spam-bot fingerprint. It reports whether the message was handled
// (spam messages are neither stored nor processed further). PMs can't be
// kicked and are ignored here. Note the match is substring-based, so anyone
// quoting the spam verbatim is also kicked — this stops retransmission.
func kickSpamBotIfMatch(c *girc.Client, e girc.Event, network *networks.Network) bool { //nolint:gocritic // girc library mandates Event by value
	if e.Source == nil || e.IsFromUser() || len(e.Params) == 0 || !strings.HasPrefix(e.Params[0], "#") {
		return false
	}

	// Never act on our own messages (nicks are case-insensitive on IRC).
	if strings.EqualFold(e.Source.Name, network.Nick) {
		return false
	}

	if !isSpamBotMessage(e.Last()) {
		return false
	}

	logger.Info("Kicking known spam bot", "network", network.NetworkName, "channel", e.Params[0], "nick", e.Source.Name)
	c.Cmd.Kick(e.Params[0], e.Source.Name, "Spam bot detected")
	return true
}

// touchChatSender records activity for the sender of a non-command message.
//
// Touching the in-memory user is done immediately so anything reading
// LatestActivity sees it right away. The database write is debounced per
// (network, ident, host) — a burst of chatter should cost one row update, not
// one per line. The persisted value is a snapshot taken at touch time, which
// is safe here because SaveSingleUser upserts by (network, ident, host) and
// the newest timestamp is all this path is responsible for.
func touchChatSender(network *networks.Network, e girc.Event) { //nolint:gocritic // girc library mandates Event by value
	if e.Source == nil || e.Source.Name == "" {
		return
	}

	// Our own output is not user activity.
	if strings.EqualFold(e.Source.Name, network.Nick) {
		return
	}

	user := network.GetUserWithIdentAndHost(e.Source.Ident, e.Source.Host)
	if user == nil {
		return
	}

	user.Touch(e.Last())

	snapshot := user.LatestActivity
	birdbase.TouchUserActivity(network.NetworkName, e.Source.Ident, e.Source.Host, snapshot)
}

// isZncBufferPlayback checks if a message is likely ZNC buffer replay and should be ignored.
func isZncBufferPlayback(e girc.Event, network *networks.Network, config *settings.Config) bool { //nolint:gocritic // girc library mandates Event by value
	const zncBufferGrace = 15 * time.Second

	if network.ConnectedAt.IsZero() {
		return false
	}

	// Method 1: If server-time is supported, check if message is from before we connected
	if !e.Timestamp.IsZero() && e.Timestamp.Before(network.ConnectedAt) {
		logger.Debug("Ignoring ZNC buffer playback (server-time)", "network", network.NetworkName, "msg_time", e.Timestamp, "connected_at", network.ConnectedAt)
		return true
	}

	// Method 2: For networks without server-time (like EFNet), ignore commands in first 15 seconds
	if e.Timestamp.IsZero() && time.Since(network.ConnectedAt) < zncBufferGrace {
		if strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger) {
			logger.Debug("Ignoring potential ZNC buffer command (startup grace period)", "network", network.NetworkName, "elapsed", time.Since(network.ConnectedAt))
			return true
		}
	}

	return false
}

// storeChannelEvent stores a non-command message for the summary feature.
// Skips bot's own messages and non-channel messages.
func storeChannelEvent(e girc.Event, network *networks.Network) { //nolint:gocritic // girc library mandates Event by value
	if e.Source.Name == network.Nick || len(e.Params) == 0 || !strings.HasPrefix(e.Params[0], "#") {
		return
	}

	networkID := birdbase.ResolveNetworkID(network.NetworkName)
	if networkID <= 0 {
		return
	}

	msg := e.Last()
	eventType := "privmsg"

	// Detect CTCP ACTION (/me): \x01ACTION text\x01
	if len(msg) > 8 && strings.HasPrefix(msg, "\x01ACTION ") && strings.HasSuffix(msg, "\x01") {
		msg = strings.TrimPrefix(msg, "\x01ACTION ")
		msg = strings.TrimSuffix(msg, "\x01")
		eventType = "action"
	} else if msg != "" && msg[0] == '\x01' {
		// Other CTCP messages (VERSION, PING, etc.) — skip storage
		return
	}

	birdbase.StoreChannelMessage(networkID, e.Params[0], e.Source.Name, eventType, msg)
}

// checkFlood implements the first tier of flood protection.
// Called for valid commands only, after Verify() passes.
// Uses nick-based keys and kicks the user from the channel on threshold breach.
// This is the visible punishment layer — separate from MessageFloodCheck which
// silently ignores the user via User.Ignored.
func checkFlood(irc *state.State) { //nolint:gocritic // State passed by value per current architecture
	if irc.Channel == nil {
		return
	}

	// Exempt admins and owners from flood check
	if irc.User != nil && (irc.User.IsAdmin || irc.User.IsOwner) {
		return
	}

	config := irc.GetConfig().AiBird
	key := fmt.Sprintf("flood:%s:%s", irc.Network.Name, irc.User.NickName)
	ban := fmt.Sprintf("flood-ban:%s:%s", irc.Network.Name, irc.User.NickName)

	// Check if user is currently banned using in-memory flood manager
	if birdbase.FloodManager.IsFloodBanned(ban) {
		return
	}

	floodWindow := time.Second * 1

	// Increment flood counter using in-memory flood manager
	countInt := birdbase.FloodManager.IncrementFloodCounter(key, floodWindow)

	if countInt > config.FloodThreshold {
		// Set flood ban using in-memory flood manager
		birdbase.FloodManager.SetFloodBan(ban, time.Duration(config.FloodIgnoreMinutes)*time.Minute)
		irc.Client.Cmd.Kick(irc.Channel.Name, irc.Event.Source.Name, "Birds fly above floods!")
	}
}
