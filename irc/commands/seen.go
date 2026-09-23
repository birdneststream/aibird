package commands

import (
	"fmt"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/state"
	"aibird/logger"

	"github.com/lrstanley/girc"
)

// seenFallbackTTL bounds how long an in-memory activity timestamp may be
// trusted when channel_messages has nothing to say about the nick.
//
// The in-memory value is only refreshed when the user invokes a bot command,
// so a purely chatty user's value goes stale. Once it is older than this,
// reporting it would be worse than saying nothing — which is exactly the bug
// this package used to have ("last seen 3 weeks ago" right after they typed).
const seenFallbackTTL = 24 * time.Hour

// lastSeenMessage resolves when we last saw nickname and renders the reply for
// !seen. channelMessages is authoritative because every non-command chat
// message plus join/part/quit/kick is recorded there; fallbackActivity is the
// in-memory timestamp, used only when the database has no rows for the nick.
//
// Returns an empty string when there is no trustworthy activity to report.
func lastSeenMessage(networkID int64, channelName, nickname string, fallbackActivity int64) string {
	seen, err := birdbase.LastSeen(networkID, channelName, nickname)
	if err != nil {
		// Fail open to the in-memory value below; a DB hiccup should not make
		// the command useless when we still hold recent activity.
		logger.Error("Failed to look up last seen", "error", err, "network_id", networkID, "channel", channelName, "nick", nickname)
		seen = birdbase.LastSeenResult{}
	}

	fallbackAge := time.Since(time.Unix(fallbackActivity, 0))
	fallbackFresh := fallbackActivity > 0 && fallbackAge < seenFallbackTTL

	// Pick whichever source is newer.
	best := seen
	if fallbackFresh && fallbackActivity > best.Timestamp {
		best = birdbase.LastSeenResult{Timestamp: fallbackActivity, EventType: "privmsg", Found: true}
	}

	if !best.Found && !fallbackFresh {
		return ""
	}
	if best.Timestamp == 0 {
		return ""
	}

	verb := "chatted"
	if best.EventType != "" && best.EventType != "privmsg" && best.EventType != "action" {
		verb = best.EventType
	}

	return fmt.Sprintf("%s was last seen %s ago (%s)", nickname, helpers.UnixTimeToHumanReadable(best.Timestamp), verb)
}

// ParseSeen reports when the bot last saw a nick in the current channel.
func ParseSeen(irc *state.State) {
	nickname := strings.TrimSpace(irc.Message())
	if nickname == "" {
		irc.Send("Usage: !seen <nick>")
		return
	}

	// Asking about yourself while standing right there is not a mystery.
	if strings.EqualFold(nickname, irc.User.NickName) {
		irc.ReplyTo(girc.Fmt("{b}Hey pal you are seen!{b}"))
		return
	}

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		// No database handle for this network; fall back to what memory holds.
		user, _ := irc.Channel.GetUserWithNick(nickname)
		if user == nil {
			irc.Send("I have not seen this user")
			return
		}
		irc.Send(user.Seen())
		return
	}

	message := lastSeenMessage(networkID, irc.Channel.Name, nickname, inMemoryActivity(irc, nickname))
	if message == "" {
		irc.Send("I have not seen " + nickname + " here")
		return
	}

	irc.Send(message)
}

// inMemoryActivity returns the newest timestamp the running process holds for
// nickname in this network, or 0 when the nick is unknown. Lookup goes through
// the channel list first so the shared pointer (and its Touch history) is used.
func inMemoryActivity(irc *state.State, nickname string) int64 {
	if irc.Channel != nil {
		if user, _ := irc.Channel.GetUserWithNick(nickname); user != nil {
			return user.LatestActivity
		}
	}

	if user := irc.Network.GetUserWithNick(nickname); user != nil {
		return user.LatestActivity
	}

	return 0
}
