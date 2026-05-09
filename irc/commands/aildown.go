package commands

import (
	"context"
	"fmt"

	"aibird/irc/state"
	"aibird/logger"
	"aibird/mcp"
	"aibird/shared/meta"

	"github.com/lrstanley/girc"
)

// ShouldQueueAilDown checks if an !aildown command needs to be queued.
// Returns true if the command is aildown with MCP enabled and a non-empty message.
func ShouldQueueAilDown(irc *state.State) bool {
	if !irc.IsAction("aildown") {
		return false
	}

	if irc.IsEmptyMessage() {
		return false
	}

	// Must have MCP enabled
	if !irc.Config.Mcp.Enabled {
		return false
	}

	return true
}

// ParseAilDown handles the !aildown command — AI with web search and URL
// scraping tools. The LLM autonomously decides when to use SearXNG for
// search and Firecrawl for page scraping.
func ParseAilDown(irc *state.State) bool {
	if !irc.IsAction("aildown") {
		return false
	}

	// Check if MCP is enabled and configured
	if !irc.Config.Mcp.Enabled {
		irc.SendError("🔍 MCP tool calling is not enabled. Configure mcp.toml to enable.")
		return true
	}

	if irc.Config.Mcp.SearXNG.Url == "" {
		irc.SendError("🔍 SearXNG is not configured. Set searxng URL in mcp.toml.")
		return true
	}

	// Handle empty message (show help hint)
	if irc.IsEmptyMessage() {
		irc.Send("Usage: !aildown <your question> — Ask anything and the AI will search the web if needed.")
		return true
	}

	// Check llama.cpp availability
	if !checkLlamaCppAvailable(irc) {
		return true
	}

	irc.ReplyTo(girc.Fmt("🔍 Processing AI with tools request, please wait..."))

	response, err := mcp.ChatWithTools(irc)
	if err != nil {
		logger.Error("Error processing aildown request", "error", err)
		irc.SendError(fmt.Sprintf("🔍 Error: %s", err))
	} else {
		logger.Debug("Calling handleAiResponse from aildown path")
		handleAiResponse(irc, response)
	}

	return true
}

// ProcessAilDownRequest handles aildown requests from the queue processor.
// ctx and gpu are retained for queue-processor API compatibility but are not
// used by the underlying handler.
func ProcessAilDownRequest(_ context.Context, irc state.State, _ meta.GPUType) {
	ParseAilDown(&irc)
}
