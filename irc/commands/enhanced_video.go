package commands

import (
	"fmt"
	"strings"
	"time"

	"aibird/birdbase"
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text"
	"aibird/text/llamacpp"

	meta "aibird/shared/meta"
)

// premiumVideoWorkflow is the ComfyUI workflow all enhanced-video style commands
// run. The MiniMax H3 model workflow is "h3" on disk (comfyuijson/h3.json).
const premiumVideoWorkflow = "h3"

// freeAccessChannel is the IRC channel where premium commands are free for
// everyone (a community/test channel). In all other channels the commands
// require Patreon-level access (see checkPremiumAccess).
const freeAccessChannel = "#h4x"

// chatroomFlag is the argument that switches an enhanced-video command into
// "chatroom" mode: fetch recent channel chat and recreate it in-theme.
const chatroomFlag = "--chatroom"

// chatroomFetchHours is the lookback window for chatroom chat history.
const chatroomFetchHours = 6

// chatroomFetchBuffer is how many raw messages to pull before filtering; we
// over-fetch because the bot's own lines and ignored nicks are dropped.
const chatroomFetchBuffer = 50

// chatroomMaxLines is the maximum number of filtered chat lines fed to the
// prompt enhancer.
const chatroomMaxLines = 30

// enhancedVideoTimeout is the per-item queue timeout for H3 video generation.
// H3 clips take roughly 8 minutes to render on the RTX 4090, so we allow 10.
const enhancedVideoTimeout = 10 * time.Minute

// enhancedVideoTemplates maps each enhanced-video command name to the prompt
// template file under text/prompts/ used to expand the user's idea via llama.cpp
// before running the h3 workflow. This is the single source of truth for which
// commands are premium enhanced-video commands and which template each loads.
var enhancedVideoTemplates = map[string]string{
	"h3":        "h3.md",
	"sienfield": "sienfield.md",
	"garfield":  "garfield.md",
	"spongebob": "spongebob.md",
	"simpsons":  "simpsons.md",
	"southpark": "southpark.md",
	"rick":      "rick.md",
	"looney":    "looney.md",
	"futurama":  "futurama.md",
	"adventure": "adventure.md",
	"familyguy": "familyguy.md",
	"tomjerry":  "tomjerry.md",
	"scooby":    "scooby.md",
	"koth":      "koth.md",
	"beavis":    "beavis.md",
}

// enhancedVideoTemplate returns the prompt template file for an enhanced-video
// command (case-insensitive) and whether the action is one of these commands.
func enhancedVideoTemplate(action string) (string, bool) {
	tpl, ok := enhancedVideoTemplates[strings.ToLower(action)]
	return tpl, ok
}

// isEnhancedVideoCommand reports whether action is one of the registered
// enhanced-video style commands (see enhancedVideoTemplates).
func isEnhancedVideoCommand(action string) bool {
	_, ok := enhancedVideoTemplate(action)
	return ok
}

// QueueItemTimeout returns the max runtime for a queued command. Enhanced-video
// commands (MiniMax H3 generation) take up to ~8 minutes, so they get an
// extended window. Returns 0 for other commands so the queue default applies.
func QueueItemTimeout(action string) time.Duration {
	if isEnhancedVideoCommand(action) {
		return enhancedVideoTimeout
	}
	return 0
}

// basePromptFile is the foundational H3 prompting-craft template prepended to
// every style overlay so all style commands share the same expert guidance.
const basePromptFile = "h3.md"

// loadStyleSystemPrompt builds the system prompt for an enhanced-video command.
// For the base command (h3.md) it returns the template as-is. For a style
// command it prepends the H3 base craft (h3.md) to the style overlay so the
// model gets both the expert prompting guidance and the theme DNA.
func loadStyleSystemPrompt(templateFile string) (string, error) {
	base, err := text.GetPrompt(basePromptFile)
	if err != nil {
		return "", fmt.Errorf("load base prompt %s: %w", basePromptFile, err)
	}
	if templateFile == basePromptFile {
		return base, nil
	}
	overlay, err := text.GetPrompt(templateFile)
	if err != nil {
		return "", fmt.Errorf("load style overlay %s: %w", templateFile, err)
	}
	return base + "\n\n---\n\n" + overlay, nil
}

// parseChatroomFlag detects the --chatroom flag in a message. When present it
// returns enabled=true, the optional location hint following the flag (e.g.
// "in the diner" from "!simpsons --chatroom in the diner"), and any remaining
// idea text that preceded the flag. Matching is case-insensitive on the flag
// token but the returned location/remaining preserve the original casing.
func parseChatroomFlag(message string) (enabled bool, location string, remaining string) {
	idx := strings.Index(strings.ToLower(message), chatroomFlag)
	if idx == -1 {
		return false, "", message
	}
	remaining = strings.TrimSpace(message[:idx])
	location = strings.TrimSpace(message[idx+len(chatroomFlag):])
	return true, location, remaining
}

// fetchChatroomLog returns up to chatroomMaxLines recent PRIVMSG lines from the
// command's channel, excluding the bot's own messages, ignored nicks, and blank
// lines. Messages are returned oldest-first (chronological order) so the dialog
// reads naturally to the enhancer.
func fetchChatroomLog(irc *state.State) ([]birdbase.ChannelMessage, error) {
	networkID := birdbase.ResolveNetworkID(irc.Network.NetworkName)
	if networkID == 0 {
		return nil, fmt.Errorf("network %q not found in database", irc.Network.NetworkName)
	}

	// Event types must be lowercase to match how StoreChannelMessage stores them
	// ("privmsg" / "action"). SQLite's default text comparison is case-sensitive.
	msgs, err := birdbase.GetChannelMessages(networkID, irc.Channel.Name, chatroomFetchHours, chatroomFetchBuffer, "privmsg", "action")
	if err != nil {
		return nil, fmt.Errorf("fetch channel messages: %w", err)
	}

	filtered := filterChatroomMessages(msgs, irc.Network.Nick, irc.Network.IsNickIgnored)
	logger.Debug("Chatroom log fetched", "channel", irc.Channel.Name,
		"raw_count", len(msgs), "filtered_count", len(filtered))
	return filtered, nil
}

// filterChatroomMessages drops the bot's own lines, ignored nicks, and blank
// messages, then trims to the most recent chatroomMaxLines (messages are assumed
// oldest-first). Pure (no I/O) so it can be tested without a database.
func filterChatroomMessages(msgs []birdbase.ChannelMessage, botNick string, isIgnored func(string) bool) []birdbase.ChannelMessage {
	var filtered []birdbase.ChannelMessage
	for _, m := range msgs {
		if botNick != "" && strings.EqualFold(m.Nickname, botNick) {
			continue
		}
		if isIgnored != nil && isIgnored(m.Nickname) {
			continue
		}
		if strings.TrimSpace(m.Message) == "" {
			continue
		}
		filtered = append(filtered, m)
	}

	// Keep only the most recent chatroomMaxLines (messages are oldest-first).
	if len(filtered) > chatroomMaxLines {
		filtered = filtered[len(filtered)-chatroomMaxLines:]
	}
	return filtered
}

// buildChatroomMessage assembles the user message handed to the prompt enhancer
// when --chatroom is active. It frames the recent chat log as a scene to
// recreate in-theme: each real chatter becomes a character fitting the style,
// the conversation's gist is preserved, and the optional location sets the
// scene. extra (any idea text before the flag) is appended as guidance.
func buildChatroomMessage(chatLog []birdbase.ChannelMessage, location, extra string) string {
	var b strings.Builder
	b.WriteString("Recreate the following IRC chat as a scene in this video style. ")
	b.WriteString("Convert each chatter below into a character fitting this style, mapping distinct nicks to distinct characters. ")
	b.WriteString("Keep the gist and rhythm of what each person actually said, adapted to those characters' voices — do not invent entirely new topics. ")
	b.WriteString("Then render the full scene as your standard MiniMax H3 video prompt, with the dialog included word-for-word.\n\n")

	if location != "" {
		fmt.Fprintf(&b, "Setting: %s\n\n", location)
	}
	if extra != "" {
		fmt.Fprintf(&b, "Additional direction: %s\n\n", extra)
	}

	b.WriteString("Recent chat (oldest first):\n")
	for _, m := range chatLog {
		fmt.Fprintf(&b, "<%s>: %s\n", m.Nickname, m.Message)
	}
	return b.String()
}

// hasPremiumAccess reports whether the current user may run a Patreon-gated
// command. Access is granted in the free community channel (#h4x) for everyone,
// or anywhere when the user holds premium access (access level >= 2 / admin /
// owner). This is the pure boolean form (no IRC side effects) for testability.
func hasPremiumAccess(irc *state.State) bool {
	if irc.Channel != nil && strings.EqualFold(irc.Channel.Name, freeAccessChannel) {
		return true
	}
	return irc.User != nil && irc.User.CanUsePremiumGPU()
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

// ParseEnhancedVideo handles an enhanced-video style command (non-queued entry
// point, kept for symmetry with the other Parse* commands).
func ParseEnhancedVideo(irc *state.State) bool {
	tpl, ok := enhancedVideoTemplate(irc.Action())
	if !ok {
		return false
	}
	return parseEnhancedVideoCommand(*irc, tpl, meta.GPU4090)
}

// ParseEnhancedVideoWithGPU handles an enhanced-video command with an explicit
// GPU selection (queued path).
func ParseEnhancedVideoWithGPU(irc state.State, gpu meta.GPUType) bool {
	tpl, ok := enhancedVideoTemplate(irc.Action())
	if !ok {
		return false
	}
	return parseEnhancedVideoCommand(irc, tpl, gpu)
}

// parseEnhancedVideoCommand expands the user's video idea using the named
// template via llama.cpp, then runs the ComfyUI h3 workflow with the expanded
// prompt. Patreon access (#h4x-or-premium) is checked before any expensive work.
// Workflow-level accessLevel inside comfyui.Process acts as a final safety net.
func parseEnhancedVideoCommand(irc state.State, templateFile string, gpu meta.GPUType) bool {
	// The h3 workflow may not be deployed yet — fail fast with a clear message.
	if !comfyui.WorkflowExists(premiumVideoWorkflow) {
		irc.SendError("The video workflow (h3) is not available yet")
		return false
	}

	// Patreon gate (with #h4x free exception) — before the LLM call.
	if !checkPremiumAccess(&irc) {
		return false
	}

	// Prompt enhancement needs llama.cpp; bail early if it's offline.
	if !checkLlamaCppAvailable(&irc) {
		return false
	}

	rawMessage := comfyui.CleanPrompt(irc.Message())
	if rawMessage == "" {
		irc.Send("Usage: !<command> <your video idea>")
		return false
	}

	// Instant acknowledgment so the user knows the command was received before
	// the (slow) prompt enhancement and ~8-minute GPU render begin.
	irc.Send(fmt.Sprintf("%s: Generating H3 prompt — video generation can take up to 10 minutes, please wait.", irc.User.NickName))

	// The message handed to the enhancer. In --chatroom mode this is replaced
	// with a themed recreation of recent channel chat.
	message := rawMessage
	if enabled, location, extra := parseChatroomFlag(rawMessage); enabled {
		chatLog, err := fetchChatroomLog(&irc)
		if err != nil {
			logger.Error("Chatroom history fetch failed", "error", err)
			irc.SendError("Failed to read channel history: " + err.Error())
			return false
		}
		if len(chatLog) == 0 {
			irc.SendError("No recent chat found in " + irc.Channel.Name + " to recreate")
			return false
		}
		message = buildChatroomMessage(chatLog, location, extra)
	}

	// Build the system prompt: the H3 base craft, plus a style overlay for
	// themed commands (e.g. !simpsons = h3.md + simpsons.md).
	systemPrompt, err := loadStyleSystemPrompt(templateFile)
	if err != nil {
		logger.Error("Enhanced video system prompt load failed", "template", templateFile, "error", err)
		irc.SendError("Failed to load prompt template: " + err.Error())
		return false
	}

	enhanced, err := llamacpp.EnhancePromptWithSystemPrompt(message, systemPrompt, irc.Config.LlamaCpp)
	if err != nil {
		logger.Error("Enhanced video prompt enhancement failed", "template", templateFile, "error", err)
		irc.SendError("Failed to enhance prompt: " + err.Error())
		return false
	}
	if strings.TrimSpace(enhanced) == "" {
		irc.SendError("Prompt enhancement returned an empty result")
		return false
	}

	// Build the display message (what the user sees) from the original input,
	// honoring the bad-words filter. Uses rawMessage (not the expanded chatroom
	// prompt) so the user sees their own command text.
	displayMessage := rawMessage
	if comfyui.BadWordsCheck(rawMessage, &irc.Config.ComfyUi) {
		displayMessage = irc.Config.ComfyUi.BadWordsPrompt
	}

	// Notify the user that processing has started (before the GPU work).
	if irc.User.CanUsePremiumGPU() {
		irc.Send(fmt.Sprintf("%s: Birdnest pal! Enjoy the 🔥rtx %s🔥 processing '%s'... please wait.", irc.User.NickName, gpu, displayMessage))
	} else {
		irc.Send(fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", irc.User.NickName, displayMessage))
	}

	// Override the action so comfyui.Process loads h3.json (the MiniMax workflow).
	// irc is a value copy local to this function; mutating it here does not affect
	// the caller's dispatch routing (which already matched on the command name).
	irc.Command.Action = premiumVideoWorkflow

	response, err := comfyui.Process(&irc, enhanced, gpu)
	if err != nil {
		logger.Error("Enhanced video ComfyUI request failed", "error", err)
		irc.SendError(err.Error())
		return false
	}

	return uploadAndReply(&irc, response, displayMessage, enhanced)
}
