package networks

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/channels"
	"aibird/irc/servers"
	"aibird/irc/users"
	"aibird/logger"
)

func (n *Network) String() string {
	return fmt.Sprintf("{b}Enabled{b}%s, {b}NetworkName{b}: %s, {b}Nick{b}: %s, {b}User{b}: %s, {b}Name{b}: %s, {b}ModesAtOnce{b}: %d, {b}PingDelay{b}: %d, {b}Version{b}: %s, {b}Throttle{b}: %d, {b}Burst{b}: %d, {b}ActionTrigger{b}: %s, {b}Users{b}: %d, {b}Channels{b}: %d, {b}Servers{b}: %d, {b}AdminHosts{b}: %d",
		helpers.StringToStatusIndicator(strconv.FormatBool(n.Enabled)),
		n.NetworkName,
		n.Nick,
		n.User,
		n.Name,
		n.GetModesAtOnce(),
		n.PingDelay,
		n.Version,
		n.Throttle,
		n.Burst,
		n.ActionTrigger,
		len(n.Users),
		len(n.Channels),
		len(n.Servers),
		len(n.AdminHosts))
}

func (n *Network) GetRandomServer() *servers.Server {
	if len(n.Servers) == 0 {
		return nil // or handle the error as appropriate
	}
	// Use crypto/rand for secure random number generation
	randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(n.Servers))))
	if err != nil {
		return nil // fallback to first server if random generation fails
	}
	return &n.Servers[randomIndex.Int64()]
}

func (n *Network) ProvideStateInit(channelName, ident, host string) (*channels.Channel, *users.User) {
	logger.Debug("ProvideStateInit called", "network", n.NetworkName, "channel", channelName, "ident", ident, "host", host)
	channel := n.GetNetworkChannel(channelName)
	user := n.GetUserWithIdentAndHost(ident, host)
	logger.Debug("ProvideStateInit result", "network", n.NetworkName, "channel_found", channel != nil, "user_found", user != nil)
	return channel, user
}

func (n *Network) GetNetworkChannel(channelName string) *channels.Channel {
	for i, channel := range n.Channels {
		if channel.Name == channelName {
			return &n.Channels[i]
		}
	}
	return nil
}

func (n *Network) GetUserWithIdentAndHost(ident, host string) *users.User {
	logger.Debug("GetUserWithIdentAndHost called", "network", n.NetworkName, "ident", ident, "host", host, "total_users", len(n.Users))
	
	var foundUsers []*users.User
	for i := range n.Users {
		if n.Users[i].Ident == ident && n.Users[i].Host == host {
			foundUsers = append(foundUsers, &n.Users[i])
		}
	}

	if len(foundUsers) == 0 {
		logger.Debug("No user found for ident/host", "network", n.NetworkName, "ident", ident, "host", host)
		return nil
	}

	if len(foundUsers) == 1 {
		logger.Debug("Found single user", "network", n.NetworkName, "ident", ident, "host", host, "nickname", foundUsers[0].NickName)
		return foundUsers[0]
	}

	latestUser := foundUsers[0]
	for _, user := range foundUsers {
		if user.LatestActivity > latestUser.LatestActivity {
			latestUser = user
		}
	}

	logger.Debug("Found multiple users, returning latest", "network", n.NetworkName, "ident", ident, "host", host, "nickname", latestUser.NickName, "count", len(foundUsers))
	return latestUser
}

func (n *Network) GetUserWithNick(nick string) *users.User {
	var foundUsers []*users.User
	for i := range n.Users {
		if n.Users[i].NickName == nick {
			foundUsers = append(foundUsers, &n.Users[i])
		}
	}

	if len(foundUsers) == 0 {
		return nil
	}

	if len(foundUsers) == 1 {
		return foundUsers[0]
	}

	latestUser := foundUsers[0]
	for _, user := range foundUsers {
		if user.LatestActivity > latestUser.LatestActivity {
			latestUser = user
		}
	}

	return latestUser
}

func (n *Network) GetModesAtOnce() int {
	if n.ModesAtOnce == 0 {
		return 4
	}
	return n.ModesAtOnce
}

func (n *Network) IsNickIgnored(nick string) bool {
	for _, ignoredNick := range n.IgnoredNicks {
		if ignoredNick == nick {
			return true
		}
	}
	return false
}

func (n *Network) IsIdentHostAdmin(ident, host string) bool {
	for _, admin := range n.AdminHosts {
		if admin.Host == host && admin.Ident == ident {
			return true
		}
	}
	return false
}

func (n *Network) IsIdentHostOwner(ident, host string) bool {
	for _, admin := range n.AdminHosts {
		if admin.Host == host && admin.Ident == ident && admin.Owner {
			return true
		}
	}
	return false
}

func (n *Network) Save() {
	if n.SaveTimer == nil {
		n.SaveTimer = time.NewTimer(0)
		// Drain initial timer
		if !n.SaveTimer.Stop() {
			<-n.SaveTimer.C
		}
	} else if !n.SaveTimer.Stop() {
		select {
		case <-n.SaveTimer.C:
		default:
		}
	}
	n.SaveTimer.Reset(3 * time.Second)

	go func() {
		<-n.SaveTimer.C
		// Use normalized database storage (no JSON fallback needed)
		if err := n.SaveNormalized(); err != nil {
			logger.Error("Error saving network to normalized database", "network", n.NetworkName, "error", err)
		}
	}()
}

// SaveNormalized saves the network using the new normalized database schema
// Uses CONFIGURATION MERGE STRATEGY:
// - Only saves runtime/persistent data (users) to database
// - Configuration data stays in config.toml and is not persisted to DB
func (n *Network) SaveNormalized() error {
	logger.Debug("SaveNormalized called", "network", n.NetworkName, "user_count", len(n.Users))
	logger.Debug("Saving normalized network data (users only)", "network", n.NetworkName, "user_count", len(n.Users))

	// Convert channels from config to ChannelData for database
	var channelsData []birdbase.ChannelData
	for _, channel := range n.Channels {
		channelsData = append(channelsData, birdbase.ChannelData{
			Name:          channel.Name,
			PreserveModes: channel.PreserveModes,
			Ai:            channel.Ai,
			Sd:            channel.Sd,
			ImageDescribe: channel.ImageDescribe,
			Sound:         channel.Sound,
			Video:         channel.Video,
			ActionTrigger: channel.ActionTrigger,
			TrimOutput:    channel.TrimOutput,
			DenyCommands:  channel.DenyCommands,
		})
	}

	// Convert and save users (runtime/persistent data) with channel info
	var usersData []birdbase.UserData
	for _, user := range n.Users {
		userData, err := user.ToUserData(0) // NetworkID will be resolved by SaveNetworkUsers
		if err != nil {
			return err
		}
		usersData = append(usersData, *userData)
	}

	// Save users to normalized database (channels are handled by SaveNetwork)
	logger.Debug("About to save users to database", "network", n.NetworkName, "users_to_save", len(usersData))
	if err := birdbase.SaveNetworkUsers(n.NetworkName, usersData); err != nil {
		logger.Error("Failed to save network users to database", "network", n.NetworkName, "error", err)
		return err
	}

	logger.Debug("Successfully saved network users to normalized database", 
		"network", n.NetworkName, 
		"users_saved", len(usersData))

	return nil
}

func (n *Network) Load() {
	// Use normalized database storage (no JSON fallback needed)
	if err := n.LoadNormalized(); err != nil {
		logger.Warn("Failed to load from normalized database, starting with empty users", "network", n.NetworkName, "error", err)
		// Start with empty users slice - will be populated as users join
		n.Users = make([]users.User, 0)
	}
}

// LoadNormalized loads the network using the new normalized database schema
// Uses CONFIGURATION MERGE STRATEGY:
// - Configuration data (servers, channels, admin hosts) always comes from config.toml
// - Runtime data (users, user modes, activity) comes from database
func (n *Network) LoadNormalized() error {
	// STEP 1: Configuration data stays from config.toml (already loaded)
	// The Network struct already has the current config.toml values for:
	// - n.Servers (from config)
	// - n.Channels (from config) 
	// - n.AdminHosts (from config)
	// - n.IgnoredNicks (from config)
	// - n.DenyCommands (from config)
	// - All other network settings (Nick, User, Name, etc.)
	
	logger.Debug("Loading normalized network data with config merge", "network", n.NetworkName)

	// STEP 2: Ensure network configuration is saved to database (configuration merge strategy)
	// This ensures all networks and channels from config.toml are persisted to database
	channelsData := make([]birdbase.ChannelData, len(n.Channels))
	for i, channel := range n.Channels {
		channelsData[i] = birdbase.ChannelData{
			Name:          channel.Name,
			PreserveModes: channel.PreserveModes,
			Ai:            channel.Ai,
			Sd:            channel.Sd,
			ImageDescribe: channel.ImageDescribe,
			Sound:         channel.Sound,
			Video:         channel.Video,
			ActionTrigger: channel.ActionTrigger,
			TrimOutput:    channel.TrimOutput,
			DenyCommands:  channel.DenyCommands,
		}
	}
	
	// Channel syncing is now handled in SaveNetwork - no need for manual cleanup here

	// Save complete network configuration to database (servers, admin hosts, channels, etc.)
	networkData := &birdbase.NetworkData{
		Enabled:       n.Enabled,
		Nick:          n.Nick,
		User:          n.User,
		Name:          n.Name,
		Pass:          n.Pass,
		PreserveModes: n.PreserveModes,
		NickServPass:  n.NickServPass,
		PingDelay:     n.PingDelay,
		Version:       n.Version,
		Throttle:      n.Throttle,
		Burst:         n.Burst,
		ActionTrigger: n.ActionTrigger,
		ModesAtOnce:   n.ModesAtOnce,
		IgnoredNicks:  n.IgnoredNicks,
		DenyCommands:  n.DenyCommands,
		Channels:      channelsData,
	}
	
	// Convert admin hosts
	for _, admin := range n.AdminHosts {
		networkData.AdminHosts = append(networkData.AdminHosts, birdbase.AdminHost{
			Host:  admin.Host,
			Ident: admin.Ident,
			Owner: admin.Owner,
		})
	}
	
	// Convert servers
	for _, server := range n.Servers {
		networkData.Servers = append(networkData.Servers, birdbase.ServerData{
			Host:          server.Host,
			Port:          server.Port,
			SSL:           server.SSL,
			SkipSSLVerify: server.SkipSslVerify,
		})
	}
	
	if err := birdbase.SaveNetwork(n.NetworkName, networkData); err != nil {
		logger.Warn("Failed to save network configuration to database", "network", n.NetworkName, "error", err)
	}

	// STEP 3: Load ONLY user data from database (runtime/persistent data)
	usersData, err := birdbase.LoadNetworkUsers(n.NetworkName)
	if err != nil {
		// If network doesn't exist in DB yet, that's fine - we'll create it on first save
		if err == sql.ErrNoRows {
			logger.Debug("Network not found in database, will be created on first save", "network", n.NetworkName)
			n.Users = make([]users.User, 0)
			return nil
		}
		return err
	}

	// STEP 4: Convert users back to User structs (preserving runtime data)
	n.Users = nil
	for _, userData := range usersData {
		user := users.NewUserFromData(&userData)
		n.Users = append(n.Users, *user)
	}

	logger.Debug("Successfully loaded network with config merge", 
		"network", n.NetworkName, 
		"users_loaded", len(n.Users),
		"servers_from_config", len(n.Servers),
		"channels_from_config", len(n.Channels))

	return nil
}
