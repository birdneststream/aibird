package commands

import (
	"fmt"
	"strings"
	"testing"

	"aibird/birdbase"
	"aibird/irc/channels"
	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/text"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPremiumVideoWorkflowConstant documents the shared workflow mapping.
func TestPremiumVideoWorkflowConstant(t *testing.T) {
	assert.Equal(t, "h3", premiumVideoWorkflow, "all enhanced-video commands run h3.json")
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
// disk. The h3.md base carries the MiniMax H3 role/craft; style files are pure
// overlays that begin with "STYLE OVERLAY:".
func TestEnhancedVideo_TemplatesLoad(t *testing.T) {
	for cmd, tpl := range enhancedVideoTemplates {
		t.Run(cmd, func(t *testing.T) {
			prompt, err := text.GetPrompt(tpl)
			require.NoError(t, err, "template %s for !%s must exist", tpl, cmd)
			require.NotEmpty(t, prompt)
			if cmd == "h3" {
				assert.Contains(t, prompt, "MiniMax H3", "h3.md base must reference MiniMax H3")
			} else {
				assert.True(t, strings.HasPrefix(prompt, "STYLE OVERLAY:"),
					"style overlay %s must start with 'STYLE OVERLAY:'", tpl)
			}
		})
	}
}

func TestLoadStyleSystemPrompt(t *testing.T) {
	// Base command returns h3.md alone (no overlay marker).
	base, err := loadStyleSystemPrompt("h3.md")
	require.NoError(t, err)
	assert.Contains(t, base, "MiniMax H3")
	assert.NotContains(t, base, "STYLE OVERLAY:")

	// Style command prepends the h3.md base craft before the overlay.
	combined, err := loadStyleSystemPrompt("simpsons.md")
	require.NoError(t, err)
	assert.Contains(t, combined, "MiniMax H3", "combined prompt must contain the base craft")
	assert.Contains(t, combined, "STYLE OVERLAY: The Simpsons", "combined prompt must contain the overlay")
	// Base must precede the overlay.
	assert.Less(t, strings.Index(combined, "MiniMax H3"), strings.Index(combined, "STYLE OVERLAY:"),
		"base craft must come before the style overlay")

	// Unknown template file errors.
	_, err = loadStyleSystemPrompt("does_not_exist.md")
	assert.Error(t, err)
}

func TestExtractQuotedDialog(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"no quotes", "a cat on a roof", nil},
		{"double-quoted dialog", `Homer says "D'oh!"`, []string{"D'oh!"}},
		{"multiple quotes", `"hello" then "goodbye"`, []string{"hello", "goodbye"}},
		{"single quotes not extracted (apostrophe)", "it's a test", nil},
		{"empty quotes ignored", `say "" please`, nil},
		{"mixed — only double extracted", `"yes" she said, it's fine`, []string{"yes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractQuotedDialog(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseChatroomFlag(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		wantEnabled   bool
		wantLocation  string
		wantRemaining string
	}{
		{"no flag", "a cat chasing a laser", false, "", "a cat chasing a laser"},
		{"flag only", "--chatroom", true, "", ""},
		{"flag with location", "--chatroom in the diner", true, "in the diner", ""},
		{"idea then flag", "a family argument --chatroom in the kitchen", true, "in the kitchen", "a family argument"},
		{"case-insensitive flag", "--CHATROOM in the bar", true, "in the bar", ""},
		{"flag preserves case of location", "--chatroom In The Diner", true, "In The Diner", ""},
		{"flag mid message", "hello --chatroom at moe's tavern bye", true, "at moe's tavern bye", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled, location, remaining := parseChatroomFlag(tt.message)
			assert.Equal(t, tt.wantEnabled, enabled)
			assert.Equal(t, tt.wantLocation, location)
			assert.Equal(t, tt.wantRemaining, remaining)
		})
	}
}

func TestFilterChatroomMessages(t *testing.T) {
	msgs := []birdbase.ChannelMessage{
		{Nickname: "alice", Message: "did you see that"},
		{Nickname: "dave", Message: "i am the bot"},  // bot nick dropped
		{Nickname: "spambot", Message: "buy crypto"}, // ignored nick dropped
		{Nickname: "bob", Message: "   "},            // blank dropped
		{Nickname: "carol", Message: "yeah it was wild"},
	}
	ignored := func(nick string) bool { return nick == "spambot" }
	got := filterChatroomMessages(msgs, "dave", ignored)
	// alice + carol survive (bob blank, dave bot, spambot ignored).
	require.Len(t, got, 2)
	assert.Equal(t, "alice", got[0].Nickname)
	assert.Equal(t, "carol", got[1].Nickname)

	// Bot nick match is case-insensitive.
	got2 := filterChatroomMessages(msgs, "DAVE", ignored)
	require.Len(t, got2, 2)

	// No ignored func (nil) — only bot + blank filtered.
	got3 := filterChatroomMessages(msgs, "dave", nil)
	require.Len(t, got3, 3) // alice, spambot, carol
}

func TestFilterChatroomMessages_CapsAtLimit(t *testing.T) {
	var msgs []birdbase.ChannelMessage
	for i := 0; i < chatroomMaxLines+5; i++ {
		msgs = append(msgs, birdbase.ChannelMessage{Nickname: "u", Message: fmt.Sprintf("msg %d", i)})
	}
	got := filterChatroomMessages(msgs, "", nil)
	assert.Len(t, got, chatroomMaxLines, "must trim to chatroomMaxLines")
	// Keeps the most recent (last) messages.
	assert.Contains(t, got[len(got)-1].Message, fmt.Sprintf("msg %d", chatroomMaxLines+4))
}

func TestBuildChatroomMessage(t *testing.T) {
	chatLog := []birdbase.ChannelMessage{
		{Nickname: "alice", Message: "did you see that"},
		{Nickname: "bob", Message: "yeah it was wild"},
	}

	t.Run("with location and extra", func(t *testing.T) {
		msg := buildChatroomMessage(chatLog, "in the diner", "make it tense")
		assert.Contains(t, msg, "<alice>: did you see that")
		assert.Contains(t, msg, "<bob>: yeah it was wild")
		assert.Contains(t, msg, "Setting: in the diner")
		assert.Contains(t, msg, "Additional direction: make it tense")
		assert.Contains(t, msg, "Convert each chatter")
	})

	t.Run("omits optional lines when empty", func(t *testing.T) {
		msg := buildChatroomMessage(chatLog, "", "")
		assert.NotContains(t, msg, "Setting:")
		assert.NotContains(t, msg, "Additional direction:")
		assert.Contains(t, msg, "<alice>: did you see that")
	})
}
