// Package mcp provides managed capability providers for LLM tool calling.
// The name "mcp" stands for "managed capability providers" — this is NOT the
// Model Context Protocol specification, but a tool-calling integration layer
// that gives the LLM access to external tools like web search and URL scraping.
package mcp

import (
	"encoding/json"

	"aibird/text/llamacpp"
)

// Tool represents a tool definition in the OpenAI tool calling format.
type Tool struct {
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function tool.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolCall represents a tool call returned by the LLM in its response.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // always "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the function name and arguments for a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string of arguments
}

// McpRequest embeds LlamaCppRequest and adds OpenAI-compatible tool calling support.
type McpRequest struct {
	llamacpp.LlamaCppRequest
	Tools      []Tool `json:"tools,omitempty"`
	ToolChoice string `json:"tool_choice,omitempty"` // "auto" or "none"
}

// McpMessage extends the standard message format with tool_calls support.
// Content is a pointer so it can be null when tool_calls is present (OpenAI convention).
type McpMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"` // nullable — null when tool_calls present
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// McpResponse represents the response from a tool-calling LLM request.
type McpResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int        `json:"index"`
		Message      McpMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// ToolMessage represents a tool result message with role "tool".
// The ToolCallID must match the ID from the originating ToolCall.
type ToolMessage struct {
	Role       string `json:"role"` // always "tool"
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}

// Tool name constants for dispatch.
const (
	ToolNameWebSearch = "web_search"
	ToolNameScrapeURL = "scrape_url"

	// Tool calling role and type constants.
	toolTypeFunction = "function"
	roleTool         = "tool"
	roleUser         = "user"
	roleAssistant    = "assistant"
	roleSystem       = "system"

	// Map key constants for conversation messages.
	keyRole    = "role"
	keyContent = "content"
	keyName    = "name"
)

// DefaultMcpTools returns the standard set of tools available to the LLM.
func DefaultMcpTools() []Tool {
	return []Tool{toolWebSearch, toolScrapeURL}
}

// Pre-defined tool schemas using OpenAI function calling JSON Schema format.
var (
	toolWebSearch = Tool{
		Type: toolTypeFunction,
		Function: ToolFunction{
			Name:        ToolNameWebSearch,
			Description: "Search the web using SearXNG. Returns a list of results with titles, URLs, and descriptions. Use this for finding current information, news, documentation, or answers to factual questions.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "The search query string. Be specific for better results."
					}
				},
				"required": ["query"]
			}`),
		},
	}

	toolScrapeURL = Tool{
		Type: toolTypeFunction,
		Function: ToolFunction{
			Name:        ToolNameScrapeURL,
			Description: "Scrape and extract the text content from a web page URL using Firecrawl. Returns the page content as markdown. Use this to get detailed information from a specific URL found via web_search.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {
						"type": "string",
						"description": "The URL to scrape. Must be a valid HTTP or HTTPS URL."
					}
				},
				"required": ["url"]
			}`),
		},
	}
)

// StringPtr returns a pointer to the given string. Useful for setting McpMessage.Content.
func StringPtr(s string) *string {
	return &s
}

// NullPtr returns a nil *string. Useful for setting McpMessage.Content to null.
func NullPtr() *string {
	return nil
}
