// Package users provides IRC user types with access levels and AI preferences.
package users

import (
	"testing"

	"aibird/birdbase"
	"aibird/irc/users/modes"
	"aibird/logger"

	"github.com/stretchr/testify/assert"
)

func init() {
	logger.Init(logger.Config{Level: logger.LevelError, Format: "text"})
}

func TestUser_CanSkipQueue(t *testing.T) {
	tests := []struct {
		name        string
		accessLevel int
		isAdmin     bool
		isOwner     bool
		want        bool
	}{
		{"access level 2", 2, false, false, true},
		{"access level 1", 1, false, false, false},
		{"access level 0", 0, false, false, false},
		{"access level 5", 5, false, false, true},
		{"admin", 0, true, false, true},
		{"owner", 0, false, true, true},
		{"regular user", 0, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{AccessLevel: tt.accessLevel, IsAdmin: tt.isAdmin, IsOwner: tt.isOwner}
			assert.Equal(t, tt.want, u.CanSkipQueue())
		})
	}
}

func TestUser_CanUsePremiumGPU(t *testing.T) {
	tests := []struct {
		name        string
		accessLevel int
		isAdmin     bool
		isOwner     bool
		want        bool
	}{
		{"access level 2", 2, false, false, true},
		{"access level 1", 1, false, false, false},
		{"admin", 0, true, false, true},
		{"owner", 0, false, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{AccessLevel: tt.accessLevel, IsAdmin: tt.isAdmin, IsOwner: tt.isOwner}
			assert.Equal(t, tt.want, u.CanUsePremiumGPU())
		})
	}
}

func TestUser_GetAccessLevelString(t *testing.T) {
	tests := []struct {
		level int
		want  string
	}{
		{0, "Free"},
		{1, "Chat Pal Status"},
		{2, "Swan Squadron"},
		{3, "Sparrow Society"},
		{4, "Golden Toucans"},
		{5, "Free Bird"},
		{99, "Free"},
	}

	for _, tt := range tests {
		t.Run(string(rune('0'+tt.level)), func(t *testing.T) {
			u := &User{AccessLevel: tt.level}
			assert.Equal(t, tt.want, u.GetAccessLevelString())
		})
	}
}

func TestUser_SettersAndGetters(t *testing.T) {
	u := &User{}

	u.SetBasePrompt("test prompt")
	assert.Equal(t, "test prompt", u.GetBasePrompt())

	u.SetAiService("llamacpp")
	assert.Equal(t, "llamacpp", u.GetAiService())

	u.SetPersonality("friendly")
	assert.Equal(t, "friendly", u.GetPersonality())

	u.SetAiModel("llama3")
	assert.Equal(t, "llama3", u.GetAiModel())
}

func TestUser_UpdateNick(t *testing.T) {
	u := &User{NickName: "old"}
	u.UpdateNick("new")
	assert.Equal(t, "new", u.NickName)
	assert.True(t, u.LatestActivity > 0)

	// Same nick should not update activity
	prevActivity := u.LatestActivity
	u.UpdateNick("new")
	assert.Equal(t, u.LatestActivity, prevActivity)
}

func TestUser_UpdateIdentHost(t *testing.T) {
	u := &User{Ident: "old", Host: "old.host"}
	u.UpdateIdentHost("new", "new.host")
	assert.Equal(t, "new", u.Ident)
	assert.Equal(t, "new.host", u.Host)

	// Same ident/host should not change
	u.UpdateIdentHost("new", "new.host")
	assert.Equal(t, "new", u.Ident)
}

func TestUser_Ignore(t *testing.T) {
	u := &User{}
	assert.False(t, u.IsIgnored())
	u.Ignore()
	assert.True(t, u.IsIgnored())
	u.UnIgnore()
	assert.False(t, u.IsIgnored())
}

func TestUser_HasModes(t *testing.T) {
	u := &User{
		CurrentModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"o"}},
		},
		PreservedModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"v"}},
		},
	}

	assert.True(t, u.HasCurrentModes("#test"))
	assert.False(t, u.HasCurrentModes("#other"))
	assert.True(t, u.HasPreservedModes("#test"))
	assert.False(t, u.HasPreservedModes("#other"))
	assert.True(t, u.HasAnyMode())

	u2 := &User{}
	assert.False(t, u2.HasAnyMode())
}

func TestUser_ToUserData(t *testing.T) {
	u := &User{
		NickName: "testuser", Ident: "test", Host: "test.host",
		AccessLevel: 2, IsAdmin: true, AiService: "llamacpp",
		CurrentModes: []modes.UserModes{{Channel: "#test", Modes: []string{"o"}}},
	}

	data := u.ToUserData(1)
	assert.Equal(t, "testuser", data.NickName)
	assert.Equal(t, 1, data.NetworkID)
	assert.Len(t, data.CurrentModes, 1)
}

func TestUser_FromUserData(t *testing.T) {
	data := &birdbase.UserData{
		NickName: "testuser", Ident: "test", Host: "test.host",
		AccessLevel: 3, AiService: "glm",
		CurrentModes: []birdbase.UserModeData{{Channel: "#chan", Modes: []string{"v"}}},
	}

	u := &User{}
	u.FromUserData(data)
	assert.Equal(t, "testuser", u.NickName)
	assert.Equal(t, 3, u.AccessLevel)
	assert.Len(t, u.CurrentModes, 1)
}

func TestNewUserFromData(t *testing.T) {
	data := &birdbase.UserData{NickName: "newuser", AccessLevel: 5}
	u := NewUserFromData(data)
	assert.Equal(t, "newuser", u.NickName)
	assert.Equal(t, 5, u.AccessLevel)
}

func TestUser_Seen_NoActivity(t *testing.T) {
	u := User{NickName: "ghost", FirstSeen: 1700000000, LatestActivity: 0}
	s := u.Seen()
	assert.Contains(t, s, "ghost")
	assert.Contains(t, s, "not seen any chats")
}

func TestUser_Seen_WithActivity(t *testing.T) {
	u := User{NickName: "alice", LatestActivity: 1700000000}
	s := u.Seen()
	assert.Contains(t, s, "alice")
	assert.Contains(t, s, "last seen")
}

func TestUser_String(t *testing.T) {
	u := User{NickName: "alice", Ident: "~alice", Host: "alice.host", AccessLevel: 3}
	s := u.String()
	assert.Contains(t, s, "alice")
	assert.NotEmpty(t, s)
}

func TestUser_IsAdminUser(t *testing.T) {
	assert.True(t, (&User{IsAdmin: true}).IsAdminUser())
	assert.False(t, (&User{IsAdmin: false}).IsAdminUser())
}

func TestUser_IsOwnerUser(t *testing.T) {
	assert.True(t, (&User{IsOwner: true}).IsOwnerUser())
	assert.False(t, (&User{IsOwner: false}).IsOwnerUser())
}

func TestUser_UnIgnore(t *testing.T) {
	u := User{Ignored: true}
	u.UnIgnore()
	assert.False(t, u.Ignored)
}

func TestUser_IsIgnored(t *testing.T) {
	assert.True(t, (&User{Ignored: true}).IsIgnored())
	assert.False(t, (&User{Ignored: false}).IsIgnored())
}

func TestUser_GetAccessLevel(t *testing.T) {
	assert.Equal(t, 5, (&User{AccessLevel: 5}).GetAccessLevel())
	assert.Equal(t, 0, (&User{}).GetAccessLevel())
}

func TestUser_GetDefaults(t *testing.T) {
	u := User{}
	assert.Equal(t, "", u.GetAiService())
	assert.Equal(t, "", u.GetAiModel())
	assert.Equal(t, "", u.GetBasePrompt())
	assert.Equal(t, "", u.GetPersonality())
}

func TestUser_Touch(t *testing.T) {
	u := User{NickName: "test"}
	before := u.LatestActivity
	u.Touch("hello")
	assert.True(t, u.LatestActivity >= before)
	assert.Equal(t, "hello", u.LatestChat)
}
