package commands

import (
	"context"
	"fmt"

	"aibird/birdbase"
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/shared/meta"
	"aibird/text"
)

// ParseVibe generates a visual representation of the channel's current vibe.
// It uses the AI provider (LlamaCpp with GLM fallback) to analyze recent
// conversation and produce a Z-Image prompt, then sends that prompt through
// the ComfyUI zimage workflow.
func ParseVibe(irc state.State, q *queue.ProcessingQueue) {
	if irc.Channel == nil {
		irc.Send("Error: vibe can only be used in a channel.")
		return
	}

	if !irc.Channel.Sd {
		irc.Send("Error: image generation is disabled in this channel.")
		return
	}

	if !hasTextProviderConfig(irc.Config) {
		irc.Send("Error: no AI provider available. Configure LlamaCpp or GLM.")
		return
	}

	if !comfyui.WorkflowExists("zimage") {
		irc.Send("Error: zimage workflow not found. Cannot generate vibe image.")
		return
	}

	if irc.Message() == "--help" {
		irc.Send("Usage: !vibe [hours] — Generates a visual artwork capturing the channel's vibe.")
		irc.Send("The bot analyzes recent conversation, creates an AI image prompt, and generates an image.")
		irc.Send("Example: !vibe, !vibe 6h")
		return
	}

	hours := 6
	if irc.Message() != "" {
		hours = parseHoursFromMessage(irc.Message(), 6, 48)
	}

	maxMessages := irc.Config.AiBird.VibeMaxMessages
	if maxMessages <= 0 {
		maxMessages = 100
	}

	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		irc.Send("Error: network not found in database.")
		return
	}

	messages, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, hours, maxMessages)
	if err != nil {
		logger.Error("Failed to get channel messages for vibe", "error", err)
		irc.Send("Error retrieving channel history.")
		return
	}

	if len(messages) == 0 {
		irc.Send(fmt.Sprintf("No activity found in %s over the last %dh.", irc.Channel.Name, hours))
		return
	}

	// Queue the entire vibe pipeline — both AI text gen and ComfyUI image gen use the GPU
	queueItem := queue.QueueItem{
		Item: queue.Item{
			State: irc,
			Function: func(ctx context.Context, s state.State, gpu meta.GPUType) {
				generateVibe(s, messages, hours)
			},
		},
		Model: "vibe",
		User:  irc.User,
		GPU:   meta.GPU4090,
	}

	msg, err := q.Enqueue(queueItem)
	if err != nil {
		irc.SendError(err.Error())
	} else if msg != "" {
		irc.Send(msg)
	}
}

// generateVibe runs the two-step pipeline in a goroutine:
//  1. The AI provider analyzes the conversation log and produces a Z-Image prompt.
//  2. ComfyUI processes the prompt via the zimage workflow and uploads the result.
func generateVibe(irc state.State, messages []birdbase.ChannelMessage, hours int) {
	irc.Send(fmt.Sprintf("%s, reading the room for the last %dh and painting the vibe...", irc.User.NickName, hours))

	// Step 1: Use GLM to generate a Z-Image prompt from the conversation.
	systemPrompt, err := text.GetPrompt("vibe.md")
	if err != nil {
		logger.Error("Failed to load vibe prompt", "error", err)
		irc.Send("Error: could not load vibe prompt.")
		return
	}

	userPrompt := formatEventLog(irc.Channel.Name, hours, messages)

	imagePrompt, err := singleRequestWithFallback(systemPrompt, userPrompt, irc.Config)
	if err != nil {
		logger.Error("Failed to generate vibe prompt", "error", err)
		irc.Send("Error: failed to analyze the channel vibe. Please try again later.")
		return
	}

	if imagePrompt == "" {
		irc.Send("Error: AI returned an empty prompt. Please try again.")
		return
	}

	// Check bad words before proceeding — abort if the AI-generated prompt violates content policy.
	if comfyui.BadWordsCheck(imagePrompt, irc.Config.ComfyUi) {
		logger.Warn("Vibe prompt blocked by content filter")
		irc.Send("Error: the generated prompt was blocked by content filters. Please try again.")
		return
	}

	logger.Info("Vibe prompt generated", "length", len(imagePrompt))

	// Step 2: Send the generated prompt through the ComfyUI zimage pipeline.
	// Use a separate state copy for ComfyUI so the original action ("vibe") is preserved
	// for correct Birdhole tags and IRC reply formatting in uploadAndReply.
	comfyState := irc
	comfyState.Command.Action = "zimage"
	comfyState.Command.Message = imagePrompt

	if irc.User.CanUsePremiumGPU() {
		irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx %s🔥 painting the vibe... please wait.", irc.User.NickName, meta.GPU4090))
	} else {
		irc.Send(fmt.Sprintf("%s: Vibe image is being generated... please wait.", irc.User.NickName))
	}

	response, err := comfyui.Process(comfyState, "", meta.GPU4090)
	if err != nil {
		logger.Error("ComfyUI vibe generation failed", "error", err)
		irc.SendError(err.Error())
		return
	}

	uploadAndReply(irc, response, imagePrompt, "")
}
