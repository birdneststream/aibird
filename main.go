package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"aibird/birdbase"
	"aibird/irc/commands"
	"aibird/irc/networks"
	"aibird/irc/participant"
	"aibird/logger"
	"aibird/queue"
	"aibird/settings"

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

	// Initialize command registry for O(1) lookups
	commands.InitRegistry(config.AiBird)

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
