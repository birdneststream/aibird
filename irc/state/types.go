package state

import (
	"aibird/irc/channels"
	"aibird/irc/networks"
	"aibird/irc/users"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

type (
	// CommandValidator is a function that takes a command string and returns if it's valid
	CommandValidator func(string) bool

	Command struct {
		Action  string
		Message string
	}

	State struct {
		Client    *girc.Client
		Event     girc.Event
		Network   *networks.Network
		User      *users.User
		Channel   *channels.Channel
		Command   Command
		Arguments []Argument
		Config    *settings.Config

		// Function to validate commands - set by the main package
		ValidateCommand CommandValidator
	}

	Argument struct {
		Key   string
		Value interface{}
	}
)

func (s *State) GetConfig() *settings.Config {
	return s.Config
}
