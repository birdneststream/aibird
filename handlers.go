package main

import (
	"time"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/networks"
	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/logger"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

func handleWelcome(c *girc.Client, e girc.Event, network *networks.Network) {
	// Set connection time to ignore ZNC buffer playback
	network.ConnectedAt = time.Now()
	logger.Info("Connected to network", "network", network.NetworkName, "connected_at", network.ConnectedAt)

	if network.NickServPass != "" {
		if err := c.Cmd.SendRaw("PRIVMSG NickServ :IDENTIFY " + network.Nick + " " + network.NickServPass); err != nil {
			logger.Warn("Error sending NickServ identify", "network", network.Name, "error", err)
		}
	}
	for _, channel := range network.Channels {
		c.Cmd.Join(channel.Name)
		if err := c.Cmd.SendRaw("WHO " + channel.Name); err != nil {
			logger.Warn("Error sending WHO command", "channel", channel.Name, "network", network.Name, "error", err)
		}
	}
}

func handleNick(c *girc.Client, e girc.Event, network *networks.Network) {
	logger.Debug("Nick change event", "network", network.NetworkName, "old_nick", e.Source.Name, "new_nick", e.Last())

	if e.Source.Name == network.Nick {
		logger.Debug("Bot nick changed", "network", network.NetworkName, "old_nick", network.Nick, "new_nick", e.Last())
		network.Nick = e.Last()
		return
	}

	if findUser := network.GetUserWithNick(e.Source.Name); findUser != nil {
		logger.Debug("Found user for nick change", "network", network.NetworkName, "old_nick", e.Source.Name, "new_nick", e.Last(), "ident", findUser.Ident, "host", findUser.Host)
		findUser.UpdateNick(e.Last())
		logger.Debug("User nick updated", "network", network.NetworkName, "new_nick", findUser.NickName, "ident", findUser.Ident, "host", findUser.Host)

		// Save just this user instead of the entire network (performance optimization)
		if userData, err := findUser.ToUserData(0); err != nil {
			logger.Error("Failed to convert user to UserData for nick change", "error", err, "network", network.NetworkName, "nick", findUser.NickName)
		} else if err := birdbase.SaveSingleUser(network.NetworkName, findUser.Ident, findUser.Host, userData); err != nil {
			logger.Error("Failed to save user after nick change", "error", err, "network", network.NetworkName, "nick", findUser.NickName)
		} else {
			logger.Debug("Successfully saved user after nick change", "network", network.NetworkName, "new_nick", findUser.NickName)
		}
	} else {
		logger.Debug("User not found for nick change", "network", network.NetworkName, "old_nick", e.Source.Name, "new_nick", e.Last(), "total_users", len(network.Users))
	}
}

func handleWhoReply(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	irc := state.Init(c, e, network, config)
	if irc.Channel != nil {
		go irc.SyncUsersFromWho()
	}
}

func handleEndOfWho(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	// RPL_ENDOFWHO indicates WHO command is complete for this channel
	// This is when we should restore missing user modes if this was a sync command
	logger.Debug("END_OF_WHO received", "network", network.NetworkName, "params", e.Params)

	if len(e.Params) < 2 {
		return
	}

	channelName := e.Params[1] // Channel name from END_OF_WHO
	irc := state.Init(c, e, network, config)

	// Override channel since e.Params doesn't contain it in the right place for state.Init
	if channel := network.GetNetworkChannel(channelName); channel != nil {
		irc.Channel = channel
		go irc.RestoreUserModes()
	}
}

func handleJoin(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	logger.Debug("JOIN event", "network", network.NetworkName, "nick", e.Source.Name, "channel", e.Params[0], "ident", e.Source.Ident, "host", e.Source.Host)

	if e.Source.Name == network.Nick {
		logger.Debug("Bot joined channel - skipping user tracking", "network", network.NetworkName, "channel", e.Params[0])
		return
	}

	// Check for existing user by nick first
	var existingUser *users.User
	if userByNick := network.GetUserWithNick(e.Source.Name); userByNick != nil {
		existingUser = userByNick
		existingUser.UpdateIdentHost(e.Source.Ident, e.Source.Host)
		logger.Debug("Updated existing user ident/host", "network", network.NetworkName, "nick", e.Source.Name, "ident", e.Source.Ident, "host", e.Source.Host)
	} else {
		// Check for existing user by ident@host (nick change case)
		if userByIdentHost := network.GetUserWithIdentAndHost(e.Source.Ident, e.Source.Host); userByIdentHost != nil {
			existingUser = userByIdentHost
			existingUser.UpdateNick(e.Source.Name)
			logger.Debug("Updated existing user nick", "network", network.NetworkName, "old_nick", existingUser.NickName, "new_nick", e.Source.Name, "ident", e.Source.Ident, "host", e.Source.Host)
		}
	}

	// Save user update if we found an existing user, or create a new user
	if existingUser != nil {
		if userData, err := existingUser.ToUserData(0); err != nil {
			logger.Warn("Failed to convert user to UserData after update", "error", err, "network", network.NetworkName, "nick", existingUser.NickName)
		} else if err := birdbase.SaveSingleUser(network.NetworkName, existingUser.Ident, existingUser.Host, userData); err != nil {
			logger.Warn("Failed to save user after update", "error", err, "network", network.NetworkName, "nick", existingUser.NickName)
		}
	} else {
		// Create new user if they don't exist in our network
		logger.Debug("Creating new user from JOIN event", "network", network.NetworkName, "nick", e.Source.Name, "ident", e.Source.Ident, "host", e.Source.Host)
		newUser := users.User{
			NickName:    e.Source.Name,
			Ident:       e.Source.Ident,
			Host:        e.Source.Host,
			FirstSeen:   time.Now().Unix(),
			IsAdmin:     network.IsIdentHostAdmin(e.Source.Ident, e.Source.Host),
			IsOwner:     network.IsIdentHostOwner(e.Source.Ident, e.Source.Host),
			Ignored:     network.IsNickIgnored(e.Source.Name),
			AccessLevel: 0,
			AiService:   "llamacpp",
			GircUser:    c.LookupUser(e.Source.Name),
		}

		// Add to network users list
		network.Users = append(network.Users, newUser)

		// Save new user to database
		if userData, err := newUser.ToUserData(0); err != nil {
			logger.Warn("Failed to convert new user to UserData", "error", err, "network", network.NetworkName, "nick", newUser.NickName)
		} else if err := birdbase.SaveSingleUser(network.NetworkName, newUser.Ident, newUser.Host, userData); err != nil {
			logger.Warn("Failed to save new user", "error", err, "network", network.NetworkName, "nick", newUser.NickName)
		}
	}

	// Track user joining this channel
	if err := birdbase.AddUserToChannel(network.NetworkName, e.Source.Ident, e.Source.Host, e.Params[0]); err != nil {
		logger.Warn("Failed to track user joining channel", "error", err, "network", network.NetworkName, "nick", e.Source.Name, "channel", e.Params[0])
	}

	// Store JOIN event for summary feature
	networkID := birdbase.ResolveNetworkID(network.NetworkName)
	if networkID > 0 {
		birdbase.StoreChannelMessage(networkID, e.Params[0], e.Source.Name, "join", "")
	}

	// Skip DelayedWhoTimer for performance during netsplits - user_channels are already tracked above
	// irc := state.Init(c, e, network, config)
	// if irc.Channel != nil {
	//	go irc.DelayedWhoTimer()
	// }
}

func handlePart(c *girc.Client, e girc.Event, network *networks.Network) {
	logger.Debug("PART event", "network", network.NetworkName, "nick", e.Source.Name, "channel", e.Params[0], "ident", e.Source.Ident, "host", e.Source.Host)

	if e.Source.Name == network.Nick {
		logger.Debug("Bot left channel - skipping user tracking", "network", network.NetworkName, "channel", e.Params[0])
		return
	}

	// Remove user from this channel
	if err := birdbase.RemoveUserFromChannel(network.NetworkName, e.Source.Ident, e.Source.Host, e.Params[0]); err != nil {
		logger.Warn("Failed to track user leaving channel", "error", err, "network", network.NetworkName, "nick", e.Source.Name, "channel", e.Params[0])
	}

	// Store PART event for summary feature
	networkID := birdbase.ResolveNetworkID(network.NetworkName)
	if networkID > 0 {
		partMsg := ""
		if len(e.Params) > 1 {
			partMsg = e.Params[1]
		}
		birdbase.StoreChannelMessage(networkID, e.Params[0], e.Source.Name, "part", partMsg)
	}
}

func handleQuit(c *girc.Client, e girc.Event, network *networks.Network) {
	logger.Debug("QUIT event", "network", network.NetworkName, "nick", e.Source.Name, "ident", e.Source.Ident, "host", e.Source.Host)

	if e.Source.Name == network.Nick {
		logger.Debug("Bot quit - skipping user tracking", "network", network.NetworkName)
		return
	}

	// Remove user from all channels on this network
	if err := birdbase.RemoveUserFromAllChannels(network.NetworkName, e.Source.Ident, e.Source.Host); err != nil {
		logger.Warn("Failed to track user quitting", "error", err, "network", network.NetworkName, "nick", e.Source.Name)
	}

	// Store QUIT event for summary feature — store against each channel the user was in
	networkID := birdbase.ResolveNetworkID(network.NetworkName)
	if networkID > 0 {
		quitMsg := ""
		if len(e.Params) > 0 {
			quitMsg = e.Last()
		}
		for _, ch := range network.Channels {
			if user, err := ch.GetUserWithNick(e.Source.Name); err == nil && user != nil {
				birdbase.StoreChannelMessage(networkID, ch.Name, e.Source.Name, "quit", quitMsg)
			}
		}
	}
}

func handleMode(c *girc.Client, e girc.Event, network *networks.Network) {
	if len(e.Params) < 3 {
		return
	}
	irc := state.Init(c, e, network, nil) // Config not needed for mode changes
	if irc.IsSelf() {
		return
	}

	modeChanges := e.Params[1]
	users := e.Params[2:]
	isAdding := false
	userIndex := 0

	for _, mode := range modeChanges {
		switch mode {
		case '+', '-':
			isAdding = (mode == '+')
		default:
			if userIndex < len(users) {
				user, _ := irc.Channel.GetUserWithNick(users[userIndex])
				if user != nil {
					mappedMode := helpers.ModeMap(mode)
					if isAdding {
						logger.Debug("Adding mode to user", "mode", mappedMode, "user", user.NickName)
						irc.Channel.SyncMode(user, mappedMode)
					} else {
						logger.Debug("Removing mode from user", "mode", mappedMode, "user", user.NickName)
						irc.Channel.ForgetMode(user, mappedMode)
					}
				}
				userIndex++
			}
		}
	}
	go network.Save()
}

func handleKick(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	// e.Params[0] = channel, e.Params[1] = kicked user, e.Params[2] = reason
	kickedNick := e.Params[1]
	channelName := e.Params[0]

	logger.Debug("KICK event", "channel", channelName, "kicked_nick", kickedNick, "kicker", e.Source.Name)

	if kickedNick == c.GetNick() {
		// Bot was kicked - handle rejoin
		logger.Info("Bot was kicked from channel", "channel", channelName, "kicker", e.Source.Name)
		delay := time.Duration(config.AiBird.KickRetryDelay) * time.Second
		if delay == 0 {
			delay = 5 * time.Second // Default value if not set
		}
		time.Sleep(delay)
		c.Cmd.Join(helpers.FindChannelNameInEventParams(e))
	} else {
		// Someone else was kicked - remove them from user_channels
		logger.Debug("User kicked from channel", "channel", channelName, "kicked_nick", kickedNick, "kicker", e.Source.Name)

		// Store KICK event for summary feature
		networkID := birdbase.ResolveNetworkID(network.NetworkName)
		if networkID > 0 {
			kickReason := ""
			if len(e.Params) > 2 {
				kickReason = e.Params[2]
			}
			birdbase.StoreChannelMessage(networkID, channelName, kickedNick, "kick", e.Source.Name+" "+kickReason)
		}

		// We need to find the kicked user's ident/host to track properly
		// Since KICK event doesn't provide ident/host, we'll look up the user in our network state
		if kickedUser := network.GetUserWithNick(kickedNick); kickedUser != nil {
			if err := birdbase.RemoveUserFromChannel(network.NetworkName, kickedUser.Ident, kickedUser.Host, channelName); err != nil {
				logger.Warn("Failed to track kicked user leaving channel", "error", err, "network", network.NetworkName, "nick", kickedNick, "channel", channelName)
			}
		} else {
			logger.Debug("Cannot track kicked user - not found in network state", "network", network.NetworkName, "nick", kickedNick, "channel", channelName)
		}
	}
}
