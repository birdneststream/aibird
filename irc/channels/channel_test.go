package channels

import (
	"testing"

	"aibird/irc/users"
	"aibird/irc/users/modes"
)

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
