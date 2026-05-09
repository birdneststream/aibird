package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"aibird/logger"
	"aibird/settings"
)

// ExecuteTool dispatches a tool call to the appropriate executor based on the
// tool name. It parses the JSON arguments, calls the right executor, and
// returns the result as a string. On error, it returns the error message as
// content (not a Go error) so the LLM can decide how to handle the failure.
func ExecuteTool(ctx context.Context, name, argsJSON string, config *settings.McpConfig) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		logger.Warn("Failed to parse tool arguments", "tool", name, "error", err)
		return fmt.Sprintf("Error: invalid arguments format: %v", err)
	}

	switch name {
	case ToolNameWebSearch:
		query, _ := args["query"].(string) //nolint:errcheck // type assertion with ok idiom
		if query == "" {
			return "Error: missing required parameter 'query'"
		}
		result, err := SearchWeb(ctx, query, config.SearXNG)
		if err != nil {
			logger.Warn("SearXNG search failed", "error", err)
			return fmt.Sprintf("Error performing web search: %v", err)
		}
		return result

	case ToolNameScrapeURL:
		targetURL, _ := args["url"].(string) //nolint:errcheck // type assertion with ok idiom
		if targetURL == "" {
			return "Error: missing required parameter 'url'"
		}
		result, err := ScrapeURL(ctx, targetURL, config.Firecrawl)
		if err != nil {
			logger.Warn("Firecrawl scrape failed", "error", err)
			return fmt.Sprintf("Error scraping URL: %v", err)
		}
		return result

	default:
		logger.Warn("Unknown tool called", "tool", name)
		return fmt.Sprintf("Error: unknown tool %q", name)
	}
}
