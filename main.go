package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/commands"
	"aibird/irc/commands/help"
	"aibird/irc/networks"
	"aibird/irc/participant"
	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/logger"
	"aibird/queue"
	"aibird/settings"

	"aibird/shared/meta"

	"github.com/lrstanley/girc"
)

var shutdown = make(chan struct{})

func main() {
	// Load configuration
	config, err := settings.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	logger.Init(config.Logging)

	// Initialize database
	birdbase.Init()
	defer birdbase.Close()

	// Initialize participant system
	participant.InitParticipant(config)

	// Clean up orphaned networks (exist in DB but not in config)
	cleanupOrphanedNetworks(config)

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalCh
		logger.Info("SIGNAL RECEIVED: Initiating shutdown", "signal", sig, "timestamp", time.Now())
		cancel()
		close(shutdown)

		// Force exit after timeout if shutdown hangs
		go func() {
			time.Sleep(30 * time.Second)
			logger.Warn("Shutdown timeout reached, forcing exit")
			os.Exit(1)
		}()
	}()

	var wg sync.WaitGroup

	// Init and start the dual queue process
	q := queue.NewProcessingQueue()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := q.ProcessQueues(ctx); err != nil && err != context.Canceled {
			logger.Error("Queue processing error", "error", err)
		}
	}()

	for i := range config.Networks {
		network := config.Networks[i]
		if !network.Enabled {
			continue
		}
		wg.Add(1)

		go ircClient(ctx, &network, config, q, &wg)
	}

	// Wait for all connections to terminate
	wg.Wait()
	logger.Info("All IRC connections terminated, shutting down")
}

// cleanupOrphanedNetworks removes networks that exist in database but not in config
func cleanupOrphanedNetworks(config *settings.Config) {
	logger.Debug("Checking for orphaned networks to cleanup")

	// Get all networks from database
	dbNetworks, err := birdbase.GetAllNetworkNames()
	if err != nil {
		logger.Warn("Failed to get database networks for cleanup", "error", err)
		return
	}

	// Build map of networks from config
	configNetworks := make(map[string]bool)
	for _, network := range config.Networks {
		configNetworks[network.NetworkName] = true
	}

	// Delete networks that exist in DB but not in config
	for _, dbNetwork := range dbNetworks {
		if !configNetworks[dbNetwork] {
			logger.Warn("Network exists in database but not in config - cleaning up", "network", dbNetwork)

			if err := birdbase.DeleteNetwork(dbNetwork); err != nil {
				logger.Error("Failed to delete orphaned network", "error", err, "network", dbNetwork)
			} else {
				logger.Info("Deleted orphaned network and all related data", "network", dbNetwork)
			}
		}
	}
}

func ircClient(ctx context.Context, network *networks.Network, config *settings.Config, q *queue.ProcessingQueue, wg *sync.WaitGroup) {
	defer wg.Done()
	network.Load()
	logger.Info("Connecting to network", "network", network.Name)

	server := network.GetRandomServer()

	ircConfig := girc.Config{
		Server:     server.Host,
		Port:       server.Port,
		Nick:       network.Nick,
		User:       network.User,
		Name:       network.Name,
		SSL:        server.SSL,
		Version:    network.Version,
		AllowFlood: network.Throttle == 0,
		PingDelay:  time.Second * time.Duration(network.PingDelay),
	}

	if server.SSL && server.SkipSslVerify {
		// WARNING: InsecureSkipVerify bypasses certificate validation
		// This should only be used for testing or when connecting to servers with self-signed certificates
		ircConfig.TLSConfig = &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 - Intentional for IRC servers with self-signed certificates
		}
	}

	if network.Pass != "" {
		ircConfig.ServerPass = network.Pass
	}

	client := girc.New(ircConfig)

	// Register handlers
	client.Handlers.Add(girc.RPL_WELCOME, func(c *girc.Client, e girc.Event) { handleWelcome(c, e, network) })
	client.Handlers.Add(girc.NICK, func(c *girc.Client, e girc.Event) { handleNick(c, e, network) })
	client.Handlers.Add(girc.RPL_WHOREPLY, func(c *girc.Client, e girc.Event) { handleWhoReply(c, e, network, config) })
	client.Handlers.Add(girc.RPL_ENDOFWHO, func(c *girc.Client, e girc.Event) { handleEndOfWho(c, e, network, config) })
	client.Handlers.Add(girc.JOIN, func(c *girc.Client, e girc.Event) { handleJoin(c, e, network, config) })
	client.Handlers.Add(girc.PART, func(c *girc.Client, e girc.Event) { handlePart(c, e, network) })
	client.Handlers.Add(girc.QUIT, func(c *girc.Client, e girc.Event) { handleQuit(c, e, network) })
	client.Handlers.Add(girc.MODE, func(c *girc.Client, e girc.Event) { handleMode(c, e, network) })
	client.Handlers.Add(girc.KICK, func(c *girc.Client, e girc.Event) { handleKick(c, e, network, config) })
	client.Handlers.Add(girc.PRIVMSG, func(c *girc.Client, e girc.Event) { handlePrivMsg(c, e, network, config, q) })

	// This goroutine listens for the shutdown signal and closes the client
	// to unblock the main connection loop.
	go func() {
		<-ctx.Done()
		client.Close()
	}()

	// Connect loop with exponential backoff
	const minBackoff = 5 * time.Second
	const maxBackoff = 300 * time.Second
	backoff := minBackoff

	for {
		select {
		case <-ctx.Done():
			logger.Info("Disconnecting from network", "network", network.Name)
			client.Close()
			return
		default:
			logger.Info("Attempting to connect to IRC", "network", network.Name, "server", client.Server())
			if err := client.Connect(); err != nil {
				logger.Error("Error connecting to IRC", "network", network.Name, "error", err)
				logger.Info("Reconnecting...", "delay", backoff)
				time.Sleep(backoff)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			} else {
				// Reset backoff after a successful connection
				backoff = minBackoff
				// This is a blocking call, it will return when disconnected.
				// We loop again to reconnect.
				logger.Warn("Disconnected from network, will attempt to reconnect...", "network", network.Name)
			}
		}
	}
}

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

func handlePrivMsg(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config, q *queue.ProcessingQueue) {
	// Ignore ZNC buffer playback
	if !network.ConnectedAt.IsZero() {
		// Method 1: If server-time is supported, check if message is from before we connected
		if !e.Timestamp.IsZero() && e.Timestamp.Before(network.ConnectedAt) {
			logger.Debug("Ignoring ZNC buffer playback (server-time)", "network", network.NetworkName, "msg_time", e.Timestamp, "connected_at", network.ConnectedAt)
			return
		}
		// Method 2: For networks without server-time (like EFNet), ignore commands in first 15 seconds
		// ZNC buffer playback happens immediately on connect, longer grace period for networks with many channels
		if e.Timestamp.IsZero() && time.Since(network.ConnectedAt) < 15*time.Second {
			if strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger) {
				logger.Debug("Ignoring potential ZNC buffer command (startup grace period)", "network", network.NetworkName, "elapsed", time.Since(network.ConnectedAt))
				return
			}
		}
	}

	// Check if this is a command (starts with trigger)
	isCommand := strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger)

	if !isCommand {
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
			return commands.IsValidCommand(cmdName, config.AiBird)
		}
		// Channel Message
		return commands.IsValidCommandForChannel(
			cmdName,
			config.AiBird,
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

// checkFlood checks for user flooding and bans them if necessary.
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

func dispatchCommand(irc state.State, q *queue.ProcessingQueue) {
	// Check if the command is denied at any level
	action := irc.Action()
	if irc.Channel != nil {
		for _, deniedCmd := range irc.Channel.DenyCommands {
			if strings.EqualFold(action, deniedCmd) {
				return
			}
		}
	}
	if irc.Network != nil {
		for _, deniedCmd := range irc.Network.DenyCommands {
			if strings.EqualFold(action, deniedCmd) {
				return
			}
		}
	}
	for _, deniedCmd := range irc.Config.AiBird.DenyCommands {
		if strings.EqualFold(action, deniedCmd) {
			return
		}
	}

	if helpArg, ok := irc.FindArgument("help", false).(bool); ok && helpArg {
		helpMsg := help.FindHelp(irc)
		irc.Send(girc.Fmt(helpMsg))
		return
	}

	// Special handling for !ai with llama.cpp - needs queue and can_use check
	if commands.ShouldQueueLlamaCppAi(irc) {
		// Check can_use flag before queueing
		if err := commands.CheckAiCanUse(irc); err != nil {
			irc.SendError(err.Error())
			return
		}

		queueItem := queue.QueueItem{
			Item: queue.Item{
				State: irc,
				Function: func(s state.State, gpu meta.GPUType) {
					commands.ProcessLlamaCppAiRequest(s, gpu)
				},
			},
			Model: "llamacpp-ai", // Special identifier for llama.cpp requests
			User:  irc.User,
			GPU:   meta.GPU4090,
		}

		msg, err := q.Enqueue(queueItem)
		if err != nil {
			irc.SendError(err.Error())
		} else if msg != "" {
			irc.Send(msg)
		}
		return
	}

	if commands.IsQueueableCommand(irc) {
		// Create QueueItem with model information
		queueItem := queue.QueueItem{
			Item: queue.Item{
				State: irc,
				Function: func(s state.State, gpu meta.GPUType) {
					commands.RunQueueableCommand(s, gpu)
				},
			},
			Model: irc.Action(), // Use the command as the model identifier
			User:  irc.User,     // User implements UserAccess interface
		}

		msg, err := q.Enqueue(queueItem)
		if err != nil {
			irc.SendError(err.Error())
		} else if msg != "" {
			irc.Send(msg)
		}
	} else {
		// Not a queueable command, so we find the correct parser
		if commands.IsTextCommand(irc.Action()) {
			commands.ParseAiText(irc)
			return
		}
		switch {
		case commands.IsStandardCommand(irc.Action()):
			go commands.ParseStandardWithQueue(irc, q)
		case commands.IsAdminCommand(irc.Action()):
			go commands.ParseAdminWithQueue(irc, q)
		case commands.IsOwnerCommand(irc.Action()):
			go commands.ParseOwner(irc)
		case commands.IsSoundCommand(irc.Action(), irc.Config.AiBird):
			go commands.ParseAiSound(irc)
		case commands.IsVideoCommand(irc.Action(), irc.Config.AiBird):
			go commands.ParseAiVideo(irc)
		default:
			logger.Warn("Command was valid but no parser was found", "command", irc.Action())
		}
	}
}
