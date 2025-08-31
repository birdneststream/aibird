package users

import (
	"fmt"
	"strconv"
	"time"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/irc/users/modes"
	"aibird/logger"

	"github.com/lrstanley/girc"
)

func (u *User) String() string {
	return girc.Fmt(fmt.Sprintf("{b}PreservedModes{b}: %s, {b}CurrentModes{b}: %s {b}NickName{b}: %s, {b}Ident{b}: %s, {b}Host{b}: %s, {b}LatestActivity{b}: %s, {b}FirstSeen{b}: %s, {b}IsAdmin{b}: %s, {b}IsOwner{b}: %s, {b}AccessLevel{b}: %d, {b}Ignored{b}: %s",
		u.PreservedModes,
		u.CurrentModes,
		u.NickName,
		u.Ident,
		u.Host,
		helpers.UnixTimeToHumanReadable(u.LatestActivity),
		helpers.UnixTimeToHumanReadable(u.FirstSeen)+" ago",
		helpers.StringToStatusIndicator(strconv.FormatBool(u.IsAdmin)),
		helpers.StringToStatusIndicator(strconv.FormatBool(u.IsOwner)),
		u.AccessLevel,
		helpers.StringToStatusIndicator(strconv.FormatBool(u.Ignored))))
}

func (u *User) Touch(latestChat string) {
	u.LatestActivity = time.Now().Unix()
	u.LatestChat = latestChat
	logger.Debug("User touched", "nickname", u.NickName, "ident", u.Ident, "host", u.Host, "latest_chat", latestChat)
}

func (u *User) UpdateNick(nick string) {
	if u.NickName == nick {
		logger.Debug("UpdateNick called but nick unchanged", "current_nick", u.NickName, "new_nick", nick, "ident", u.Ident, "host", u.Host)
		return
	}

	oldNick := u.NickName
	u.NickName = nick
	// Update activity timestamp when nick changes
	u.LatestActivity = time.Now().Unix()
	logger.Debug("Nick updated in User object", "old_nick", oldNick, "new_nick", u.NickName, "ident", u.Ident, "host", u.Host, "latest_activity", u.LatestActivity)
}

func (u *User) UpdateIdentHost(ident, host string) {
	if u.Ident == ident && u.Host == host {
		return
	}

	u.Ident = ident
	u.Host = host
}

func (u *User) Seen() string {
	if u.LatestActivity == 0 {
		return "I first saw " + u.NickName + " " + helpers.UnixTimeToHumanReadable(u.FirstSeen) + " ago but have not seen any chats"
	}

	return u.NickName + " was last seen " + helpers.UnixTimeToHumanReadable(u.LatestActivity) + " ago"
}

func (u *User) CanSkipQueue() bool {
	return u.GetAccessLevel() >= 2 || u.IsAdmin || u.IsOwner
}

func (u *User) IsAdminUser() bool {
	return u.IsAdmin
}

func (u *User) IsOwnerUser() bool {
	return u.IsOwner
}

func (u *User) Ignore() {
	u.Ignored = true
}

func (u *User) UnIgnore() {
	u.Ignored = false
}

func (u *User) GetAccessLevel() int {
	return u.AccessLevel
}

func (u *User) GetAccessLevelString() string {
	switch u.AccessLevel {
	case 1:
		return "Chat Pal Status"
	case 2:
		return "Swan Squadron"
	case 3:
		return "Sparrow Society"
	case 4:
		return "Golden Toucans"
	case 5:
		return "Free Bird"
	default:
		return "Free"
	}
}

func (u *User) CanUse4090() bool {
	// Perform a strict check to prevent false positives
	accessLevel := u.GetAccessLevel()
	return accessLevel >= 2 || u.IsAdmin || u.IsOwner
}

func (u *User) IsIgnored() bool {
	return u.Ignored
}

func (u *User) SetBasePrompt(prompt string) {
	u.AiBasePrompt = prompt
}

func (u *User) SetAiService(service string) {
	u.AiService = service
}

func (u *User) SetPersonality(personality string) {
	u.AiPersonality = personality
}

func (u *User) GetBasePrompt() string {
	return u.AiBasePrompt
}

func (u *User) GetAiService() string {
	return u.AiService
}

func (u *User) GetPersonality() string {
	return u.AiPersonality
}

func (u *User) SetAiModel(model string) {
	u.AiModel = model
}

func (u *User) GetAiModel() string {
	return u.AiModel
}

func (u *User) HasCurrentModes(channel string) bool {
	for _, mode := range u.CurrentModes {
		if mode.Channel == channel {
			return true
		}
	}
	return false
}

func (u *User) HasPreservedModes(channel string) bool {
	for _, mode := range u.PreservedModes {
		if mode.Channel == channel {
			return true
		}
	}
	return false
}

func (u *User) HasAnyMode() bool {
	if len(u.CurrentModes) == 0 {
		return false
	}
	for _, mode := range u.CurrentModes {
		if len(mode.Modes) > 0 {
			return true
		}
	}
	return false
}

// ===========================================
// NORMALIZED DATABASE CONVERSION METHODS
// ===========================================

// ToUserData converts a User struct to birdbase.UserData for database storage
func (u *User) ToUserData(networkID int) (*birdbase.UserData, error) {
	userData := &birdbase.UserData{
		NetworkID:      networkID,
		NickName:       u.NickName,
		Ident:          u.Ident,
		Host:           u.Host,
		FirstSeen:      u.FirstSeen,
		LatestActivity: u.LatestActivity,
		LatestChat:     u.LatestChat,
		IsAdmin:        u.IsAdmin,
		IsOwner:        u.IsOwner,
		Ignored:        u.Ignored,
		AccessLevel:    u.AccessLevel,
		AiService:      u.AiService,
		AiModel:        u.AiModel,
		AiBasePrompt:   u.AiBasePrompt,
		AiPersonality:  u.AiPersonality,
	}

	// Convert PreservedModes
	for _, mode := range u.PreservedModes {
		userData.PreservedModes = append(userData.PreservedModes, birdbase.UserModeData{
			Channel: mode.Channel,
			Modes:   mode.Modes,
		})
	}

	// Convert CurrentModes
	for _, mode := range u.CurrentModes {
		userData.CurrentModes = append(userData.CurrentModes, birdbase.UserModeData{
			Channel: mode.Channel,
			Modes:   mode.Modes,
		})
	}

	return userData, nil
}

// FromUserData converts birdbase.UserData back to a User struct
func (u *User) FromUserData(userData *birdbase.UserData) error {
	u.NickName = userData.NickName
	u.Ident = userData.Ident
	u.Host = userData.Host
	u.FirstSeen = userData.FirstSeen
	u.LatestActivity = userData.LatestActivity
	u.LatestChat = userData.LatestChat
	u.IsAdmin = userData.IsAdmin
	u.IsOwner = userData.IsOwner
	u.Ignored = userData.Ignored
	u.AccessLevel = userData.AccessLevel
	u.AiService = userData.AiService
	u.AiModel = userData.AiModel
	u.AiBasePrompt = userData.AiBasePrompt
	u.AiPersonality = userData.AiPersonality

	// Clear existing modes
	u.PreservedModes = nil
	u.CurrentModes = nil

	// Convert PreservedModes
	for _, modeData := range userData.PreservedModes {
		u.PreservedModes = append(u.PreservedModes, modes.UserModes{
			Channel: modeData.Channel,
			Modes:   modeData.Modes,
		})
	}

	// Convert CurrentModes
	for _, modeData := range userData.CurrentModes {
		u.CurrentModes = append(u.CurrentModes, modes.UserModes{
			Channel: modeData.Channel,
			Modes:   modeData.Modes,
		})
	}

	return nil
}

// NewUserFromData creates a new User from birdbase.UserData
func NewUserFromData(userData *birdbase.UserData) *User {
	user := &User{}
	user.FromUserData(userData)
	return user
}
