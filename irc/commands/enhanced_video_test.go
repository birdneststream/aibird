package commands

import (
	"strings"
	"testing"

	"aibird/irc/channels"
	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/text"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPremiumVideoWorkflowConstant documents the shared workflow mapping.
func TestPremiumVideoWorkflowConstant(t *testing.T) {
	assert.Equal(t, "m3", premiumVideoWorkflow, "all enhanced-video commands run m3.json")
	assert.Equal(t, "#h4x", freeAccessChannel)
}

func TestEnhancedVideoTemplate(t *testing.T) {
	// Every registered command must look up its own template (derived from the
	// map so the test stays in sync without repeating literals).
	for cmd, tpl := range enhancedVideoTemplates {
		t.Run(cmd, func(t *testing.T) {
			got, ok := enhancedVideoTemplate(cmd)
			assert.True(t, ok, "%s should be an enhanced-video command", cmd)
			assert.Equal(t, tpl, got)
			assert.True(t, isEnhancedVideoCommand(cmd))
		})
	}
	// Non-commands and case-insensitive matching.
	for _, bad := range []string{"hello", "ai", ""} {
		_, ok := enhancedVideoTemplate(bad)
		assert.False(t, ok, "%q should not be an enhanced-video command", bad)
	}
	// Lookups are case-insensitive.
	tpl, ok := enhancedVideoTemplate(strings.ToUpper("h3"))
	assert.True(t, ok)
	assert.NotEmpty(t, tpl)
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

// TestEnhancedVideo_RegistryRouting verifies all registered commands are
// queueable video commands (locks in the dispatch + help wiring).
func TestEnhancedVideo_RegistryRouting(t *testing.T) {
	for cmd := range enhancedVideoTemplates {
		assert.True(t, IsQueueableFromHelp(cmd), "%s should be queueable", cmd)
		assert.True(t, IsVideoCommand(cmd), "%s should be a video command", cmd)
	}
}

// TestEnhancedVideo_TemplatesLoad verifies every registered template exists on
// disk and begins with the /no_think directive (so the model skips reasoning).
func TestEnhancedVideo_TemplatesLoad(t *testing.T) {
	for cmd, tpl := range enhancedVideoTemplates {
		t.Run(cmd, func(t *testing.T) {
			prompt, err := text.GetPrompt(tpl)
			require.NoError(t, err, "template %s for !%s must exist", tpl, cmd)
			require.NotEmpty(t, prompt)
			assert.True(t, len(prompt) > len("/no_think") && prompt[:len("/no_think")] == "/no_think",
				"template %s must start with /no_think", tpl)
		})
	}
}
