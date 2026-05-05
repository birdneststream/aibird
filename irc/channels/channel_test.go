package channels

import (
	"testing"
	"time"

	"aibird/irc/users"
	"aibird/irc/users/modes"
	"aibird/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
}

// TestForgetMode_RemovesOnlyTargetMode verifies that ForgetMode removes exactly
// the requested mode from the middle of a multi-mode slice without skipping
// the element that follows. This is a regression test for a missing break
// after append-based slice removal.
func TestForgetMode_RemovesOnlyTargetMode(t *testing.T) {
	ch := &Channel{Name: "#test", PreserveModes: true}

	tests := []struct {
		name           string
		currentModes   []modes.UserModes
		preservedModes []modes.UserModes
		removeMode     string
		isAdmin        bool
		wantCurrent    []string // expected current modes for #test after removal
		wantPreserved  []string // expected preserved modes for #test after removal
	}{
		{
			name: "remove middle mode from 3-mode current slice",
			currentModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v", "h"}},
			},
			preservedModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v", "h"}},
			},
			removeMode:    "v",
			isAdmin:       false,
			wantCurrent:   []string{"o", "h"},
			wantPreserved: []string{"o", "h"},
		},
		{
			name: "remove first mode from current slice",
			currentModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			preservedModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			removeMode:    "o",
			isAdmin:       false,
			wantCurrent:   []string{"v"},
			wantPreserved: []string{"v"},
		},
		{
			name: "remove last mode from current slice",
			currentModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			preservedModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			removeMode:    "v",
			isAdmin:       false,
			wantCurrent:   []string{"o"},
			wantPreserved: []string{"o"},
		},
		{
			name: "remove mode not present — no change",
			currentModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			preservedModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			removeMode:    "q",
			isAdmin:       false,
			wantCurrent:   []string{"o", "v"},
			wantPreserved: []string{"o", "v"},
		},
		{
			name: "admin user — preserved modes untouched",
			currentModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v", "h"}},
			},
			preservedModes: []modes.UserModes{
				{Channel: "#test", Modes: []string{"o", "v", "h"}},
			},
			removeMode:    "v",
			isAdmin:       true,
			wantCurrent:   []string{"o", "h"},
			wantPreserved: []string{"o", "v", "h"}, // unchanged for admin
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &users.User{
				NickName:       "testnick",
				Ident:          "testident",
				Host:           "testhost",
				IsAdmin:        tt.isAdmin,
				CurrentModes:   make([]modes.UserModes, len(tt.currentModes)),
				PreservedModes: make([]modes.UserModes, len(tt.preservedModes)),
			}
			copy(user.CurrentModes, tt.currentModes)
			copy(user.PreservedModes, tt.preservedModes)

			ch.ForgetMode(user, tt.removeMode)

			// Check current modes
			gotCurrent := modesForChannel(user.CurrentModes, "#test")
			if !equalModes(gotCurrent, tt.wantCurrent) {
				t.Errorf("current modes: got %v, want %v", gotCurrent, tt.wantCurrent)
			}

			// Check preserved modes
			gotPreserved := modesForChannel(user.PreservedModes, "#test")
			if !equalModes(gotPreserved, tt.wantPreserved) {
				t.Errorf("preserved modes: got %v, want %v", gotPreserved, tt.wantPreserved)
			}
		})
	}
}

// modesForChannel returns the mode list for a given channel, or nil if not found.
func modesForChannel(userModes []modes.UserModes, channel string) []string {
	for _, um := range userModes {
		if um.Channel == channel {
			return um.Modes
		}
	}
	return nil
}

// equalModes compares two string slices for equality regardless of order.
func equalModes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	// Since modes are small (typically 1-3), linear scan is fine
	for _, m := range a {
		found := false
		for _, n := range b {
			if m == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// TestGetUserWithNick_NilChannel verifies that calling GetUserWithNick on a nil channel returns an error.
func TestGetUserWithNick_NilChannel(t *testing.T) {
	var ch *Channel
	user, err := ch.GetUserWithNick("testnick")
	require.Error(t, err)
	assert.Nil(t, user)
	assert.Contains(t, err.Error(), "nil *Channel")
}

// TestGetUserWithNick_NoUsers verifies that GetUserWithNick returns nil when no users match.
func TestGetUserWithNick_NoUsers(t *testing.T) {
	ch := &Channel{Name: "#test", Users: []*users.User{}}
	user, err := ch.GetUserWithNick("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, user)
}

// TestGetUserWithNick_SingleMatch verifies that GetUserWithNick returns the matching user.
func TestGetUserWithNick_SingleMatch(t *testing.T) {
	ch := &Channel{
		Name: "#test",
		Users: []*users.User{
			{NickName: "alice", Ident: "alice", Host: "example.com"},
		},
	}
	user, err := ch.GetUserWithNick("alice")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "alice", user.NickName)
}

// TestGetUserWithNick_MultipleUsers_Tiebreaker verifies that GetUserWithNick returns the user with the latest activity when multiple match.
func TestGetUserWithNick_MultipleUsers_Tiebreaker(t *testing.T) {
	now := time.Now().Unix()
	ch := &Channel{
		Name: "#test",
		Users: []*users.User{
			{NickName: "alice", Ident: "alice1", Host: "example.com", LatestActivity: now - 3600},
			{NickName: "alice", Ident: "alice2", Host: "example.com", LatestActivity: now},
			{NickName: "alice", Ident: "alice3", Host: "example.com", LatestActivity: now - 7200},
		},
	}
	user, err := ch.GetUserWithNick("alice")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "alice2", user.Ident, "expected user with latest activity")
}

// TestGetUserWithNick_MultipleUsers_EqualActivity verifies first-match behavior when LatestActivity is equal.
func TestGetUserWithNick_MultipleUsers_EqualActivity(t *testing.T) {
	now := time.Now().Unix()
	ch := &Channel{
		Name: "#test",
		Users: []*users.User{
			{NickName: "alice", Ident: "alice1", Host: "example.com", LatestActivity: now},
			{NickName: "alice", Ident: "alice2", Host: "example.com", LatestActivity: now},
		},
	}
	user, err := ch.GetUserWithNick("alice")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "alice1", user.Ident, "expected first user when activity is equal")
}

// TestSyncUser_NilChannel verifies that SyncUser returns false on a nil channel.
func TestSyncUser_NilChannel(t *testing.T) {
	var ch *Channel
	user := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	assert.False(t, ch.SyncUser(user))
}

// TestSyncUser_ExistingUser verifies that SyncUser returns false when the user already exists.
func TestSyncUser_ExistingUser(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	ch.Users = []*users.User{user}

	// Same nick, ident, and host — should not add again
	assert.False(t, ch.SyncUser(&users.User{NickName: "alice", Ident: "alice", Host: "example.com"}))
	assert.Len(t, ch.Users, 1)
}

// TestSyncUser_NewUser verifies that SyncUser appends a new user and returns true.
func TestSyncUser_NewUser(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user1 := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	user2 := &users.User{NickName: "bob", Ident: "bob", Host: "example.com"}

	assert.True(t, ch.SyncUser(user1))
	assert.Len(t, ch.Users, 1)
	assert.True(t, ch.SyncUser(user2))
	assert.Len(t, ch.Users, 2)
}

// TestSyncUser_SameNickDifferentHost verifies that users with the same nick but different ident/host are treated as distinct.
func TestSyncUser_SameNickDifferentHost(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user1 := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	user2 := &users.User{NickName: "alice", Ident: "alice", Host: "other.com"}

	assert.True(t, ch.SyncUser(user1))
	assert.True(t, ch.SyncUser(user2))
	assert.Len(t, ch.Users, 2)
}

// TestSyncCurrentModes_NilUser verifies early return when user is nil.
func TestSyncCurrentModes_NilUser(t *testing.T) {
	ch := &Channel{Name: "#test"}
	// Should not panic
	ch.SyncCurrentModes(nil, []string{"o"})
}

// TestSyncCurrentModes_NilChannel verifies early return when channel is nil.
func TestSyncCurrentModes_NilChannel(t *testing.T) {
	var ch *Channel
	user := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	// Should not panic
	ch.SyncCurrentModes(user, []string{"o"})
}

// TestSyncCurrentModes_UpdateExisting verifies updating existing channel modes.
func TestSyncCurrentModes_UpdateExisting(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName: "alice",
		CurrentModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"o", "v"}},
			{Channel: "#other", Modes: []string{"h"}},
		},
	}

	ch.SyncCurrentModes(user, []string{"o", "v", "h"})

	got := modesForChannel(user.CurrentModes, "#test")
	assert.Equal(t, []string{"o", "v", "h"}, got)
	// Other channel should be untouched
	assert.Equal(t, []string{"h"}, modesForChannel(user.CurrentModes, "#other"))
}

// TestSyncCurrentModes_AppendNew verifies appending a new channel entry.
func TestSyncCurrentModes_AppendNew(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName:     "alice",
		CurrentModes: []modes.UserModes{},
	}

	ch.SyncCurrentModes(user, []string{"o", "v"})

	assert.Len(t, user.CurrentModes, 1)
	assert.Equal(t, "#test", user.CurrentModes[0].Channel)
	assert.Equal(t, []string{"o", "v"}, user.CurrentModes[0].Modes)
}

// TestSyncPreservedModes_NilUser verifies early return when user is nil.
func TestSyncPreservedModes_NilUser(t *testing.T) {
	ch := &Channel{Name: "#test"}
	ch.SyncPreservedModes(nil, []string{"o"})
}

// TestSyncPreservedModes_NilChannel verifies early return when channel is nil.
func TestSyncPreservedModes_NilChannel(t *testing.T) {
	var ch *Channel
	user := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	ch.SyncPreservedModes(user, []string{"o"})
}

// TestSyncPreservedModes_UpdateExisting verifies updating existing preserved modes.
func TestSyncPreservedModes_UpdateExisting(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName: "alice",
		PreservedModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"v"}},
		},
	}

	ch.SyncPreservedModes(user, []string{"o", "v"})

	assert.Equal(t, []string{"o", "v"}, modesForChannel(user.PreservedModes, "#test"))
}

// TestSyncPreservedModes_AppendNew verifies appending a new channel entry.
func TestSyncPreservedModes_AppendNew(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName:       "alice",
		PreservedModes: []modes.UserModes{},
	}

	ch.SyncPreservedModes(user, []string{"o"})

	assert.Len(t, user.PreservedModes, 1)
	assert.Equal(t, "#test", user.PreservedModes[0].Channel)
}

// TestSyncMode_NilUser verifies early return when user is nil.
func TestSyncMode_NilUser(t *testing.T) {
	ch := &Channel{Name: "#test"}
	ch.SyncMode(nil, "o")
}

// TestSyncMode_NilChannel verifies early return when channel is nil.
func TestSyncMode_NilChannel(t *testing.T) {
	var ch *Channel
	user := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	ch.SyncMode(user, "o")
}

// TestSyncMode_AddToEmpty verifies adding a mode to empty mode lists.
func TestSyncMode_AddToEmpty(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName:       "alice",
		CurrentModes:   []modes.UserModes{},
		PreservedModes: []modes.UserModes{},
	}

	ch.SyncMode(user, "o")

	assert.Equal(t, []string{"o"}, modesForChannel(user.CurrentModes, "#test"))
	assert.Equal(t, []string{"o"}, modesForChannel(user.PreservedModes, "#test"))
}

// TestSyncMode_DuplicateMode verifies that adding a duplicate mode is a no-op.
func TestSyncMode_DuplicateMode(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName: "alice",
		CurrentModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"o", "v"}},
		},
		PreservedModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"o", "v"}},
		},
	}

	ch.SyncMode(user, "o")

	assert.Len(t, modesForChannel(user.CurrentModes, "#test"), 2)
	assert.Len(t, modesForChannel(user.PreservedModes, "#test"), 2)
}

// TestSyncMode_AddToExisting verifies adding a new mode to an existing channel entry.
func TestSyncMode_AddToExisting(t *testing.T) {
	ch := &Channel{Name: "#test"}
	user := &users.User{
		NickName: "alice",
		CurrentModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"v"}},
			{Channel: "#other", Modes: []string{"h"}},
		},
		PreservedModes: []modes.UserModes{
			{Channel: "#test", Modes: []string{"v"}},
		},
	}

	ch.SyncMode(user, "o")

	assert.Equal(t, []string{"v", "o"}, modesForChannel(user.CurrentModes, "#test"))
	assert.Equal(t, []string{"v", "o"}, modesForChannel(user.PreservedModes, "#test"))
	// Other channel should be untouched
	assert.Equal(t, []string{"h"}, modesForChannel(user.CurrentModes, "#other"))
}

// TestChannel_String verifies that String contains key channel fields.
func TestChannel_String(t *testing.T) {
	ch := &Channel{
		Name:          "#test",
		PreserveModes: true,
		Ai:            true,
		Sd:            false,
		Users: []*users.User{
			{NickName: "alice"},
			{NickName: "bob"},
		},
	}

	s := ch.String()
	assert.Contains(t, s, "#test")
	assert.Contains(t, s, "2") // user count
	assert.Contains(t, s, "Ai")
	assert.Contains(t, s, "Sd")
}

// TestForgetMode_NilUser verifies early return when user is nil.
func TestForgetMode_NilUser(t *testing.T) {
	ch := &Channel{Name: "#test"}
	ch.ForgetMode(nil, "o")
}

// TestForgetMode_NilChannel verifies early return when channel is nil.
func TestForgetMode_NilChannel(t *testing.T) {
	var ch *Channel
	user := &users.User{NickName: "alice", Ident: "alice", Host: "example.com"}
	ch.ForgetMode(user, "o")
}
