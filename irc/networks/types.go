package networks

import (
	"time"

	"aibird/irc/channels"
	"aibird/irc/servers"
	"aibird/irc/users"
)

type (
	AdminHost struct {
		Host  string
		Ident string
		Owner bool
	}

	Network struct {
		Enabled       bool
		NetworkName   string
		Nick          string
		User          string
		Name          string
		Pass          string
		PreserveModes bool
		IgnoredNicks  []string
		NickServPass  string
		PingDelay     int
		Version       string
		Throttle      int
		Burst         int
		ActionTrigger string
		DenyCommands  []string `toml:"denyCommands"`
		ModesAtOnce   int
		Users         []users.User
		Servers       []servers.Server
		Channels      []channels.Channel
		AdminHosts    []AdminHost
		ConnectedAt   time.Time // Track when bot connected to ignore ZNC buffer playback
	}
)
