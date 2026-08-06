package commands

import (
	"testing"

	"aibird/irc/channels"
	"aibird/irc/state"
	"aibird/irc/users"

	"github.com/stretchr/testify/assert"
)

// TestPremiumVideoWorkflowConstant documents the h3 workflow mapping.
func TestPremiumVideoWorkflowConstant(t *testing.T) {
	assert.Equal(t, "h3", premiumVideoWorkflow, "the h3 video command runs h3.json")
	assert.Equal(t, "#h4x", freeAccessChannel)
}

func TestIsEnhancedVideoCommand(t *testing.T) {
	assert.True(t, isEnhancedVideoCommand("h3"))
	assert.True(t, isEnhancedVideoCommand("H3"), "matching is case-insensitive")
	for _, bad := range []string{"hello", "sienfield", "garfield", "ai", ""} {
		assert.False(t, isEnhancedVideoCommand(bad), "%q should not be the h3 video command", bad)
	}
}

func TestHasPremiumAccess(t *testing.T) {
	tests := []struct {
		name        string
		channelName string
		channelNil  bool
		accessLevel int
		userNil     bool
		want        bool
	}{
		{"#h4x free for free user", "#h4x", false, 0, false, true},
		{"#h4x free for patreon", "#h4x", false, 5, false, true},
		{"#h4x case-insensitive", "#H4X", false, 0, false, true},
		{"patreon swan squadron in other channel", "#general", false, 2, false, true},
		{"patreon top tier in other channel", "#general", false, 5, false, true},
		{"free user in non-h4x channel denied", "#general", false, 0, false, false},
		{"chat pal (level 1) denied outside #h4x", "#general", false, 1, false, false},
		{"nil channel free user denied", "", true, 0, false, false},
		{"nil user in non-h4x channel denied", "#general", false, 0, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &state.State{}
			if !tt.channelNil {
				s.Channel = &channels.Channel{Name: tt.channelName}
			}
			if !tt.userNil {
				s.User = &users.User{AccessLevel: tt.accessLevel}
			}
			assert.Equal(t, tt.want, hasPremiumAccess(s))
		})
	}
}

// TestH3_RegistryRouting verifies the h3 command is registered as a
// queueable video command (locks in the dispatch + help wiring).
func TestH3_RegistryRouting(t *testing.T) {
	assert.True(t, IsQueueableFromHelp("h3"), "h3 should be queueable")
	assert.True(t, IsVideoCommand("h3"), "h3 should be a video command")
}
