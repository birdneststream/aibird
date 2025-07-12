package channels

import (
	"aibird/helpers"
	"aibird/irc/users"
	"aibird/irc/users/modes"
	"aibird/logger"
	"fmt"
	"strconv"

	"github.com/lrstanley/girc"
)

func (c *Channel) String() string {
	return girc.Fmt(fmt.Sprintf("{b}Name{b}: %s {b}Users{b}: %d {b}PreserveModes{b}: %s {b}Ai{b}: %s {b}Sd{b}: %s {b}ImageDescribe{b}: %s {b}Sound{b}: %s {b}ActionTrigger{b}: %s {b}TrimOutput{b}: %s",
		c.Name,
		len(c.Users),
		helpers.StringToStatusIndicator(strconv.FormatBool(c.PreserveModes)),
		helpers.StringToStatusIndicator(strconv.FormatBool(c.Ai)),
		helpers.StringToStatusIndicator(strconv.FormatBool(c.Sd)),
		helpers.StringToStatusIndicator(strconv.FormatBool(c.ImageDescribe)),
		helpers.StringToStatusIndicator(strconv.FormatBool(c.Sound)),
		c.ActionTrigger,
		helpers.StringToStatusIndicator(strconv.FormatBool(c.TrimOutput))))
}

func (c *Channel) GetUserWithNick(nick string) (*users.User, error) {
	if c == nil {
		logger.Warn("GetUserWithNick called on nil *Channel")
		return nil, fmt.Errorf("GetUserWithNick called on nil *Channel")
	}

	var foundUsers []*users.User
	for _, user := range c.Users {
		if user.NickName == nick {
			foundUsers = append(foundUsers, user)
		}
	}

	if len(foundUsers) == 0 {
		return nil, nil
	}

	if len(foundUsers) == 1 {
		return foundUsers[0], nil
	}

	latestUser := foundUsers[0]
	for _, user := range foundUsers {
		if user.LatestActivity > latestUser.LatestActivity {
			latestUser = user
		}
	}

	return latestUser, nil
}

func (c *Channel) SyncUser(user *users.User) bool {
	if c == nil {
		logger.Warn("SyncUserToChannel called on nil *Channel")
		return false
	}

	for _, existingUser := range c.Users {
		if existingUser.NickName == user.NickName && existingUser.Ident == user.Ident && existingUser.Host == user.Host {
			// User already exists in the channel
			return false
		}
	}
	// User does not exist, append to channel
	c.Users = append(c.Users, user)
	return true
}

// SyncCurrentModes
// Used on WHO sync to sync the users current modes if they are first seen
func (c *Channel) SyncCurrentModes(user *users.User, newModes []string) {
	if user == nil {
		logger.Warn("SyncCurrentModes: User is nil")
		return
	}

	if c == nil {
		logger.Warn("SyncCurrentModes: Channel is nil")
		return
	}

	for i, userMode := range user.CurrentModes {
		if userMode.Channel == c.Name {
			user.CurrentModes[i].Modes = newModes
			return
		}
	}

	user.CurrentModes = append(user.CurrentModes, modes.UserModes{
		Channel: c.Name,
		Modes:   newModes,
	})

}

func (c *Channel) SyncPreservedModes(user *users.User, newModes []string) {
	if user == nil {
		logger.Warn("SyncPreservedModes: User is nil")
		return
	}

	if c == nil {
		logger.Warn("SyncPreservedModes: Channel is nil")
		return
	}

	for i, userMode := range user.PreservedModes {
		if userMode.Channel == c.Name {
			user.PreservedModes[i].Modes = newModes
			return
		}
	}

	user.PreservedModes = append(user.PreservedModes, modes.UserModes{
		Channel: c.Name,
		Modes:   newModes,
	})

}

// SyncMode - Used on mode change
func (c *Channel) SyncMode(user *users.User, mode string) {
	if user == nil {
		logger.Warn("RememberChannelMode: User is nil")
		return
	}

	if c == nil {
		logger.Warn("RememberChannelMode: Channel is nil")
		return
	}

	hasMode := false

	for i, userMode := range user.PreservedModes {
		if userMode.Channel == c.Name {
			for _, existingMode := range user.PreservedModes[i].Modes {
				if existingMode == mode {
					hasMode = true
					break
				}
			}

			if !hasMode {
				user.PreservedModes[i].Modes = append(user.PreservedModes[i].Modes, mode)
			}

			break
		}
	}

	if !hasMode {
		user.PreservedModes = append(user.PreservedModes, modes.UserModes{
			Channel: c.Name,
			Modes:   []string{mode},
		})
	}

	hasMode = false

	for i, userMode := range user.CurrentModes {
		if userMode.Channel == c.Name {
			// Mode for this channel found, check if the mode already exists
			for _, existingMode := range user.CurrentModes[i].Modes {
				if existingMode == mode {
					hasMode = true
					break
				}
			}
			// Mode not found, add it
			if !hasMode {
				user.CurrentModes[i].Modes = append(user.CurrentModes[i].Modes, mode)
				hasMode = true
			}

			break
		}
	}

	if !hasMode {
		user.CurrentModes = append(user.CurrentModes, modes.UserModes{
			Channel: c.Name,
			Modes:   []string{mode},
		})
	}

}

// ForgetChannelMode - Used on mode change
func (c *Channel) ForgetMode(user *users.User, mode string) {
	if user == nil {
		logger.Warn("ForgetChannelMode: User is nil")
		return
	}

	if c == nil {
		logger.Warn("ForgetChannelMode: Channel is nil")
		return
	}

	for i, userModes := range user.CurrentModes {
		if userModes.Channel == c.Name {
			// Mode for this channel found, remove it
			for j, userMode := range userModes.Modes {
				if userMode == mode {
					user.CurrentModes[i].Modes = append(user.CurrentModes[i].Modes[:j], user.CurrentModes[i].Modes[j+1:]...)
				}
			}
		}
	}

	// We don't want to accidentally remove the owner or admin preserved modes
	if user.IsOwnerUser() || user.IsAdminUser() {
		return
	}

	for i, userModes := range user.PreservedModes {
		if userModes.Channel == c.Name {
			// Mode for this channel found, remove it
			for j, userMode := range userModes.Modes {
				if userMode == mode {
					user.PreservedModes[i].Modes = append(user.PreservedModes[i].Modes[:j], user.PreservedModes[i].Modes[j+1:]...)
				}
			}
		}
	}

}

// CanUserOp
// Used to check if the bot has ops in the channel to save spamming
func (c *Channel) CanUserOp(user *users.User) bool {
	if user == nil {
		return false
	}

	for _, userMode := range user.CurrentModes {
		if userMode.Channel == c.Name {
			for _, mode := range userMode.Modes {
				if mode == "@" || mode == "~" || mode == "%" {
					return true
				}
			}
		}
	}
	return false
}

// Forget and Sync modes for all users in a channel
// Used on PART or KICK to clear modes for all users in a channel
func (c *Channel) AllUsersForgetSyncModes(user *users.User, modes []string) {
	if user == nil {
		return
	}

	// Forget modes for the specified user
	for _, mode := range modes {
		c.ForgetMode(user, mode)
	}

	// Sync modes for all users in the channel
	for _, channelUser := range c.Users {
		for _, mode := range modes {
			c.SyncMode(channelUser, mode)
		}
	}
}

func (c *Channel) RemoveUser(user *users.User) {
	if c == nil || user == nil {
		return
	}

	// Remove the user from the channel's user list
	for i, u := range c.Users {
		if u.NickName == user.NickName {
			c.Users = append(c.Users[:i], c.Users[i+1:]...)
			break
		}
	}
}
