package help

import (
	"strings"
	"testing"

	"aibird/irc/channels"
	"aibird/irc/networks"
	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/settings"

	"github.com/stretchr/testify/assert"
)

func TestFindHelp_OwnerCommandsVisibleForOwner(t *testing.T) {
	irc := &state.State{
		Command: state.Command{Action: "help"},
		Network: &networks.Network{NetworkName: "testnet"},
		User:    &users.User{NickName: "owner", IsOwner: true},
		Channel: &channels.Channel{Name: "#test"},
		Config:  &settings.Config{},
	}

	result := FindHelp(irc)
	assert.True(t, strings.Contains(result, "debug"), "owner should see 'debug' command in help")
	assert.True(t, strings.Contains(result, "dbstats"), "owner should see 'dbstats' command in help")
}

func TestFindHelp_OwnerCommandsHiddenForNonOwner(t *testing.T) {
	irc := &state.State{
		Command: state.Command{Action: "help"},
		Network: &networks.Network{NetworkName: "testnet"},
		User:    &users.User{NickName: "regular", IsOwner: false},
		Channel: &channels.Channel{Name: "#test"},
		Config:  &settings.Config{},
	}

	result := FindHelp(irc)
	assert.False(t, strings.Contains(result, "debug"), "non-owner should NOT see 'debug' command")
	assert.False(t, strings.Contains(result, "dbstats"), "non-owner should NOT see 'dbstats' command")
}

func TestFindHelp_StandardCommandsAlwaysVisible(t *testing.T) {
	irc := &state.State{
		Command: state.Command{Action: "help"},
		Network: &networks.Network{NetworkName: "testnet"},
		User:    &users.User{NickName: "regular"},
		Channel: &channels.Channel{Name: "#test"},
		Config:  &settings.Config{},
	}

	result := FindHelp(irc)
	assert.True(t, strings.Contains(result, "hello"), "all users should see 'hello' command")
}
