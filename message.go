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

func handlePrivMsg(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config, q *queue.ProcessingQueue) {
	// Grace period after connect to ignore ZNC buffer playback commands
	const zncBufferGrace = 15 * time.Second

	// Ignore ZNC buffer playback
	if !network.ConnectedAt.IsZero() {
		// Method 1: If server-time is supported, check if message is from before we connected
		if !e.Timestamp.IsZero() && e.Timestamp.Before(network.ConnectedAt) {
			logger.Debug("Ignoring ZNC buffer playback (server-time)", "network", network.NetworkName, "msg_time", e.Timestamp, "connected_at", network.ConnectedAt)
			return
		}
		// Method 2: For networks without server-time (like EFNet), ignore commands in first 15 seconds
		// ZNC buffer playback happens immediately on connect, longer grace period for networks with many channels
		if e.Timestamp.IsZero() && time.Since(network.ConnectedAt) < zncBufferGrace {
			if strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger) {
				logger.Debug("Ignoring potential ZNC buffer command (startup grace period)", "network", network.NetworkName, "elapsed", time.Since(network.ConnectedAt))
				return
			}
		}
	}

	// Check if this is a command (starts with trigger)
	isCommand := strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger)

	if !isCommand {
		// Store non-command message for summary feature (skip bot's own messages and non-channel messages)
		if e.Source.Name != network.Nick && len(e.Params) > 0 && strings.HasPrefix(e.Params[0], "#") {
			networkID := birdbase.ResolveNetworkID(network.NetworkName)
			if networkID > 0 {
				msg := e.Last()
				eventType := "privmsg"
				// Detect CTCP ACTION (/me): \x01ACTION text\x01
				if len(msg) > 8 && strings.HasPrefix(msg, "\x01ACTION ") && strings.HasSuffix(msg, "\x01") {
					msg = strings.TrimPrefix(msg, "\x01ACTION ")
					msg = strings.TrimSuffix(msg, "\x01")
					eventType = "action"
				} else if len(msg) > 0 && msg[0] == '\x01' {
					// Other CTCP messages (VERSION, PING, etc.) — skip storage
					eventType = ""
				}
				if eventType != "" {
					birdbase.StoreChannelMessage(networkID, e.Params[0], e.Source.Name, eventType, msg)
				}
			}
		}
		// Not a command - check if we should handle it as a chat message for participant system
		participant.HandleChatMessage(c, e, network, config)
		return
	}

	// It is a command, so we initialize state and then perform checks.
	irc := state.Init(c, e, network, config)

	if irc.User == nil {
		// This should not happen with the new state.Init logic, but as a safeguard.
		logger.Warn("Dropping message because user state is nil.", "source", e.Source.String())
		return
	}

	// Set the command validator function that state.Verify will use.
	irc.ValidateCommand = func(cmdName string) bool {
		if irc.Channel == nil {
			// Private Message
			return commands.IsValidCommand(cmdName)
		}
		// Channel Message
		return commands.IsValidCommandForChannel(
			cmdName,
			irc.Channel.Ai,
			irc.Channel.Sd,
			irc.Channel.Sound,
			irc.Channel.Video,
			irc.User.IsAdmin,
			irc.User.IsOwner,
		)
	}

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
		checkFlood(irc)

		dispatchCommand(irc, q)
	}
}

// checkFlood implements the first tier of flood protection.
// Called for valid commands only, after Verify() passes.
// Uses nick-based keys and kicks the user from the channel on threshold breach.
// This is the visible punishment layer — separate from MessageFloodCheck which
// silently ignores the user via User.Ignored.
func checkFlood(irc state.State) {
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
