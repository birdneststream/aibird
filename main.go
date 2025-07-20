package main

import (
	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/commands"
	"aibird/irc/commands/help"
	"aibird/irc/networks"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/settings"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-signalCh
		logger.Info("Received shutdown signal, initiating shutdown", "signal", sig)
		cancel()
		close(shutdown)
	}()

	// Init and start the dual queue process
	q := queue.NewDualQueue()
	go q.ProcessQueues()

	var wg sync.WaitGroup

	for i := range config.Networks {
		network := config.Networks[i]
		if !network.Enabled {
			continue
		}
		wg.Add(1)

		go ircClient(ctx, &network, config, q, &wg)
	}

	wg.Wait()
	logger.Info("All IRC connections terminated, shutting down")
}

func ircClient(ctx context.Context, network *networks.Network, config *settings.Config, q *queue.DualQueue, wg *sync.WaitGroup) {
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
			InsecureSkipVerify: true,
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
	client.Handlers.Add(girc.JOIN, func(c *girc.Client, e girc.Event) { handleJoin(c, e, network, config) })
	client.Handlers.Add(girc.MODE, func(c *girc.Client, e girc.Event) { handleMode(c, e, network) })
	client.Handlers.Add(girc.KICK, func(c *girc.Client, e girc.Event) { handleKick(c, e, config) })
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
	if e.Source.Name == network.Nick {
		network.Nick = e.Last()
		return
	}
	if findUser := network.GetUserWithNick(e.Source.Name); findUser != nil {
		findUser.UpdateNick(e.Last())
	}
}

func handleWhoReply(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	irc := state.Init(c, e, network, config)
	if irc.Channel != nil {
		go irc.SyncUsersFromWho()
	}
}

func handleJoin(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	if e.Source.Name == network.Nick {
		return
	}
	if existingUser := network.GetUserWithNick(e.Source.Name); existingUser != nil {
		existingUser.UpdateIdentHost(e.Source.Ident, e.Source.Host)
	}
	irc := state.Init(c, e, network, config)
	if irc.Channel != nil {
		go irc.DelayedWhoTimer()
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

func handleKick(c *girc.Client, e girc.Event, config *settings.Config) {
	if e.Params[1] == c.GetNick() {
		delay := time.Duration(config.AiBird.KickRetryDelay) * time.Second
		if delay == 0 {
			delay = 5 * time.Second // Default value if not set
		}
		time.Sleep(delay)
		c.Cmd.Join(helpers.FindChannelNameInEventParams(e))
	}
}

func handlePrivMsg(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config, q *queue.DualQueue) {
	// Lightweight check for command trigger before initializing state. If it's not a command, do nothing.
	if !strings.HasPrefix(e.Last(), config.AiBird.ActionTrigger) {
		return
	}

	// It is a command, so we initialize state and then perform checks.
	irc := state.Init(c, e, network, config)

	// Only check for flooding on messages that are commands.
	checkFlood(irc)

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

	if birdbase.Has(ban) {
		return
	}

	floodWindow := 1

	if !birdbase.Has(key) {
		if err := birdbase.PutStringExpireSeconds(key, "1", floodWindow); err != nil {
			logger.Warn("Failed to set flood key in birdbase", "key", key, "error", err)
		}
	} else {
		countBytes, _ := birdbase.Get(key)
		count := string(countBytes)
		countInt, _ := strconv.Atoi(count)
		countInt++
		if err := birdbase.PutStringExpireSeconds(key, strconv.Itoa(countInt), floodWindow); err != nil {
			logger.Warn("Failed to update flood key in birdbase", "key", key, "error", err)
		}

		if countInt > config.FloodThreshold {
			if err := birdbase.PutStringExpireSeconds(ban, "1", config.FloodIgnoreMinutes*60); err != nil {
				logger.Warn("Failed to set flood-ban key in birdbase", "key", ban, "error", err)
			}
			irc.Client.Cmd.Kick(irc.Channel.Name, irc.Event.Source.Name, "Birds fly above floods!")
		}
	}
}

func dispatchCommand(irc state.State, q *queue.DualQueue) {
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

	if irc.FindArgument("help", false).(bool) {
		helpMsg := help.FindHelp(irc)
		irc.Send(girc.Fmt(helpMsg))
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
