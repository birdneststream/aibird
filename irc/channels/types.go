package channels

import (
	"aibird/irc/users"
	"time"
)

type (
	Channel struct {
		Name          string
		PreserveModes bool
		Ai            bool
		Sd            bool
		ImageDescribe bool
		Sound         bool
		Video         bool
		ActionTrigger string
		DenyCommands  []string `toml:"denyCommands"`
		Users         []*users.User
		TrimOutput    bool
		ActivityTimer *time.Timer // Used in DelayedWhoTimer to prevent multiple who requests
	}
)
