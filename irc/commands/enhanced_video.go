package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"

	meta "aibird/shared/meta"
)

// premiumVideoWorkflow is the ComfyUI workflow the h3 video command runs. The
// MiniMax H3 model workflow is "h3" on disk (comfyuijson/h3.json).
const premiumVideoWorkflow = "h3"

// freeAccessChannel is the IRC channel where premium commands are free for
// everyone (a community/test channel). In all other channels the command
// requires Patreon-level access (see checkPremiumAccess).
const freeAccessChannel = "#h4x"

// enhancedVideoTimeout is the per-item queue timeout for H3 video generation.
// H3 clips take roughly 8 minutes to render on the RTX 4090, so we allow 10.
const enhancedVideoTimeout = 10 * time.Minute

// isEnhancedVideoCommand reports whether action is the h3 premium video
// command (case-insensitive). Kept as a predicate so dispatch and help routing
// can treat it as a distinct, queueable video command with an extended runtime.
func isEnhancedVideoCommand(action string) bool {
	return strings.EqualFold(action, premiumVideoWorkflow)
}

// QueueItemTimeout returns the max runtime for a queued command. The h3 video
// command (MiniMax H3 generation) takes up to ~8 minutes, so it gets an
// extended window. Returns 0 for other commands so the queue default applies.
func QueueItemTimeout(action string) time.Duration {
	if isEnhancedVideoCommand(action) {
		return enhancedVideoTimeout
	}
	return 0
}

// hasPremiumAccess reports whether the current user may run a Patreon-gated
// command. Access is granted in the free community channel (#h4x) for everyone,
// or anywhere when the user holds premium access (access level >= 2 / admin /
// owner). This is the pure boolean form (no IRC side effects) for testability.
func hasPremiumAccess(irc *state.State) bool {
	if irc.Channel != nil && strings.EqualFold(irc.Channel.Name, freeAccessChannel) {
		return true
	}
	return irc.User != nil && irc.User.CanSkipQueue()
}

// checkPremiumAccess is the guard used by premium commands. Returns true when
// access is allowed; otherwise sends an explanatory error to the channel/user
// and returns false.
func checkPremiumAccess(irc *state.State) bool {
	if hasPremiumAccess(irc) {
		return true
	}
	irc.SendError("✨ This command is for Patreon supporters (Swan Squadron+). Try it in #h4x or see !support")
	return false
}

// ParseEnhancedVideo handles the h3 video command (non-queued entry point,
// kept for symmetry with the other Parse* commands).
func ParseEnhancedVideo(irc *state.State) bool {
	if !isEnhancedVideoCommand(irc.Action()) {
		return false
	}
	return parseEnhancedVideoCommand(context.Background(), *irc, meta.GPU4090)
}

// ParseEnhancedVideoWithGPU handles the h3 video command with an explicit GPU
// selection (queued path).
func ParseEnhancedVideoWithGPU(ctx context.Context, irc state.State, gpu meta.GPUType) bool {
	if !isEnhancedVideoCommand(irc.Action()) {
		return false
	}
	return parseEnhancedVideoCommand(ctx, irc, gpu)
}

// parseEnhancedVideoCommand runs the ComfyUI h3 workflow with the user's raw
// prompt (no LLM expansion). Patreon access (#h4x-or-premium) is checked before
// any expensive work. Workflow-level accessLevel inside comfyui.Process acts as
// a final safety net.
func parseEnhancedVideoCommand(ctx context.Context, irc state.State, gpu meta.GPUType) bool {
	// The h3 workflow may not be deployed yet — fail fast with a clear message.
	if !comfyui.WorkflowExists(premiumVideoWorkflow) {
		irc.SendError("The video workflow (h3) is not available yet")
		return false
	}

	// Patreon gate (with #h4x free exception) — before the GPU work.
	if !checkPremiumAccess(&irc) {
		return false
	}

	message := comfyui.CleanPrompt(irc.Message())
	if message == "" {
		irc.Send("Usage: !h3 <your video idea>")
		return false
	}

	// Build the display message (what the user sees), honoring the bad-words
	// filter.
	displayMessage := message
	if comfyui.BadWordsCheck(message, &irc.Config.ComfyUi) {
		displayMessage = irc.Config.ComfyUi.BadWordsPrompt
	}

	// Instant acknowledgment so the user knows the command was received before
	// the ~8-minute GPU render begins.
	if irc.User.CanSkipQueue() {
		irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx %s🔥 processing '%s'... please wait.", irc.User.NickName, gpu, displayMessage))
	} else {
		irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, displayMessage))
	}

	// Override the action so comfyui.Process loads h3.json (the MiniMax workflow).
	// irc is a value copy local to this function; mutating it here does not affect
	// the caller's dispatch routing (which already matched on the command name).
	irc.Command.Action = premiumVideoWorkflow

	response, err := comfyui.Process(ctx, &irc, "", gpu)
	if err != nil {
		logger.Error("Enhanced video ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(&irc, response, displayMessage)
}
