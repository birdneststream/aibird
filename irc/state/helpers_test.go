package state

import (
	"strings"
	"testing"

	"aibird/irc/channels"
	"aibird/irc/networks"
	"aibird/irc/users"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

// newStateWithArgs creates a State with pre-set arguments for testing.
// Uses a real girc.Client (not connected) so Send* methods don't panic.
func newStateWithArgs(args []Argument) State {
	client := girc.New(girc.Config{
		Server: "irc.example.com",
		Port:   6667,
		Name:   "test",
		Nick:   "testbot",
	})
	return State{
		Client:  client,
		Command: Command{Action: "test", Message: "test"},
		Network: &networks.Network{
			NetworkName: "testnet",
			Nick:        "testbot",
		},
		User: &users.User{
			NickName: "testuser",
			Ident:    "testid",
			Host:     "testhost",
		},
		Channel: &channels.Channel{
			Name: "#test",
		},
		Config:    &settings.Config{},
		Arguments: args,
		Event: girc.Event{
			Source: &girc.Source{Name: "testuser", Ident: "testid", Host: "testhost"},
			Params: []string{"#test"},
		},
	}
}

// TestUpdateBasedOnArgs_BoolField verifies that reflection updates a bool field on a User.
func TestUpdateBasedOnArgs_BoolField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "isAdmin", Value: true},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	immutableKeys := map[string]bool{
		"NickName": true, "Ident": true, "Host": true,
		"PreservedModes": true, "CurrentModes": true,
	}

	s.UpdateBasedOnArgs(user, immutableKeys)

	if !user.IsAdmin {
		t.Error("IsAdmin should be true after UpdateBasedOnArgs")
	}
}

// TestUpdateBasedOnArgs_IntField verifies that reflection updates an int field on a User.
func TestUpdateBasedOnArgs_IntField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "accessLevel", Value: 42},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	immutableKeys := map[string]bool{
		"NickName": true, "Ident": true, "Host": true,
		"PreservedModes": true, "CurrentModes": true,
	}

	s.UpdateBasedOnArgs(user, immutableKeys)

	if user.AccessLevel != 42 {
		t.Errorf("AccessLevel should be 42, got %d", user.AccessLevel)
	}
}

// TestUpdateBasedOnArgs_StringField verifies that reflection updates a string field on a User.
func TestUpdateBasedOnArgs_StringField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "aiService", Value: "glm"},
	})
	user := &users.User{NickName: "testuser", AiService: "llamacpp"}
	s.User = user

	immutableKeys := map[string]bool{
		"NickName": true, "Ident": true, "Host": true,
		"PreservedModes": true, "CurrentModes": true,
	}

	s.UpdateBasedOnArgs(user, immutableKeys)

	if user.AiService != "glm" {
		t.Errorf("AiService should be 'glm', got %q", user.AiService)
	}
}

// TestUpdateBasedOnArgs_ImmutableKeyBlocked verifies that immutable fields cannot be changed.
func TestUpdateBasedOnArgs_ImmutableKeyBlocked(t *testing.T) {
	tests := []struct {
		key      string
		value    interface{}
		field    string
		original string
	}{
		{"NickName", "hacker", "nickname", "testuser"},
		{"Ident", "newid", "ident", "testid"},
		{"Host", "newhost", "host", "testhost"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})
			user := &users.User{NickName: "testuser", Ident: "testid", Host: "testhost"}
			s.User = user

			immutableKeys := map[string]bool{
				"NickName": true, "Ident": true, "Host": true,
				"PreservedModes": true, "CurrentModes": true,
			}

			s.UpdateBasedOnArgs(user, immutableKeys)

			switch tt.field {
			case "nickname":
				if user.NickName != tt.original {
					t.Errorf("NickName should not change: got %q, want %q", user.NickName, tt.original)
				}
			case "ident":
				if user.Ident != tt.original {
					t.Errorf("Ident should not change: got %q, want %q", user.Ident, tt.original)
				}
			case "host":
				if user.Host != tt.original {
					t.Errorf("Host should not change: got %q, want %q", user.Host, tt.original)
				}
			}
		})
	}
}

// TestUpdateBasedOnArgs_MultipleFields verifies multiple fields updated in one call.
func TestUpdateBasedOnArgs_MultipleFields(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "isAdmin", Value: true},
		{Key: "accessLevel", Value: 10},
		{Key: "aiModel", Value: "gpt-4"},
		{Key: "ignored", Value: true},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	immutableKeys := map[string]bool{
		"NickName": true, "Ident": true, "Host": true,
		"PreservedModes": true, "CurrentModes": true,
	}

	s.UpdateBasedOnArgs(user, immutableKeys)

	if !user.IsAdmin {
		t.Error("IsAdmin should be true")
	}
	if user.AccessLevel != 10 {
		t.Errorf("AccessLevel should be 10, got %d", user.AccessLevel)
	}
	if user.AiModel != "gpt-4" {
		t.Errorf("AiModel should be 'gpt-4', got %q", user.AiModel)
	}
	if !user.Ignored {
		t.Error("Ignored should be true")
	}
}

// TestUpdateBasedOnArgs_ChannelBoolField verifies updating channel fields.
func TestUpdateBasedOnArgs_ChannelBoolField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "ai", Value: true},
		{Key: "sd", Value: true},
	})

	immutableKeys := map[string]bool{
		"Name": true, "Users": true, "ActivityTimer": true,
	}

	s.UpdateBasedOnArgs(s.Channel, immutableKeys)

	if !s.Channel.Ai {
		t.Error("Channel Ai should be true")
	}
	if !s.Channel.Sd {
		t.Error("Channel Sd should be true")
	}
}

// TestUpdateBasedOnArgs_NetworkField verifies updating network fields.
func TestUpdateBasedOnArgs_NetworkField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "preserveModes", Value: true},
		{Key: "burst", Value: 10},
	})

	immutableKeys := map[string]bool{
		"Name": true, "NetworkName": true, "Nick": true,
		"Users": true, "Channels": true, "Servers": true, "ModesAtOnce": true,
	}

	s.UpdateBasedOnArgs(s.Network, immutableKeys)

	if !s.Network.PreserveModes {
		t.Error("Network PreserveModes should be true")
	}
	if s.Network.Burst != 10 {
		t.Errorf("Network Burst should be 10, got %d", s.Network.Burst)
	}
}

// TestUpdateBasedOnArgs_NonexistentField verifies unknown fields are silently ignored.
func TestUpdateBasedOnArgs_NonexistentField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "nonexistent", Value: "value"},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	// Should not panic
	s.UpdateBasedOnArgs(user, map[string]bool{})
}

// TestUpdateUserBasedOnArgs_WrapperImmutable verifies the user wrapper blocks immutable fields.
func TestUpdateUserBasedOnArgs_WrapperImmutable(t *testing.T) {
	tests := []struct {
		name      string
		args      []Argument
		field     string
		wantValue interface{}
	}{
		{
			name:      "aiService updated",
			args:      []Argument{{Key: "aiService", Value: "glm"}},
			field:     "aiservice",
			wantValue: "glm",
		},
		{
			name:      "isAdmin updated",
			args:      []Argument{{Key: "isAdmin", Value: true}},
			field:     "isadmin",
			wantValue: true,
		},
		{
			name:      "NickName blocked",
			args:      []Argument{{Key: "NickName", Value: "newnick"}},
			field:     "nickname",
			wantValue: "testuser", // original
		},
		{
			name:      "Ident blocked",
			args:      []Argument{{Key: "Ident", Value: "newident"}},
			field:     "ident",
			wantValue: "testid",
		},
		{
			name:      "Host blocked",
			args:      []Argument{{Key: "Host", Value: "newhost"}},
			field:     "host",
			wantValue: "testhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStateWithArgs(tt.args)
			user := &users.User{
				NickName: "testuser",
				Ident:    "testid",
				Host:     "testhost",
			}
			s.User = user

			s.UpdateUserBasedOnArgs(user)

			switch strings.ToLower(tt.field) {
			case "aiservice":
				if user.AiService != tt.wantValue.(string) {
					t.Errorf("AiService = %q, want %q", user.AiService, tt.wantValue)
				}
			case "isadmin":
				if user.IsAdmin != tt.wantValue.(bool) {
					t.Errorf("IsAdmin = %v, want %v", user.IsAdmin, tt.wantValue)
				}
			case "nickname":
				if user.NickName != tt.wantValue.(string) {
					t.Errorf("NickName = %q (should be blocked)", user.NickName)
				}
			case "ident":
				if user.Ident != tt.wantValue.(string) {
					t.Errorf("Ident = %q (should be blocked)", user.Ident)
				}
			case "host":
				if user.Host != tt.wantValue.(string) {
					t.Errorf("Host = %q (should be blocked)", user.Host)
				}
			}
		})
	}
}

// TestGetActionTrigger_Default verifies default action trigger fallback.
func TestGetActionTrigger_Default(t *testing.T) {
	s := newStateWithArgs(nil)
	if trigger := s.GetActionTrigger(); trigger != "!" {
		t.Errorf("Default trigger should be '!', got %q", trigger)
	}
}

// TestGetActionTrigger_ChannelOverride verifies channel-level action trigger.
func TestGetActionTrigger_ChannelOverride(t *testing.T) {
	s := newStateWithArgs(nil)
	s.Channel.ActionTrigger = "~"
	if trigger := s.GetActionTrigger(); trigger != "~" {
		t.Errorf("Channel override should be '~', got %q", trigger)
	}
}

// TestGetActionTrigger_NetworkOverride verifies network-level action trigger.
func TestGetActionTrigger_NetworkOverride(t *testing.T) {
	s := newStateWithArgs(nil)
	s.Network.ActionTrigger = "@"
	if trigger := s.GetActionTrigger(); trigger != "@" {
		t.Errorf("Network override should be '@', got %q", trigger)
	}
}

// TestGetActionTrigger_ChannelTakesPrecedence verifies channel trigger wins over network.
func TestGetActionTrigger_ChannelTakesPrecedence(t *testing.T) {
	s := newStateWithArgs(nil)
	s.Channel.ActionTrigger = "~"
	s.Network.ActionTrigger = "@"
	if trigger := s.GetActionTrigger(); trigger != "~" {
		t.Errorf("Channel should take precedence, got %q", trigger)
	}
}

// TestGetActionTrigger_ResolveOrder verifies the full resolution order via table-driven test.
func TestGetActionTrigger_ResolveOrder(t *testing.T) {
	tests := []struct {
		name           string
		channelTrigger string
		networkTrigger string
		expected       string
	}{
		{"both empty", "", "", "!"},
		{"channel only", "~", "", "~"},
		{"network only", "", "@", "@"},
		{"channel wins", "~", "@", "~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStateWithArgs(nil)
			s.Channel.ActionTrigger = tt.channelTrigger
			s.Network.ActionTrigger = tt.networkTrigger
			if trigger := s.GetActionTrigger(); trigger != tt.expected {
				t.Errorf("GetActionTrigger() = %q, want %q", trigger, tt.expected)
			}
		})
	}
}

// TestParseArguments_EndToEnd verifies the full parse → argument extraction pipeline
// including message reconstruction after argument extraction.
func TestParseArguments_EndToEnd(t *testing.T) {
	tests := []struct {
		name            string
		message         string
		expectedArgs    []Argument
		expectedMessage string
	}{
		{
			name:            "no args",
			message:         "hello world",
			expectedArgs:    nil,
			expectedMessage: "hello world",
		},
		{
			name:            "bool flag stripped from message",
			message:         "hello --isAdmin",
			expectedArgs:    []Argument{{Key: "isAdmin", Value: true}},
			expectedMessage: "hello",
		},
		{
			name:            "string value stripped from message",
			message:         "hello --aiService=glm",
			expectedArgs:    []Argument{{Key: "aiService", Value: "glm"}},
			expectedMessage: "hello",
		},
		{
			name:            "quoted multi-word value",
			message:         `hello --aiBasePrompt="you are helpful"`,
			expectedArgs:    []Argument{{Key: "aiBasePrompt", Value: "you are helpful"}},
			expectedMessage: "hello",
		},
		{
			name:            "single-quoted value",
			message:         `hello --aiModel='llama3'`,
			expectedArgs:    []Argument{{Key: "aiModel", Value: "llama3"}},
			expectedMessage: "hello",
		},
		{
			name:    "multiple args stripped",
			message: "prompt text --seed=42 --verbose --width=512",
			expectedArgs: []Argument{
				{Key: "seed", Value: "42"},
				{Key: "verbose", Value: true},
				{Key: "width", Value: "512"},
			},
			expectedMessage: "prompt text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newState("test", tt.message)
			s.ParseArguments()

			// Verify message reconstruction
			if s.Message() != tt.expectedMessage {
				t.Errorf("Message after parse = %q, want %q", s.Message(), tt.expectedMessage)
			}

			if tt.expectedArgs == nil {
				if len(s.Arguments) != 0 {
					t.Errorf("Expected no arguments, got %d", len(s.Arguments))
				}
				return
			}

			if len(s.Arguments) != len(tt.expectedArgs) {
				t.Fatalf("Expected %d arguments, got %d", len(tt.expectedArgs), len(s.Arguments))
			}

			for i, expected := range tt.expectedArgs {
				if s.Arguments[i].Key != expected.Key {
					t.Errorf("Arg %d key: got %q, want %q", i, s.Arguments[i].Key, expected.Key)
				}
				if s.Arguments[i].Value != expected.Value {
					t.Errorf("Arg %d value: got %v, want %v", i, s.Arguments[i].Value, expected.Value)
				}
			}
		})
	}
}
