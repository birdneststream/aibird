package commands

import (
	"aibird/irc/commands/help"
	"aibird/settings"
	"strings"
)

// GetAllCommands returns a slice of all available command names across all command types
// It can optionally filter by channel capabilities (ai, sd, sound, video)
func GetAllCommands(config settings.AiBird, enableAi, enableSd, enableSound, enableVideo bool, isAdmin, isOwner bool) []string {
	var commands []string

	// Add standard commands (always available)
	for _, cmd := range help.StandardHelp() {
		commands = append(commands, cmd.Name)
	}

	// Add image commands if SD is enabled
	if enableSd {
		for _, cmd := range help.ImageHelp(config) {
			commands = append(commands, cmd.Name)
		}
	}

	// Add video commands if Video is enabled
	if enableVideo {
		for _, cmd := range help.VideoHelp(config) {
			commands = append(commands, cmd.Name)
		}
	}

	// Add text commands if AI is enabled
	if enableAi {
		for _, cmd := range help.TextHelp() {
			commands = append(commands, cmd.Name)
		}
	}

	// Add sound commands if sound is enabled
	if enableSound {
		for _, cmd := range help.SoundHelp(config) {
			commands = append(commands, cmd.Name)
		}
	}

	// Add admin commands if user is admin
	if isAdmin {
		for _, cmd := range help.AdminHelp() {
			commands = append(commands, cmd.Name)
		}
	}

	// Add owner commands if user is owner
	if isOwner {
		for _, cmd := range help.OwnerHelp() {
			if cmd.Name != "" { // Skip empty command names
				commands = append(commands, cmd.Name)
			}
		}
	}

	return commands
}

// GetAllCommandsUnfiltered returns all commands regardless of channel capabilities
// This is useful for admin purposes or when you don't have channel context
func GetAllCommandsUnfiltered(config settings.AiBird) []string {
	return GetAllCommands(config, true, true, true, true, true, true)
}

// IsValidCommand checks if a command is in the list of valid commands
// This ignores channel settings and returns true if the command exists anywhere
func IsValidCommand(command string, config settings.AiBird) bool {
	commands := GetAllCommandsUnfiltered(config)

	for _, cmd := range commands {
		if cmd == command {
			return true
		}
	}

	return false
}

// IsValidCommandForChannel checks if a command is valid for a specific channel with its capabilities
func IsValidCommandForChannel(command string, config settings.AiBird, enableAi, enableSd, enableSound, enableVideo bool, isAdmin, isOwner bool) bool {
	commands := GetAllCommands(config, enableAi, enableSd, enableSound, enableVideo, isAdmin, isOwner)

	for _, cmd := range commands {
		if cmd == command {
			return true
		}
	}

	return false
}

// IsStandardCommand checks if a command is in the list of standard commands
func IsStandardCommand(command string) bool {
	for _, cmd := range help.StandardHelp() {
		if cmd.Name == command {
			return true
		}
	}
	return false
}

// IsAdminCommand checks if a command is in the list of admin commands
func IsAdminCommand(command string) bool {
	for _, cmd := range help.AdminHelp() {
		if cmd.Name == command {
			return true
		}
	}
	return false
}

// IsOwnerCommand checks if a command is in the list of owner commands
func IsOwnerCommand(command string) bool {
	for _, cmd := range help.OwnerHelp() {
		if cmd.Name == command {
			return true
		}
	}
	return false
}

// IsSoundCommand checks if a command is in the list of sound commands
func IsSoundCommand(command string, config settings.AiBird) bool {
	for _, cmd := range help.SoundHelp(config) {
		if cmd.Name == command {
			return true
		}
	}
	return false
}

// IsVideoCommand checks if a command is in the list of video commands
func IsVideoCommand(command string, config settings.AiBird) bool {
	for _, cmd := range help.VideoHelp(config) {
		if cmd.Name == command {
			return true
		}
	}
	return false
}

// IsTextCommand checks if a command is a text command (from help.TextHelp)
func IsTextCommand(action string) bool {
	for _, cmd := range help.TextHelp() {
		if strings.EqualFold(action, cmd.Name) {
			return true
		}
	}
	return false
}