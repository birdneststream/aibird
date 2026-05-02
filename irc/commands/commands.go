package commands

import (
	"sort"
	"strings"
	"sync"

	"aibird/irc/commands/help"
	"aibird/logger"
	"aibird/settings"
)

// CommandEntry holds metadata about a single command for O(1) lookups.
type CommandEntry struct {
	Name      string // Original case from help definition
	Type      string // "standard", "admin", "owner", "image", "video", "sound", "text"
	Queueable bool
}

// CommandRegistry provides O(1) command lookups by name and type.
// Built once via InitRegistry() from all help categories.
type CommandRegistry struct {
	byLower map[string]*CommandEntry // lowercase name → entry
}

var (
	cachedRegistry *CommandRegistry
	registryOnce   sync.Once
	registryMu     sync.RWMutex
)

// InitRegistry builds and caches the command registry. Call at startup with
// the actual config to include ComfyUI workflow commands. Safe to call multiple
// times — each call replaces the previous registry.
func InitRegistry(config settings.AiBird) {
	r := &CommandRegistry{
		byLower: make(map[string]*CommandEntry),
	}

	addAll := func(cmds []help.Help, cmdType string) {
		for i := range cmds {
			if cmds[i].Name == "" {
				continue
			}
			key := strings.ToLower(cmds[i].Name)
			if existing, exists := r.byLower[key]; exists {
				logger.Warn("Command registry collision",
					"command", cmds[i].Name,
					"existing_type", existing.Type,
					"new_type", cmdType)
			}
			r.byLower[key] = &CommandEntry{
				Name:      cmds[i].Name,
				Type:      cmdType,
				Queueable: cmds[i].Queueable,
			}
		}
	}

	addAll(help.StandardHelp(), "standard")
	addAll(help.AdminHelp(), "admin")
	addAll(help.OwnerHelp(), "owner")
	addAll(help.TextHelp(), "text")
	addAll(help.ImageHelp(config), "image")
	addAll(help.VideoHelp(config), "video")
	addAll(help.SoundHelp(config), "sound")

	registryMu.Lock()
	cachedRegistry = r
	registryMu.Unlock()
}

// getRegistry returns the cached registry, building a zero-config one if needed.
// Thread-safe via sync.Once for the lazy init path.
func getRegistry() *CommandRegistry {
	registryOnce.Do(func() {
		if cachedRegistry == nil {
			InitRegistry(settings.AiBird{})
		}
	})
	registryMu.RLock()
	r := cachedRegistry
	registryMu.RUnlock()
	return r
}

// lookup finds a command entry by name (case-insensitive).
func (r *CommandRegistry) lookup(command string) *CommandEntry {
	return r.byLower[strings.ToLower(command)]
}

// GetAllCommands returns a sorted slice of available command names filtered by capabilities.
func GetAllCommands(enableAi, enableSd, enableSound, enableVideo bool, isAdmin, isOwner bool) []string {
	r := getRegistry()
	var result []string
	for _, entry := range r.byLower {
		switch entry.Type {
		case "standard":
			result = append(result, entry.Name)
		case "admin":
			if isAdmin {
				result = append(result, entry.Name)
			}
		case "owner":
			if isOwner {
				result = append(result, entry.Name)
			}
		case "text":
			if enableAi {
				result = append(result, entry.Name)
			}
		case "image":
			if enableSd {
				result = append(result, entry.Name)
			}
		case "video":
			if enableVideo {
				result = append(result, entry.Name)
			}
		case "sound":
			if enableSound {
				result = append(result, entry.Name)
			}
		}
	}
	sort.Strings(result)
	return result
}

// GetAllCommandsUnfiltered returns all commands regardless of channel capabilities.
func GetAllCommandsUnfiltered() []string {
	return GetAllCommands(true, true, true, true, true, true)
}

// IsValidCommand checks if a command exists in the registry.
// Case-sensitive: uses exact name match against help definitions.
func IsValidCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Name == command
}

// IsValidCommandForChannel checks if a command is valid for a specific channel.
// Case-sensitive exact match, filtered by channel capabilities.
func IsValidCommandForChannel(command string, enableAi, enableSd, enableSound, enableVideo bool, isAdmin, isOwner bool) bool {
	entry := getRegistry().lookup(command)
	if entry == nil || entry.Name != command {
		return false
	}
	switch entry.Type {
	case "standard":
		return true
	case "admin":
		return isAdmin
	case "owner":
		return isOwner
	case "text":
		return enableAi
	case "image":
		return enableSd
	case "video":
		return enableVideo
	case "sound":
		return enableSound
	default:
		return false
	}
}

// IsStandardCommand checks if a command is in the list of standard commands (case-sensitive).
func IsStandardCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Type == "standard" && entry.Name == command
}

// IsAdminCommand checks if a command is in the list of admin commands (case-sensitive).
func IsAdminCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Type == "admin" && entry.Name == command
}

// IsOwnerCommand checks if a command is in the list of owner commands (case-sensitive).
func IsOwnerCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Type == "owner" && entry.Name == command
}

// IsSoundCommand checks if a command is in the list of sound commands (case-insensitive).
func IsSoundCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Type == "sound"
}

// IsVideoCommand checks if a command is in the list of video commands (case-insensitive).
func IsVideoCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Type == "video"
}

// IsTextCommand checks if a command is a text command (case-insensitive).
func IsTextCommand(action string) bool {
	entry := getRegistry().lookup(action)
	return entry != nil && entry.Type == "text"
}

// IsImageCommand checks if a command is an image generation command (case-insensitive).
func IsImageCommand(command string) bool {
	entry := getRegistry().lookup(command)
	return entry != nil && entry.Type == "image"
}
