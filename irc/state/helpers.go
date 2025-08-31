package state

import (
	"fmt"
	"os"
	"reflect"
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

func (s *State) CheckShouldVoiceUser() bool {
	if s.User == nil {
		return false
	}

	for _, userMode := range s.User.PreservedModes {
		if userMode.Channel == s.Channel.Name {
			for _, mode := range userMode.Modes {
				if mode == "+" {
					return true
				}
			}
		}
	}

	return false
}

func (s *State) DelayedWhoTimer() {
	if s.Channel.ActivityTimer != nil {

		if !s.Channel.ActivityTimer.Stop() {
			select {
			case <-s.Channel.ActivityTimer.C:
			default:
			}
		}
	}
	s.Channel.ActivityTimer = time.NewTimer(1 * time.Second)
	go func() {
		<-s.Channel.ActivityTimer.C
		_ = s.Client.Cmd.SendRaw("WHO " + s.Channel.Name)
	}()
}

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

func (s *State) MessageFloodCheck() bool {
	if s.User.IsOwner {
		return false
	}

	ban := s.Network.Name + s.Channel.Name + s.User.Host + s.User.Ident + "flood_ban"
	key := s.Network.Name + s.Channel.Name + s.User.Host + s.User.Ident + "flood_check"

	// Check if user is currently banned using in-memory flood manager
	if birdbase.FloodManager.IsFloodBanned(ban) {
		return true
	}

	// Increment flood counter using in-memory flood manager
	countInt := birdbase.FloodManager.IncrementFloodCounter(key, 3*time.Second)

	if countInt > s.Config.AiBird.FloodThreshold {
		// Set user as ignored instead of kick/ban system
		s.User.Ignored = true
		s.Network.Save()

		// Set temporary ignore duration using in-memory flood manager
		birdbase.FloodManager.SetFloodBan(ban, time.Duration(s.Config.AiBird.FloodIgnoreMinutes)*time.Minute)

		// Schedule automatic un-ignore after the flood ignore time
		go func() {
			time.Sleep(time.Duration(s.Config.AiBird.FloodIgnoreMinutes) * time.Minute)
			s.User.Ignored = false
			s.Network.Save()
			logger.Info("User automatically un-ignored after flood timeout", "user", s.User.NickName, "network", s.Network.NetworkName)
		}()

		logger.Info("User ignored due to flood", "user", s.User.NickName, "network", s.Network.NetworkName, "duration", fmt.Sprintf("%dm", s.Config.AiBird.FloodIgnoreMinutes))
		return true
	}

	return countInt > 1 // Return true if we have multiple messages
}

func (s *State) JoinFloodCheck() {
	key := s.Network.Name + s.Channel.Name + "flood_check"
	waitTime := 3 * time.Second

	// If we have a lot of people rejoin on a netsplit we don't want to trigger this
	// we can see if the bot already reconises them
	if s.Client.LookupUser(s.Event.Source.Name) != nil {
		return
	}

	// Increment join flood counter using in-memory flood manager
	countInt := birdbase.FloodManager.IncrementFloodCounter(key, waitTime)

	if countInt > 4 {
		go s.RemoveFloodCheck()
		// +i the channel
		s.Client.Cmd.Mode(s.Channel.Name, "+i")
		s.Client.Cmd.Mode(s.Channel.Name, "+m")
	}
}

func (s *State) RemoveFloodCheck() {
	time.Sleep(2 * time.Minute)

	// Remove invite-only and moderated modes
	s.Client.Cmd.Mode(s.Channel.Name, "-i")
	s.Client.Cmd.Mode(s.Channel.Name, "-m")

	// Note: We don't need to explicitly remove the flood check key
	// as it will expire automatically from in-memory storage
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

			// Only restore modes if preservation is enabled
			if s.ShouldPreserveModes() {
				applyOps := s.GetModesFromChannel()
				if len(applyOps) > 0 {
					s.Client.Cmd.SendRaw("MODE " + s.Channel.Name + " +" + strings.Join(applyOps, "") + " " + findUser.NickName)
				}
			}
		}
		return
	}

	// Create new user if not found
	user := users.User{
		NickName:    nick,
		Ident:       ident,
		Host:        host,
		FirstSeen:   time.Now().Unix(),
		IsAdmin:     s.Network.IsIdentHostAdmin(ident, host),
		IsOwner:     s.Network.IsIdentHostOwner(ident, host),
		Ignored:     s.Network.IsNickIgnored(nick),
		AccessLevel: 0,
		AiService:   "ollama",
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

// UpdateUserBasedOnArgs This will update the user based on the arguments provided
func (s *State) UpdateUserBasedOnArgs(user *users.User) {
	immutableKeys := map[string]bool{
		"NickName":       true,
		"Ident":          true,
		"Host":           true,
		"PreservedModes": true,
		"CurrentModes":   true,
	}

	s.UpdateBasedOnArgs(user, immutableKeys)
}

func (s *State) UpdateChannelBasedOnArgs() {
	immutableKeys := map[string]bool{
		"Name":          true,
		"Users":         true,
		"ActivityTimer": true,
	}

	s.UpdateBasedOnArgs(s.Channel, immutableKeys)
}

func (s *State) UpdateNetworkBasedOnArgs() {
	immutableKeys := map[string]bool{
		"Name":        true,
		"NetworkName": true,
		"Nick":        true,
		"Users":       true,
		"Channels":    true,
		"Servers":     true,
		"ModesAtOnce": true,
	}

	s.UpdateBasedOnArgs(s.Network, immutableKeys)
}

// UpdateBasedOnArgs accepts Network, Channel and User to update their fields based on the arguments provided
func (s *State) UpdateBasedOnArgs(obj interface{}, immutableKeys map[string]bool) { //nolint:gocyclo
	args := s.GetArguments()

	uValue := reflect.ValueOf(obj).Elem()

	for _, arg := range args {
		fieldName := helpers.CapitaliseFirst(arg.Key)
		// Check if the field is immutable
		if _, exists := immutableKeys[fieldName]; exists {
			s.SendWarning(fmt.Sprintf("Cannot change protected %s", fieldName))
			continue // Skip the update for this field
		}

		var fieldValueStr string

		// Determine the type of arg.Value and convert to string if necessary
		switch v := arg.Value.(type) {
		case string:
			fieldValueStr = v
		case bool:
			fieldValueStr = strconv.FormatBool(v)
		case int, int8, int16, int32, int64:
			fieldValueStr = fmt.Sprintf("%d", v)
		case uint, uint8, uint16, uint32, uint64:
			fieldValueStr = fmt.Sprintf("%d", v)
		case float32, float64:
			fieldValueStr = fmt.Sprintf("%f", v)
		default:
			s.SendWarning(fmt.Sprintf("Unsupported type for field %s: %T", fieldName, arg.Value))
			continue
		}

		if fieldVal := uValue.FieldByName(fieldName); fieldVal.IsValid() {
			var successMessage string
			var boolErr, intErr, uintErr, floatErr error
			var boolVal bool
			var intVal int64
			var uintVal uint64
			var floatVal float64
			switch fieldVal.Kind() {
			case reflect.Bool:
				boolVal, boolErr = strconv.ParseBool(fieldValueStr)
				if boolErr == nil {
					fieldVal.SetBool(boolVal)
					successMessage = fmt.Sprintf("Updated %s to %t", fieldName, boolVal)
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				intVal, intErr = strconv.ParseInt(fieldValueStr, 10, fieldVal.Type().Bits())
				if intErr == nil {
					fieldVal.SetInt(intVal)
					successMessage = fmt.Sprintf("Updated %s to %d", fieldName, intVal)
				}
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				uintVal, uintErr = strconv.ParseUint(fieldValueStr, 10, fieldVal.Type().Bits())
				if uintErr == nil {
					fieldVal.SetUint(uintVal)
					successMessage = fmt.Sprintf("Updated %s to %d", fieldName, uintVal)
				}
			case reflect.String:
				fieldVal.SetString(fieldValueStr)
				successMessage = fmt.Sprintf("Updated %s to %s", fieldName, fieldValueStr)
			case reflect.Float32, reflect.Float64:
				floatVal, floatErr = strconv.ParseFloat(fieldValueStr, fieldVal.Type().Bits())
				if floatErr == nil {
					fieldVal.SetFloat(floatVal)
					successMessage = fmt.Sprintf("Updated %s to %f", fieldName, floatVal)
				}
			}

			// Check for parsing errors
			if boolErr != nil || intErr != nil || uintErr != nil || floatErr != nil {
				var parseErr error
				if boolErr != nil {
					parseErr = boolErr
				} else if intErr != nil {
					parseErr = intErr
				} else if uintErr != nil {
					parseErr = uintErr
				} else if floatErr != nil {
					parseErr = floatErr
				}
				s.SendError(fmt.Sprintf("Error updating %s: %v", fieldName, parseErr))
			} else if successMessage != "" {
				s.SendSuccess(successMessage)
			}
		}
	}

	s.Network.Save()
}

// RestoreUserModes checks all users in the channel and restores any missing modes
func (s *State) RestoreUserModes() {
	if s.Channel == nil {
		logger.Warn("RestoreUserModes called with nil channel")
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
