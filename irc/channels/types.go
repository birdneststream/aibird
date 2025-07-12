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
		ActionTrigger string
		Users         []*users.User
		TrimOutput    bool
		ActivityTimer *time.Timer // Used in DelayedWhoTimer to prevent multiple who requests
	}
)
