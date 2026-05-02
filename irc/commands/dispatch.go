package commands

import (
	"context"
	"strings"

	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/shared/meta"
)

// IsQueueableFromHelp checks if a command is queueable using the command registry.
func IsQueueableFromHelp(action string) bool {
	// O(1) registry lookup
	entry := getRegistry().lookup(action)
	if entry != nil {
		logger.Debug("Found command in registry", "action", action, "queueable", entry.Queueable)
		return entry.Queueable
	}

	// If not found in registry, check if it's a ComfyUI workflow
	// All ComfyUI workflows are assumed to be queueable
	for _, workflow := range comfyui.GetCachedWorkflows() {
		if strings.EqualFold(action, workflow) {
			logger.Debug("Found ComfyUI workflow", "action", action, "queueable", true)
			return true
		}
	}

	logger.Debug("Command not found in registry or workflows", "action", action, "queueable", false)
	return false
}

// IsQueueableCommand checks if a command should be queued.
func IsQueueableCommand(s state.State) bool {
	action := s.Action()
	if action == "" {
		logger.Debug("IsQueueableCommand: action is empty")
		return false
	}

	logger.Debug("IsQueueableCommand: checking action", "action", action)

	isQueueable := IsQueueableFromHelp(action)

	logger.Debug("IsQueueableCommand: result", "action", action, "queueable", isQueueable)
	return isQueueable
}

// RunQueueableCommand runs a command that has been taken from the queue.
// It routes to the existing handlers that already have upload functionality.
// The ctx parameter allows cancellation on timeout or queue shutdown.
func RunQueueableCommand(ctx context.Context, s state.State, gpu meta.GPUType) {
	actionLower := strings.ToLower(s.Action())

	logger.Debug("Routing queue command", "action", s.Action(), "actionLower", actionLower)

	// Route based on the command action to existing handlers
	switch {
	case IsTextCommand(actionLower):
		logger.Debug("Command categorized as text", "action", s.Action())
		ParseAiText(s)
	case isImageCommand(actionLower):
		logger.Debug("Command categorized as image", "action", s.Action())
		ParseAiImageWithGPU(s, gpu)
	case isVideoCommand(actionLower):
		logger.Debug("Command categorized as video", "action", s.Action())
		ParseAiVideoWithGPU(s, gpu)
	case isSoundCommand(actionLower):
		logger.Debug("Command categorized as sound", "action", s.Action())
		ParseAiSoundWithGPU(s, gpu)
	default:
		logger.Debug("Command categorized as default (image)", "action", s.Action())
		ParseAiImageWithGPU(s, gpu)
	}
}

// Helper functions to categorize commands based on registry and workflow metadata

func isImageCommand(action string) bool {
	entry := getRegistry().lookup(action)
	if entry != nil && entry.Type == "image" {
		return true
	}
	meta := comfyui.GetCachedMeta(action)
	if meta != nil && meta.Type == "image" {
		return true
	}
	return false
}

func isVideoCommand(action string) bool {
	entry := getRegistry().lookup(action)
	if entry != nil && entry.Type == "video" {
		return true
	}
	meta := comfyui.GetCachedMeta(action)
	if meta != nil && meta.Type == "video" {
		return true
	}
	return false
}

func isSoundCommand(action string) bool {
	entry := getRegistry().lookup(action)
	if entry != nil && entry.Type == "sound" {
		return true
	}
	meta := comfyui.GetCachedMeta(action)
	if meta != nil && meta.Type == "sound" {
		return true
	}
	return false
}
