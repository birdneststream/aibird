package commands

import (
	"aibird/image/comfyui"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"
	"aibird/shared/meta"
	"strings"
)

// IsQueueableFromHelp checks if a command is queueable using the help system
func IsQueueableFromHelp(action string, config settings.AiBird) bool {
	// Check all help categories for the command
	allHelp := []help.Help{}

	// Add all help categories
	allHelp = append(allHelp, help.StandardHelp()...)
	allHelp = append(allHelp, help.ImageHelp(config)...)
	allHelp = append(allHelp, help.VideoHelp(config)...)
	allHelp = append(allHelp, help.TextHelp()...)
	allHelp = append(allHelp, help.SoundHelp(config)...)
	allHelp = append(allHelp, help.AdminHelp()...)
	allHelp = append(allHelp, help.OwnerHelp()...)

	// Look for the command in the help system
	for _, cmd := range allHelp {
		if strings.EqualFold(action, cmd.Name) {
			logger.Debug("Found command in help system", "action", action, "queueable", cmd.Queueable)
			return cmd.Queueable
		}
	}

	// If not found in help system, check if it's a ComfyUI workflow
	// All ComfyUI workflows are assumed to be queueable
	workflows := comfyui.GetWorkFlowsSlice()
	for _, workflow := range workflows {
		if strings.EqualFold(action, workflow) {
			logger.Debug("Found ComfyUI workflow", "action", action, "queueable", true)
			return true // All ComfyUI workflows are queueable
		}
	}

	logger.Debug("Command not found in help system or workflows", "action", action, "queueable", false)
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

	// Use the help system to determine if command is queueable
	config := s.Config.AiBird
	isQueueable := IsQueueableFromHelp(action, config)

	logger.Debug("IsQueueableCommand: result", "action", action, "queueable", isQueueable)
	return isQueueable
}

// RunQueueableCommand runs a command that has been taken from the queue.
// It routes to the existing handlers that already have upload functionality.
func RunQueueableCommand(s state.State, gpu meta.GPUType) {
	actionLower := strings.ToLower(s.Action())

	logger.Debug("Routing queue command", "action", s.Action(), "actionLower", actionLower)

	// Route based on the command action to existing handlers
	// Prioritize text commands over image commands since they are more specific
	switch {
	case IsTextCommand(actionLower):
		logger.Debug("Command categorized as text", "action", s.Action())
		// Use existing ParseAiText which already has upload functionality
		ParseAiText(s)
	case isImageCommand(actionLower, s.Config.AiBird):
		logger.Debug("Command categorized as image", "action", s.Action())
		// Use existing ParseAiImageWithGPU which accepts GPU parameter
		ParseAiImageWithGPU(s, gpu)
	case isVideoCommand(actionLower, s.Config.AiBird):
		logger.Debug("Command categorized as video", "action", s.Action())
		// Use existing ParseAiVideoWithGPU which accepts GPU parameter
		ParseAiVideoWithGPU(s, gpu)
	case isSoundCommand(actionLower, s.Config.AiBird):
		logger.Debug("Command categorized as sound", "action", s.Action())
		// Use existing ParseAiSoundWithGPU which accepts GPU parameter
		ParseAiSoundWithGPU(s, gpu)
	default:
		logger.Debug("Command categorized as default (image)", "action", s.Action())
		// Fallback for custom workflows - use image handler with GPU
		ParseAiImageWithGPU(s, gpu)
	}
}

// Helper functions to categorize commands based on help system
func isImageCommand(action string, config settings.AiBird) bool {
	// Get image commands from help system FIRST
	imageHelp := help.ImageHelp(config)
	for _, cmd := range imageHelp {
		if strings.EqualFold(action, cmd.Name) {
			return true
		}
	}

	// Only then check if it's a ComfyUI workflow with image/video type
	workflows := comfyui.GetWorkFlowsSlice()
	for _, workflow := range workflows {
		if strings.EqualFold(action, workflow) {
			workflowFile := "comfyuijson/" + workflow + ".json"
			meta, err := comfyui.GetAibirdMeta(workflowFile)
			if err == nil && meta != nil {
				return meta.Type == "image"
			}
		}
	}
	return false
}

func isVideoCommand(action string, config settings.AiBird) bool {
	// Get video commands from help system FIRST
	videoHelp := help.VideoHelp(config)
	for _, cmd := range videoHelp {
		if strings.EqualFold(action, cmd.Name) {
			return true
		}
	}

	// Only then check if it's a ComfyUI workflow with image/video type
	workflows := comfyui.GetWorkFlowsSlice()
	for _, workflow := range workflows {
		if strings.EqualFold(action, workflow) {
			workflowFile := "comfyuijson/" + workflow + ".json"
			meta, err := comfyui.GetAibirdMeta(workflowFile)
			if err == nil && meta != nil {
				return meta.Type == "video"
			}
		}
	}
	return false
}

func isSoundCommand(action string, config settings.AiBird) bool {
	// Get sound commands from help system FIRST
	soundHelp := help.SoundHelp(config)
	for _, cmd := range soundHelp {
		if strings.EqualFold(action, cmd.Name) {
			return true
		}
	}

	// Only then check if it's a ComfyUI workflow with sound type
	workflows := comfyui.GetWorkFlowsSlice()
	for _, workflow := range workflows {
		if strings.EqualFold(action, workflow) {
			workflowFile := "comfyuijson/" + workflow + ".json"
			meta, err := comfyui.GetAibirdMeta(workflowFile)
			if err == nil && meta != nil {
				return meta.Type == "sound"
			}
		}
	}
	return false
}