package users

import (
	"aibird/irc/users/modes"

	"github.com/lrstanley/girc"
)

type (
	User struct {
		NickName       string
		Ident          string
		Host           string
		LatestActivity int64
		FirstSeen      int64
		LatestChat     string
		PreservedModes []modes.UserModes
		CurrentModes   []modes.UserModes
		IsAdmin        bool
		IsOwner        bool
		Ignored        bool
		AccessLevel    int
		GircUser       *girc.User

		// Users settings for !ai use
		AiService     string
		AiModel       string
		AiBasePrompt  string
		AiPersonality string
	}
)
