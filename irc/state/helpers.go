package state

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/http/uploaders/birdhole"
	"aibird/irc/users"
	"aibird/logger"

	"github.com/google/uuid"
)

// connectionGracePeriod is the time after connect before mode enforcement begins.
const connectionGracePeriod = 30 * time.Second

// GetModesFromChannel
// Uses the state user and channel to get the modes for the user in the channel
func (s *State) GetModesFromChannel() []string {
	if s.User == nil {
		logger.Warn("GetModesFromChannel: User is nil")
		return []string{}
	}

	if s.Channel == nil {
		logger.Warn("GetModesFromChannel: Channel is nil")
		return []string{}
	}

	var preservedModeMap = make(map[string]bool)
	for _, userMode := range s.User.PreservedModes {
		if userMode.Channel == s.Channel.Name {
			for _, mode := range userMode.Modes {
				preservedModeMap[mode] = true
			}
		}
	}

	var currentModeMap = make(map[string]bool)
	for _, userMode := range s.User.CurrentModes {
		if userMode.Channel == s.Channel.Name {
			for _, mode := range userMode.Modes {
				currentModeMap[mode] = true
			}
		}
	}

	var modesToSet []string
	for mode := range preservedModeMap {
		if !currentModeMap[mode] {
			modesToSet = append(modesToSet, helpers.ReverseModeMap(mode))
		}
	}

	return modesToSet
}

func (s *State) CompareUserModes() { //nolint:gocyclo
	var differences []ModeDifference

	for _, user := range s.Channel.Users {
		var diffModes []string
		preservedModeMap := make(map[string]bool)
		currentModeMap := make(map[string]bool)

		// Map preserved modes for easy comparison
		for _, mode := range user.PreservedModes {
			if mode.Channel == s.Channel.Name {
				for _, m := range mode.Modes {
					preservedModeMap[m] = true
				}
			}
		}

		// Map current modes for easy comparison
		for _, mode := range user.CurrentModes {
			if mode.Channel == s.Channel.Name {
				for _, m := range mode.Modes {
					currentModeMap[m] = true
					// If the mode is not in preserved modes, add it to diffModes
					if !preservedModeMap[m] {
						diffModes = append(diffModes, m)
					}
				}
			}
		}

		// Check for modes in preserved not in current
		for mode := range preservedModeMap {
			if !currentModeMap[mode] {
				diffModes = append(diffModes, mode)
			}
		}

		if len(diffModes) > 0 {
			differences = append(differences, ModeDifference{
				Nick:  user.NickName,
				Modes: diffModes,
			})
		}
	}

	counter := 0
	var users []string
	var modes string

	for i, diff := range differences {
		users = append(users, diff.Nick)
		for _, mode := range diff.Modes {
			modes += "+" + helpers.ReverseModeMap(mode)
		}

		counter++
		if counter == 4 || i == len(differences)-1 {
			s.Client.Cmd.SendRaw("MODE " + s.Channel.Name + " " + modes + " " + strings.Join(users, " "))

			counter = 0
			users = []string{}
			modes = ""
		}
	}

}

// MessageFloodCheck implements the second tier of flood protection.
// Called from Verify() before command execution. Uses ident+host-based keys
// (survives nick changes) and silently ignores the user on threshold breach.
// This is the persistent ignore layer — separate from checkFlood in main.go
// which kicks the user from the channel.
//
// Returns true to block the command if:
// - User is already flood-banned (ban check), or
// - User sent 2+ messages in the 3-second window (rate limiting), or
// - User exceeded the configured flood threshold (ignore + ban)
func (s *State) MessageFloodCheck() bool {
	if s.User.IsOwner {
		return false
	}

	// Use default values if config values are not set properly
	floodThreshold := s.Config.AiBird.FloodThreshold
	if floodThreshold == 0 {
		floodThreshold = 3 // Default to 3 if not configured
	}

	floodIgnoreMinutes := s.Config.AiBird.FloodIgnoreMinutes
	if floodIgnoreMinutes == 0 {
		floodIgnoreMinutes = 15 // Default to 15 minutes if not configured
	}

	ban := s.Network.Name + s.Channel.Name + s.User.Host + s.User.Ident + "flood_ban"
	key := s.Network.Name + s.Channel.Name + s.User.Host + s.User.Ident + "flood_check"

	// Check if user is currently banned using in-memory flood manager
	if birdbase.FloodManager.IsFloodBanned(ban) {
		return true
	}

	// Increment flood counter using in-memory flood manager
	countInt := birdbase.FloodManager.IncrementFloodCounter(key, 3*time.Second)

	logger.Debug("Flood check values", "network", s.Network.NetworkName, "user", s.User.NickName, "count", countInt, "threshold", floodThreshold, "ignore_minutes", floodIgnoreMinutes)

	if countInt > floodThreshold {
		// Set user as ignored instead of kick/ban system
		s.User.Ignored = true
		s.Network.Save()

		// Set temporary ignore duration using in-memory flood manager
		birdbase.FloodManager.SetFloodBan(ban, time.Duration(floodIgnoreMinutes)*time.Minute)

		// Use flood manager's ban expiration to determine when to un-ignore
		// The user will be un-ignored on their next command after the ban expires
		// This avoids spawning uncontrolled goroutines

		logger.Info("User ignored due to flood", "user", s.User.NickName, "network", s.Network.NetworkName, "duration", fmt.Sprintf("%dm", floodIgnoreMinutes))
		return true
	}

	// Rate-limit: block if user sent 2+ messages in the 3-second window,
	// even if threshold not yet breached. This prevents rapid-fire commands.
	return countInt > 1
}

func (s *State) GetActionTrigger() string {
	if s.Channel.ActionTrigger != "" {
		return s.Channel.ActionTrigger
	}

	if s.Network.ActionTrigger != "" {
		return s.Network.ActionTrigger
	}

	return "!"
}

func (s *State) SyncUsersFromWho() {
	ident := s.Event.Params[2]
	host := s.Event.Params[3]
	nick := s.Event.Params[5]
	modes := helpers.GetModes(s.Event.Params[6])
	logger.Debug("SyncUsersFromWho parsing", "network", s.Network.NetworkName, "ident", ident, "host", host, "nick", nick, "channel", s.Channel.Name, "event_params", s.Event.Params)
	findUser := s.Network.GetUserWithIdentAndHost(ident, host)

	if findUser != nil {
		findUser.UpdateNick(nick)

		// Associate user to channel
		s.Channel.SyncUser(findUser)

		// Track user-channel membership in database from WHO response
		if err := birdbase.AddUserToChannel(s.Network.NetworkName, findUser.Ident, findUser.Host, s.Channel.Name); err != nil {
			logger.Warn("Failed to track user-channel membership from WHO", "error", err, "network", s.Network.NetworkName, "nick", findUser.NickName, "channel", s.Channel.Name)
		}

		// Get their current modes
		s.Channel.SyncCurrentModes(findUser, modes)

		if !findUser.HasPreservedModes(s.Channel.Name) {
			logger.Warn("User does not have channel data", "channel", s.Channel.Name)
			s.Channel.SyncPreservedModes(findUser, modes)
		} else {
			// user already exists, check if modes are in sync
			s.User = findUser

			// Only restore modes if preservation is enabled AND we're past the connection grace period
			// Skip mode restoration during initial connection to avoid flooding when connecting through ZNC
			if s.ShouldPreserveModes() && (s.Network.ConnectedAt.IsZero() || time.Since(s.Network.ConnectedAt) >= connectionGracePeriod) {
				applyOps := s.GetModesFromChannel()
				if len(applyOps) > 0 {
					s.Client.Cmd.SendRaw("MODE " + s.Channel.Name + " +" + strings.Join(applyOps, "") + " " + findUser.NickName)
				}
			}
		}
		return
	}

	// Create new user if not found
	ignoreStatus := s.Network.IsNickIgnored(nick)
	logger.Debug("Creating new user", "network", s.Network.NetworkName, "nick", nick, "ident", ident, "host", host, "ignored_status", ignoreStatus)

	user := users.User{
		NickName:    nick,
		Ident:       ident,
		Host:        host,
		FirstSeen:   time.Now().Unix(),
		IsAdmin:     s.Network.IsIdentHostAdmin(ident, host),
		IsOwner:     s.Network.IsIdentHostOwner(ident, host),
		Ignored:     ignoreStatus,
		AccessLevel: 0,
		AiService:   "llamacpp",
		GircUser:    s.Client.LookupUser(nick),
	}

	// Sync current and preserved modes
	s.Channel.SyncCurrentModes(&user, modes)
	s.Channel.SyncPreservedModes(&user, modes)

	// append to s.Network.Users
	s.Network.Users = append(s.Network.Users, user)
	s.Channel.SyncUser(&user)

	// Track user-channel membership in database for new user from WHO response
	if err := birdbase.AddUserToChannel(s.Network.NetworkName, user.Ident, user.Host, s.Channel.Name); err != nil {
		logger.Warn("Failed to track new user-channel membership from WHO", "error", err, "network", s.Network.NetworkName, "nick", user.NickName, "channel", s.Channel.Name)
	}

	s.Network.Save()
}

func (s *State) TextToBirdhole(message string) {
	name := uuid.New().String()
	trim := s.ShouldTrimOutput(message)

	// Use a secure temporary file path in /tmp directory
	filePath := os.TempDir() + "/" + name + ".txt"

	// write to a txt file message
	err := os.WriteFile(filePath, []byte(message), 0600) // More restrictive permissions
	if err != nil {
		s.SendError(err.Error())
		return
	}

	// Ensure the file gets cleaned up when done
	defer os.Remove(filePath)

	response, err := birdhole.BirdHole(filePath, s.Action()+" "+s.Message(), nil, s.Config.Birdhole)
	if err != nil {
		s.SendError("Failed to upload to birdhole: " + err.Error())
		return
	}

	if trim {
		// Avoid potential slice panic by checking length
		if len(message) > 250 {
			message = strings.ReplaceAll(message, "\n", " ")
			message = message[:250]
		} else {
			message = strings.ReplaceAll(message, "\n", " ")
		}
		s.ReplyTo(response + " - " + message)
	} else {
		s.Send(message)
	}
}

// updateUserField applies a single argument to a user field using explicit type-safe setters.
// Returns true if the field was recognized (including immutable blocked fields).
// Uses originalKey for display messages to preserve user-specified casing.
func (s *State) updateUserField(user *users.User, originalKey string, value interface{}) bool {
	key := strings.ToLower(originalKey)
	switch key {
	// Immutable fields — blocked
	case "nickname", "ident", "host", "preservedmodes", "currentmodes", "gircuser":
		s.SendWarning(fmt.Sprintf("Cannot change protected %s", helpers.CapitaliseFirst(originalKey)))
		return true

	// Bool fields
	case "isadmin":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating IsAdmin: %v", err))
			return true
		}
		user.IsAdmin = val
		s.SendSuccess(fmt.Sprintf("Updated IsAdmin to %t", user.IsAdmin))
	case "isowner":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating IsOwner: %v", err))
			return true
		}
		user.IsOwner = val
		s.SendSuccess(fmt.Sprintf("Updated IsOwner to %t", user.IsOwner))
	case "ignored":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Ignored: %v", err))
			return true
		}
		user.Ignored = val
		s.SendSuccess(fmt.Sprintf("Updated Ignored to %t", user.Ignored))

	// Int fields
	case "accesslevel":
		val, err := toIntE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating AccessLevel: %v", err))
			return true
		}
		user.AccessLevel = val
		s.SendSuccess(fmt.Sprintf("Updated AccessLevel to %d", user.AccessLevel))
	case "firstseen":
		val, err := toInt64E(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating FirstSeen: %v", err))
			return true
		}
		user.FirstSeen = val
		s.SendSuccess(fmt.Sprintf("Updated FirstSeen to %d", user.FirstSeen))
	case "latestactivity":
		val, err := toInt64E(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating LatestActivity: %v", err))
			return true
		}
		user.LatestActivity = val
		s.SendSuccess(fmt.Sprintf("Updated LatestActivity to %d", user.LatestActivity))

	// String fields
	case "aiservice":
		user.AiService = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated AiService to %s", user.AiService))
	case "aimodel":
		user.AiModel = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated AiModel to %s", user.AiModel))
	case "aibaseprompt":
		user.AiBasePrompt = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated AiBasePrompt to %s", user.AiBasePrompt))
	case "aipersonality":
		user.AiPersonality = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated AiPersonality to %s", user.AiPersonality))
	case "latestchat":
		user.LatestChat = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated LatestChat to %s", user.LatestChat))

	default:
		return false // Field not recognized
	}
	return true
}

// UpdateUserBasedOnArgs updates user fields based on the parsed arguments.
// Uses explicit type-safe setters instead of reflection.
func (s *State) UpdateUserBasedOnArgs(user *users.User) {
	for _, arg := range s.GetArguments() {
		if !s.updateUserField(user, arg.Key, arg.Value) {
			s.SendWarning(fmt.Sprintf("Unknown field: %s", arg.Key))
		}
	}
	s.Network.Save()
}

// updateChannelField applies a single argument to a channel field using explicit type-safe setters.
// Returns true if the field was recognized (including immutable blocked fields).
func (s *State) updateChannelField(originalKey string, value interface{}) bool {
	key := strings.ToLower(originalKey)
	switch key {
	// Immutable fields — blocked
	case "name", "users", "denycommands":
		s.SendWarning(fmt.Sprintf("Cannot change protected %s", helpers.CapitaliseFirst(originalKey)))
		return true

	// Bool fields
	case "ai":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Ai: %v", err))
			return true
		}
		s.Channel.Ai = val
		s.SendSuccess(fmt.Sprintf("Updated Ai to %t", s.Channel.Ai))
	case "sd":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Sd: %v", err))
			return true
		}
		s.Channel.Sd = val
		s.SendSuccess(fmt.Sprintf("Updated Sd to %t", s.Channel.Sd))
	case "imagedescribe":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating ImageDescribe: %v", err))
			return true
		}
		s.Channel.ImageDescribe = val
		s.SendSuccess(fmt.Sprintf("Updated ImageDescribe to %t", s.Channel.ImageDescribe))
	case "sound":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Sound: %v", err))
			return true
		}
		s.Channel.Sound = val
		s.SendSuccess(fmt.Sprintf("Updated Sound to %t", s.Channel.Sound))
	case "video":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Video: %v", err))
			return true
		}
		s.Channel.Video = val
		s.SendSuccess(fmt.Sprintf("Updated Video to %t", s.Channel.Video))
	case "trimoutput":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating TrimOutput: %v", err))
			return true
		}
		s.Channel.TrimOutput = val
		s.SendSuccess(fmt.Sprintf("Updated TrimOutput to %t", s.Channel.TrimOutput))
	case "preservemodes":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating PreserveModes: %v", err))
			return true
		}
		s.Channel.PreserveModes = val
		s.SendSuccess(fmt.Sprintf("Updated PreserveModes to %t", s.Channel.PreserveModes))
	case "chatmode":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating ChatMode: %v", err))
			return true
		}
		s.Channel.ChatMode = val
		s.SendSuccess(fmt.Sprintf("Updated ChatMode to %t", s.Channel.ChatMode))
	case "companionmode":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating CompanionMode: %v", err))
			return true
		}
		s.Channel.CompanionMode = val
		s.SendSuccess(fmt.Sprintf("Updated CompanionMode to %t", s.Channel.CompanionMode))
	case "sendarturl":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating SendArtURL: %v", err))
			return true
		}
		s.Channel.SendArtURL = val
		s.SendSuccess(fmt.Sprintf("Updated SendArtURL to %t", s.Channel.SendArtURL))

	// String fields
	case "actiontrigger":
		s.Channel.ActionTrigger = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated ActionTrigger to %s", s.Channel.ActionTrigger))
	case "chatpersonality":
		s.Channel.ChatPersonality = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated ChatPersonality to %s", s.Channel.ChatPersonality))

	// Float fields
	case "chatresponserate":
		val, err := toFloat64E(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating ChatResponseRate: %v", err))
			return true
		}
		s.Channel.ChatResponseRate = val
		s.SendSuccess(fmt.Sprintf("Updated ChatResponseRate to %f", s.Channel.ChatResponseRate))

	default:
		return false
	}
	return true
}

// UpdateChannelBasedOnArgs updates channel fields based on the parsed arguments.
func (s *State) UpdateChannelBasedOnArgs() {
	for _, arg := range s.GetArguments() {
		if !s.updateChannelField(arg.Key, arg.Value) {
			s.SendWarning(fmt.Sprintf("Unknown field: %s", arg.Key))
		}
	}
	s.Network.Save()
}

// updateNetworkField applies a single argument to a network field using explicit type-safe setters.
// Returns true if the field was recognized (including immutable blocked fields).
func (s *State) updateNetworkField(originalKey string, value interface{}) bool {
	key := strings.ToLower(originalKey)
	switch key {
	// Immutable fields — blocked (includes sensitive and runtime fields)
	case "name", "networkname", "nick", "users", "channels", "servers", "modesatonce",
		"pass", "nickservpass", "connectedat", "ignorednicks", "adminhosts", "denycommands":
		s.SendWarning(fmt.Sprintf("Cannot change protected %s", helpers.CapitaliseFirst(originalKey)))
		return true

	// Bool fields
	case "enabled":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Enabled: %v", err))
			return true
		}
		s.Network.Enabled = val
		s.SendSuccess(fmt.Sprintf("Updated Enabled to %t", s.Network.Enabled))
	case "preservemodes":
		val, err := toBoolE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating PreserveModes: %v", err))
			return true
		}
		s.Network.PreserveModes = val
		s.SendSuccess(fmt.Sprintf("Updated PreserveModes to %t", s.Network.PreserveModes))

	// Int fields
	case "pingdelay":
		val, err := toIntE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating PingDelay: %v", err))
			return true
		}
		s.Network.PingDelay = val
		s.SendSuccess(fmt.Sprintf("Updated PingDelay to %d", s.Network.PingDelay))
	case "throttle":
		val, err := toIntE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Throttle: %v", err))
			return true
		}
		s.Network.Throttle = val
		s.SendSuccess(fmt.Sprintf("Updated Throttle to %d", s.Network.Throttle))
	case "burst":
		val, err := toIntE(value)
		if err != nil {
			s.SendError(fmt.Sprintf("Error updating Burst: %v", err))
			return true
		}
		s.Network.Burst = val
		s.SendSuccess(fmt.Sprintf("Updated Burst to %d", s.Network.Burst))

	// String fields
	case "version":
		s.Network.Version = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated Version to %s", s.Network.Version))
	case "actiontrigger":
		s.Network.ActionTrigger = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated ActionTrigger to %s", s.Network.ActionTrigger))
	case "user":
		s.Network.User = toString(value)
		s.SendSuccess(fmt.Sprintf("Updated User to %s", s.Network.User))

	default:
		return false
	}
	return true
}

// UpdateNetworkBasedOnArgs updates network fields based on the parsed arguments.
func (s *State) UpdateNetworkBasedOnArgs() {
	for _, arg := range s.GetArguments() {
		if !s.updateNetworkField(arg.Key, arg.Value) {
			s.SendWarning(fmt.Sprintf("Unknown field: %s", arg.Key))
		}
	}
	s.Network.Save()
}

// Type conversion helpers for argument values.
// These handle the various types that ParseArguments produces (bool, int, string).
// Error-returning variants (toBoolE, toIntE, toInt64E, toFloat64E) are used by
// the field setters to report invalid input back to the IRC user.
// Non-error variants (toString) are used for string fields where any value is valid.

func toBoolE(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return strconv.ParseBool(val)
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

func toIntE(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case string:
		i, err := strconv.Atoi(val)
		return i, err
	case float64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func toInt64E(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case string:
		i, err := strconv.ParseInt(val, 10, 64)
		return i, err
	case float64:
		return int64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

func toFloat64E(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		f, err := strconv.ParseFloat(val, 64)
		return f, err
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// RestoreUserModes checks all users in the channel and restores any missing modes
func (s *State) RestoreUserModes() {
	if s.Channel == nil {
		logger.Warn("RestoreUserModes called with nil channel")
		return
	}

	// Skip mode restoration during initial connection period (ZNC buffer playback)
	// When connecting through ZNC, the bot receives WHO data but users already have their modes
	// Wait 30 seconds after connection to allow ZNC state to stabilize
	if !s.Network.ConnectedAt.IsZero() && time.Since(s.Network.ConnectedAt) < connectionGracePeriod {
		logger.Debug("Skipping mode restoration during connection grace period", "network", s.Network.NetworkName, "channel", s.Channel.Name, "elapsed", time.Since(s.Network.ConnectedAt))
		return
	}

	// Check if mode preservation is enabled for this channel
	if !s.ShouldPreserveModes() {
		logger.Debug("Mode preservation disabled, skipping restoration", "network", s.Network.NetworkName, "channel", s.Channel.Name)
		return
	}

	logger.Debug("Starting mode restoration for channel", "network", s.Network.NetworkName, "channel", s.Channel.Name)

	// Get all users currently in this channel
	for i := range s.Network.Users {
		user := &s.Network.Users[i]

		// Check if this user has preserved modes for this channel
		if !user.HasPreservedModes(s.Channel.Name) {
			continue
		}

		// Get modes this user should have
		s.User = user
		applyOps := s.GetModesFromChannel()
		if len(applyOps) > 0 {
			logger.Debug("Restoring modes for user", "network", s.Network.NetworkName, "channel", s.Channel.Name, "nick", user.NickName, "modes", applyOps)

			// Apply the missing modes
			modeString := "+" + strings.Join(applyOps, "")
			modeCommand := fmt.Sprintf("MODE %s %s %s", s.Channel.Name, modeString, user.NickName)

			if err := s.Client.Cmd.SendRaw(modeCommand); err != nil {
				logger.Error("Failed to send mode command", "error", err, "command", modeCommand)
			}
		}
	}

	logger.Debug("Completed mode restoration for channel", "network", s.Network.NetworkName, "channel", s.Channel.Name)
}
