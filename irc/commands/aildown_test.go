package commands

import (
	"testing"

	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

// newTestState creates a minimal state for testing with a girc client that
// silently drops IRC messages (girc.New works without a connection).
func newTestStateAildown(action, message string, mcp settings.McpConfig) *state.State {
	client := girc.New(girc.Config{Server: "localhost", Nick: "testbot"})
	return &state.State{
		Client:  client,
		Event:   girc.Event{Source: &girc.Source{Name: "testuser", Ident: "test", Host: "host"}},
		Command: state.Command{Action: action, Message: message},
		Config:  &settings.Config{Mcp: mcp},
		User:    &users.User{},
	}
}

func TestShouldQueueAilDown(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		message string
		mcp     settings.McpConfig
		want    bool
	}{
		{
			name:    "valid aildown with MCP enabled",
			action:  "aildown",
			message: "What is Go?",
			mcp:     settings.McpConfig{Enabled: true},
			want:    true,
		},
		{
			name:    "wrong action",
			action:  "ai",
			message: "What is Go?",
			mcp:     settings.McpConfig{Enabled: true},
			want:    false,
		},
		{
			name:    "empty message",
			action:  "aildown",
			message: "",
			mcp:     settings.McpConfig{Enabled: true},
			want:    false,
		},
		{
			name:    "MCP disabled",
			action:  "aildown",
			message: "What is Go?",
			mcp:     settings.McpConfig{Enabled: false},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			irc := &state.State{
				Command: state.Command{Action: tt.action, Message: tt.message},
				Config:  &settings.Config{Mcp: tt.mcp},
			}
			got := ShouldQueueAilDown(irc)
			if got != tt.want {
				t.Errorf("ShouldQueueAilDown() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAilDown_MCPDisabled(t *testing.T) {
	irc := newTestStateAildown("aildown", "test", settings.McpConfig{Enabled: false})
	result := ParseAilDown(irc)
	if !result {
		t.Error("expected ParseAilDown to return true for disabled MCP")
	}
}

func TestParseAilDown_EmptyMessage(t *testing.T) {
	irc := newTestStateAildown("aildown", "", settings.McpConfig{Enabled: true})
	result := ParseAilDown(irc)
	if !result {
		t.Error("expected ParseAilDown to return true for empty message")
	}
}

func TestParseAilDown_SearXNGNotConfigured(t *testing.T) {
	irc := newTestStateAildown("aildown", "test", settings.McpConfig{
		Enabled: true,
		SearXNG: settings.SearXNGConfig{},
	})
	result := ParseAilDown(irc)
	if !result {
		t.Error("expected ParseAilDown to return true for missing SearXNG config")
	}
}

func TestParseAilDown_WrongAction(t *testing.T) {
	irc := newTestStateAildown("ai", "test", settings.McpConfig{Enabled: true})
	result := ParseAilDown(irc)
	if result {
		t.Error("expected ParseAilDown to return false for wrong action")
	}
}
