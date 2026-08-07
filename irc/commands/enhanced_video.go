package commands

import (
	"context"
	"fmt"
	"strings"

	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
)

// premiumVideoWorkflow is the ComfyUI workflow the h3 video command runs. The
// MiniMax H3 model workflow is "h3" on disk (comfyuijson/h3.json).
const premiumVideoWorkflow = "h3"

// freeAccessChannel is the IRC channel where premium commands are free for
// everyone (a community/test channel). In all other channels the command
// requires Patreon-level access (see checkPremiumAccess).
const freeAccessChannel = "#h4x"

// isEnhancedVideoCommand reports whether action is the h3 premium video
// command (case-insensitive). Kept as a predicate so dispatch and help routing
// can treat it as a distinct, queueable video command with an extended runtime.
func isEnhancedVideoCommand(action string) bool {
	return strings.EqualFold(action, premiumVideoWorkflow)
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

// ParseEnhancedVideo runs the ComfyUI h3 workflow with the user's raw prompt
// (no LLM expansion). Patreon access (#h4x-or-premium) is checked before any
// expensive work. Workflow-level accessLevel inside comfyui.Process acts as a
// final safety net. ctx allows cancellation on queue timeout/shutdown;
// non-queued callers pass context.Background().
func ParseEnhancedVideo(ctx context.Context, irc state.State) bool {
	if !isEnhancedVideoCommand(irc.Action()) {
		return false
	}

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
		irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx 4090🔥 processing '%s'... please wait.", irc.User.NickName, displayMessage))
	} else {
		irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, displayMessage))
	}

	// Override the action so comfyui.Process loads h3.json (the MiniMax workflow).
	// irc is a value copy local to this function; mutating it here does not affect
	// the caller's dispatch routing (which already matched on the command name).
	irc.Command.Action = premiumVideoWorkflow

	response, err := comfyui.Process(ctx, &irc, "")
	if err != nil {
		logger.Error("Enhanced video ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(&irc, response, displayMessage)
}
