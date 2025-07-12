package networks

import (
	"aibird/irc/channels"
	"aibird/irc/servers"
	"aibird/irc/users"
	"time"
)

type (
	Admins struct {
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
		ModesAtOnce   int
		Users         []users.User
		Servers       []servers.Server
		Channels      []channels.Channel
		AdminHosts    []Admins
		SaveTimer     *time.Timer
	}
)
