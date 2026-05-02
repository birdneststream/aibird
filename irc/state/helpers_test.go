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

// --- User update tests ---

// TestUpdateUserBasedOnArgs_BoolField verifies that explicit setters update a bool field on a User.
func TestUpdateUserBasedOnArgs_BoolField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "isAdmin", Value: true},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	s.UpdateUserBasedOnArgs(user)

	if !user.IsAdmin {
		t.Error("IsAdmin should be true after UpdateUserBasedOnArgs")
	}
}

// TestUpdateUserBasedOnArgs_IntField verifies that explicit setters update an int field on a User.
func TestUpdateUserBasedOnArgs_IntField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "accessLevel", Value: 42},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	s.UpdateUserBasedOnArgs(user)

	if user.AccessLevel != 42 {
		t.Errorf("AccessLevel should be 42, got %d", user.AccessLevel)
	}
}

// TestUpdateUserBasedOnArgs_StringField verifies that explicit setters update a string field on a User.
func TestUpdateUserBasedOnArgs_StringField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "aiService", Value: "glm"},
	})
	user := &users.User{NickName: "testuser", AiService: "llamacpp"}
	s.User = user

	s.UpdateUserBasedOnArgs(user)

	if user.AiService != "glm" {
		t.Errorf("AiService should be 'glm', got %q", user.AiService)
	}
}

// TestUpdateUserBasedOnArgs_ImmutableKeyBlocked verifies that immutable fields cannot be changed.
func TestUpdateUserBasedOnArgs_ImmutableKeyBlocked(t *testing.T) {
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

			s.UpdateUserBasedOnArgs(user)

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

// TestUpdateUserBasedOnArgs_MultipleFields verifies multiple fields updated in one call.
func TestUpdateUserBasedOnArgs_MultipleFields(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "isAdmin", Value: true},
		{Key: "accessLevel", Value: 10},
		{Key: "aiModel", Value: "gpt-4"},
		{Key: "ignored", Value: true},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	s.UpdateUserBasedOnArgs(user)

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

// TestUpdateUserBasedOnArgs_AllStringFields verifies all user string fields are writable.
func TestUpdateUserBasedOnArgs_AllStringFields(t *testing.T) {
	tests := []struct {
		key       string
		value     string
		fieldName string
		gotFunc   func(*users.User) string
	}{
		{"aiService", "glm", "AiService", func(u *users.User) string { return u.AiService }},
		{"aiModel", "gpt-4", "AiModel", func(u *users.User) string { return u.AiModel }},
		{"aiBasePrompt", "be helpful", "AiBasePrompt", func(u *users.User) string { return u.AiBasePrompt }},
		{"aiPersonality", "friendly", "AiPersonality", func(u *users.User) string { return u.AiPersonality }},
		{"latestChat", "hello", "LatestChat", func(u *users.User) string { return u.LatestChat }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})
			user := &users.User{NickName: "testuser"}
			s.User = user

			s.UpdateUserBasedOnArgs(user)

			if got := tt.gotFunc(user); got != tt.value {
				t.Errorf("%s = %q, want %q", tt.fieldName, got, tt.value)
			}
		})
	}
}

// TestUpdateUserBasedOnArgs_AllBoolFields verifies all user bool fields are writable.
func TestUpdateUserBasedOnArgs_AllBoolFields(t *testing.T) {
	tests := []struct {
		key       string
		fieldName string
		gotFunc   func(*users.User) bool
	}{
		{"isAdmin", "IsAdmin", func(u *users.User) bool { return u.IsAdmin }},
		{"isOwner", "IsOwner", func(u *users.User) bool { return u.IsOwner }},
		{"ignored", "Ignored", func(u *users.User) bool { return u.Ignored }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: true}})
			user := &users.User{NickName: "testuser"}
			s.User = user

			s.UpdateUserBasedOnArgs(user)

			if got := tt.gotFunc(user); !got {
				t.Errorf("%s should be true", tt.fieldName)
			}
		})
	}
}

// TestUpdateUserBasedOnArgs_AllIntFields verifies all user int/int64 fields are writable.
func TestUpdateUserBasedOnArgs_AllIntFields(t *testing.T) {
	tests := []struct {
		key       string
		value     interface{}
		fieldName string
		gotInt    func(*users.User) int
		gotInt64  func(*users.User) int64
	}{
		{"accessLevel", 42, "AccessLevel", func(u *users.User) int { return u.AccessLevel }, nil},
		{"firstSeen", int64(1000), "FirstSeen", nil, func(u *users.User) int64 { return u.FirstSeen }},
		{"latestActivity", int64(2000), "LatestActivity", nil, func(u *users.User) int64 { return u.LatestActivity }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})
			user := &users.User{NickName: "testuser"}
			s.User = user

			s.UpdateUserBasedOnArgs(user)

			if tt.gotInt != nil {
				if got := tt.gotInt(user); got != tt.value.(int) {
					t.Errorf("%s = %d, want %d", tt.fieldName, got, tt.value.(int))
				}
			}
			if tt.gotInt64 != nil {
				if got := tt.gotInt64(user); got != tt.value.(int64) {
					t.Errorf("%s = %d, want %d", tt.fieldName, got, tt.value.(int64))
				}
			}
		})
	}
}

// TestUpdateUserBasedOnArgs_NonexistentField verifies unknown fields produce a warning.
func TestUpdateUserBasedOnArgs_NonexistentField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "nonexistent", Value: "value"},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	// Should not panic
	s.UpdateUserBasedOnArgs(user)
}

// TestUpdateUserBasedOnArgs_TypeCoercion verifies that string values are coerced to correct types.
func TestUpdateUserBasedOnArgs_TypeCoercion(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "isAdmin", Value: "true"},
		{Key: "accessLevel", Value: "15"},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	s.UpdateUserBasedOnArgs(user)

	if !user.IsAdmin {
		t.Error("IsAdmin should be true when set from string 'true'")
	}
	if user.AccessLevel != 15 {
		t.Errorf("AccessLevel should be 15 when set from string '15', got %d", user.AccessLevel)
	}
}

// TestUpdateUserBasedOnArgs_InvalidCoercion verifies that invalid values don't corrupt fields.
func TestUpdateUserBasedOnArgs_InvalidCoercion(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		value         interface{}
		fieldName     string
		originalValue interface{}
	}{
		{"invalid bool", "isAdmin", "notabool", "IsAdmin", false},
		{"invalid int", "accessLevel", "abc", "AccessLevel", 0},
		{"invalid int64", "firstSeen", "xyz", "FirstSeen", int64(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})
			user := &users.User{NickName: "testuser"}
			s.User = user

			s.UpdateUserBasedOnArgs(user)

			// Field should remain at zero value (not corrupted)
			switch strings.ToLower(tt.fieldName) {
			case "isadmin":
				if user.IsAdmin != tt.originalValue.(bool) {
					t.Errorf("IsAdmin should remain %v on invalid input, got %v", tt.originalValue, user.IsAdmin)
				}
			case "accesslevel":
				if user.AccessLevel != tt.originalValue.(int) {
					t.Errorf("AccessLevel should remain %v on invalid input, got %d", tt.originalValue, user.AccessLevel)
				}
			case "firstseen":
				if user.FirstSeen != tt.originalValue.(int64) {
					t.Errorf("FirstSeen should remain %v on invalid input, got %d", tt.originalValue, user.FirstSeen)
				}
			}
		})
	}
}

// TestUpdateUserBasedOnArgs_GircUserImmutable verifies that GircUser is explicitly blocked.
func TestUpdateUserBasedOnArgs_GircUserImmutable(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "GircUser", Value: "something"},
	})
	user := &users.User{NickName: "testuser"}
	s.User = user

	s.UpdateUserBasedOnArgs(user)

	// GircUser should remain nil (blocked as immutable)
	if user.GircUser != nil {
		t.Error("GircUser should be blocked as immutable")
	}
}

// TestUpdateUserBasedOnArgs_CaseInsensitive verifies that field names are case-insensitive.
func TestUpdateUserBasedOnArgs_CaseInsensitive(t *testing.T) {
	tests := []struct {
		key   string
		value interface{}
	}{
		{"isAdmin", true},
		{"ISADMIN", true},
		{"isadmin", true},
		{"IsAdmin", true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})
			user := &users.User{NickName: "testuser"}
			s.User = user

			s.UpdateUserBasedOnArgs(user)

			if !user.IsAdmin {
				t.Errorf("IsAdmin should be true for key %q", tt.key)
			}
		})
	}
}

// --- Channel update tests ---

// TestUpdateChannelBasedOnArgs_BoolFields verifies updating channel bool fields.
func TestUpdateChannelBasedOnArgs_BoolFields(t *testing.T) {
	tests := []struct {
		key       string
		fieldName string
		gotFunc   func(*channels.Channel) bool
	}{
		{"ai", "Ai", func(c *channels.Channel) bool { return c.Ai }},
		{"sd", "Sd", func(c *channels.Channel) bool { return c.Sd }},
		{"imageDescribe", "ImageDescribe", func(c *channels.Channel) bool { return c.ImageDescribe }},
		{"sound", "Sound", func(c *channels.Channel) bool { return c.Sound }},
		{"video", "Video", func(c *channels.Channel) bool { return c.Video }},
		{"trimOutput", "TrimOutput", func(c *channels.Channel) bool { return c.TrimOutput }},
		{"preserveModes", "PreserveModes", func(c *channels.Channel) bool { return c.PreserveModes }},
		{"chatMode", "ChatMode", func(c *channels.Channel) bool { return c.ChatMode }},
		{"companionMode", "CompanionMode", func(c *channels.Channel) bool { return c.CompanionMode }},
		{"sendArtUrl", "SendArtURL", func(c *channels.Channel) bool { return c.SendArtURL }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: true}})

			s.UpdateChannelBasedOnArgs()

			if got := tt.gotFunc(s.Channel); !got {
				t.Errorf("Channel %s should be true", tt.fieldName)
			}
		})
	}
}

// TestUpdateChannelBasedOnArgs_StringFields verifies updating channel string fields.
func TestUpdateChannelBasedOnArgs_StringFields(t *testing.T) {
	tests := []struct {
		key       string
		value     string
		fieldName string
		gotFunc   func(*channels.Channel) string
	}{
		{"actionTrigger", "~", "ActionTrigger", func(c *channels.Channel) string { return c.ActionTrigger }},
		{"chatPersonality", "friendly", "ChatPersonality", func(c *channels.Channel) string { return c.ChatPersonality }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})

			s.UpdateChannelBasedOnArgs()

			if got := tt.gotFunc(s.Channel); got != tt.value {
				t.Errorf("Channel %s = %q, want %q", tt.fieldName, got, tt.value)
			}
		})
	}
}

// TestUpdateChannelBasedOnArgs_FloatField verifies updating channel float64 fields.
func TestUpdateChannelBasedOnArgs_FloatField(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "chatResponseRate", Value: 0.75},
	})

	s.UpdateChannelBasedOnArgs()

	if s.Channel.ChatResponseRate != 0.75 {
		t.Errorf("Channel ChatResponseRate = %f, want 0.75", s.Channel.ChatResponseRate)
	}
}

// TestUpdateChannelBasedOnArgs_FloatFieldFromString verifies string-to-float conversion.
func TestUpdateChannelBasedOnArgs_FloatFieldFromString(t *testing.T) {
	s := newStateWithArgs([]Argument{
		{Key: "chatResponseRate", Value: "0.5"},
	})

	s.UpdateChannelBasedOnArgs()

	if s.Channel.ChatResponseRate != 0.5 {
		t.Errorf("Channel ChatResponseRate = %f, want 0.5", s.Channel.ChatResponseRate)
	}
}

// TestUpdateChannelBasedOnArgs_ImmutableField verifies immutable channel fields are blocked.
func TestUpdateChannelBasedOnArgs_ImmutableField(t *testing.T) {
	tests := []struct {
		key       string
		value     interface{}
		checkFunc func(*channels.Channel) bool // returns true if the field was NOT changed
	}{
		{"Name", "newname", func(c *channels.Channel) bool { return c.Name == "#test" }},
		{"Users", "something", func(c *channels.Channel) bool { return c.Users == nil }},
		{"denyCommands", "something", func(c *channels.Channel) bool { return c.DenyCommands == nil }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})

			s.UpdateChannelBasedOnArgs()

			if !tt.checkFunc(s.Channel) {
				t.Errorf("Channel %s should not be changed", tt.key)
			}
		})
	}
}

// --- Network update tests ---

// TestUpdateNetworkBasedOnArgs_BoolFields verifies updating network bool fields.
func TestUpdateNetworkBasedOnArgs_BoolFields(t *testing.T) {
	tests := []struct {
		key       string
		fieldName string
		gotFunc   func(*networks.Network) bool
	}{
		{"enabled", "Enabled", func(n *networks.Network) bool { return n.Enabled }},
		{"preserveModes", "PreserveModes", func(n *networks.Network) bool { return n.PreserveModes }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: true}})

			s.UpdateNetworkBasedOnArgs()

			if got := tt.gotFunc(s.Network); !got {
				t.Errorf("Network %s should be true", tt.fieldName)
			}
		})
	}
}

// TestUpdateNetworkBasedOnArgs_IntFields verifies updating network int fields.
func TestUpdateNetworkBasedOnArgs_IntFields(t *testing.T) {
	tests := []struct {
		key       string
		value     int
		fieldName string
		gotFunc   func(*networks.Network) int
	}{
		{"pingDelay", 120, "PingDelay", func(n *networks.Network) int { return n.PingDelay }},
		{"throttle", 5, "Throttle", func(n *networks.Network) int { return n.Throttle }},
		{"burst", 10, "Burst", func(n *networks.Network) int { return n.Burst }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})

			s.UpdateNetworkBasedOnArgs()

			if got := tt.gotFunc(s.Network); got != tt.value {
				t.Errorf("Network %s = %d, want %d", tt.fieldName, got, tt.value)
			}
		})
	}
}

// TestUpdateNetworkBasedOnArgs_StringFields verifies updating network string fields.
func TestUpdateNetworkBasedOnArgs_StringFields(t *testing.T) {
	tests := []struct {
		key       string
		value     string
		fieldName string
		gotFunc   func(*networks.Network) string
	}{
		{"version", "v2.0", "Version", func(n *networks.Network) string { return n.Version }},
		{"actionTrigger", "@", "ActionTrigger", func(n *networks.Network) string { return n.ActionTrigger }},
		{"user", "newuser", "User", func(n *networks.Network) string { return n.User }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})

			s.UpdateNetworkBasedOnArgs()

			if got := tt.gotFunc(s.Network); got != tt.value {
				t.Errorf("Network %s = %q, want %q", tt.fieldName, got, tt.value)
			}
		})
	}
}

// TestUpdateNetworkBasedOnArgs_SensitiveFieldsBlocked verifies that sensitive fields are explicitly blocked.
func TestUpdateNetworkBasedOnArgs_SensitiveFieldsBlocked(t *testing.T) {
	tests := []struct {
		key       string
		value     interface{}
		checkFunc func(*networks.Network) bool
	}{
		{"Pass", "hacked", func(n *networks.Network) bool { return n.Pass == "" }},
		{"NickServPass", "hacked", func(n *networks.Network) bool { return n.NickServPass == "" }},
		{"ConnectedAt", "something", func(n *networks.Network) bool { return n.ConnectedAt.IsZero() }},
		{"ignoredNicks", "something", func(n *networks.Network) bool { return n.IgnoredNicks == nil }},
		{"adminHosts", "something", func(n *networks.Network) bool { return n.AdminHosts == nil }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})

			s.UpdateNetworkBasedOnArgs()

			if !tt.checkFunc(s.Network) {
				t.Errorf("Network %s should be blocked as immutable", tt.key)
			}
		})
	}
}

// TestUpdateNetworkBasedOnArgs_ImmutableField verifies immutable network fields are blocked.
func TestUpdateNetworkBasedOnArgs_ImmutableField(t *testing.T) {
	tests := []struct {
		key       string
		value     interface{}
		checkFunc func(*networks.Network) bool
	}{
		{"Name", "newname", func(n *networks.Network) bool { return n.Name == "" }},
		{"NetworkName", "newnet", func(n *networks.Network) bool { return n.NetworkName == "testnet" }},
		{"Nick", "newnick", func(n *networks.Network) bool { return n.Nick == "testbot" }},
		{"Users", "something", func(n *networks.Network) bool { return n.Users == nil }},
		{"Channels", "something", func(n *networks.Network) bool { return n.Channels == nil }},
		{"Servers", "something", func(n *networks.Network) bool { return n.Servers == nil }},
		{"ModesAtOnce", "something", func(n *networks.Network) bool { return n.ModesAtOnce == 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			s := newStateWithArgs([]Argument{{Key: tt.key, Value: tt.value}})

			s.UpdateNetworkBasedOnArgs()

			if !tt.checkFunc(s.Network) {
				t.Errorf("Network %s should not be changed", tt.key)
			}
		})
	}
}

// --- Integration: wrapper functions with immutable fields ---

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

// --- GetActionTrigger tests ---

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

// --- ParseArguments end-to-end tests ---

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
