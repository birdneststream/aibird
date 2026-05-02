package channels

import (
	"aibird/irc/users"
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
		// Participant system fields
		ChatMode         bool    `toml:"chatMode"`         // Enable AI participant mode
		CompanionMode    bool    `toml:"companionMode"`    // Enable companion personality
		ChatPersonality  string  `toml:"chatPersonality"`  // Personality profile name
		ChatResponseRate float64 `toml:"chatResponseRate"` // Base response probability (0.0-1.0)
	}
)
