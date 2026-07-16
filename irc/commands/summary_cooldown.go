package commands

import (
	"strings"
	"time"

	"aibird/birdbase"
)

// Anti-abuse controls for !summary.
//
// !summary was being spammed in #soy, so it is limited to one generation per
// hour there. Any attempt inside the cooldown window — including repeated
// attempts — is rejected with the channel in-joke "blame Filfipinx".
const (
	// summaryCooldownChannel is the only channel currently under a summary
	// cooldown. Add more here (or generalize to config) if other channels
	// need throttling.
	summaryCooldownChannel = "#soy"
	// summaryCooldownDuration is how long a channel must wait between summaries.
	summaryCooldownDuration = time.Hour
)

// summaryCooldownKey builds the RateLimiter key for a channel's summary
// cooldown. It is namespaced by network so the limit does not leak across
// networks that happen to share a channel name. The channel is lowercased so
// "#soy" and "#SOY" share the same limit.
func summaryCooldownKey(network, channel string) string {
	return "summary-cooldown:" + network + ":" + strings.ToLower(channel)
}

// enforceSummaryCooldown applies the once-per-hour limit for the cooldown
// channel (#soy). It returns true when the summary may proceed — and records
// that use so subsequent attempts within the window are blocked — or false
// when the channel is still cooling down. Channels not under cooldown always
// return true and are never recorded.
func enforceSummaryCooldown(network, channel string) bool {
	if !strings.EqualFold(channel, summaryCooldownChannel) {
		return true
	}
	key := summaryCooldownKey(network, channel)
	if birdbase.RateLimiter.IsRateLimited(key) {
		return false
	}
	birdbase.RateLimiter.SetRateLimit(key, summaryCooldownDuration)
	return true
}
