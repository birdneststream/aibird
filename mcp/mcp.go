package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aibird/helpers"
	"aibird/http/request"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/text"
)

const (
	defaultMaxIterations = 5
	defaultTimeoutSecs   = 120
)

// Pre-compiled regexes for text-based tool call parsing.
// Models output tool calls in various text formats when llama.cpp doesn't
// parse them into structured tool_calls.
var (
	// <toolcall>...</toolcall> or <tool_call...>...</tool_call⟩ blocks (JSON inside)
	reToolCallBlock = regexp.MustCompile(`(?is)<tool_?call\s*>(.*?)</tool_?call>`)
	// <function=NAME>(ARGS) format — Qwen models use this
	reFunctionCall = regexp.MustCompile(`(?is)<function\s*=\s*(\w+)\s*>\s*\((.*?)\)`)
	// <function=NAME> without parens (standalone tag)
	reFunctionTag = regexp.MustCompile(`(?is)<function\s*=\s*(\w+)\s*>`)
	// Also try: NAME>(key="value") — garbled function format where <function= is missing/broken
	reGarbledFuncCall = regexp.MustCompile(`(?is)(?:web_search|scrape_url)\s*>\s*\((\w+)\s*=\s*"(.*?)"\)`)

	// Strippers for cleaning final responses:
	// Specific patterns
	reStripToolTags = regexp.MustCompile(`(?is)<tool_?call\s*>.*?</tool_?call\s*>`)
	reStripToolOpen = regexp.MustCompile(`(?is)<tool_?call[^>]*>`)
	reStripFuncCall = regexp.MustCompile(`(?is)<function\s*=\s*\w+\s*>\s*\(.*?\)`)
	reStripFuncTag  = regexp.MustCompile(`(?is)<function\s*=\s*\w+\s*>`)
	// Catch-all: any XML-like tag that looks like a tool artifact (parameter, result, toolcodename, api_result, etc.)
	reStripToolArtifact = regexp.MustCompile(`(?is)</?(?:parameter|result|tool\w*|api_\w*|function\w*|toolcall|tool_call)[^>]*>`)
	// Garbled tool calls: toolname>(args)
	reStripGarbledCall = regexp.MustCompile(`(?is)(?:web_search|scrape_url)\s*>[^<]*`)
	// Base64 data URIs in markdown images: ![...](data:image/...;base64,...)
	reStripBase64Image = regexp.MustCompile(`!\[[^\]]*\]\(data:image/[^;]+;base64,[^)]*\)`)
	// JSON error blocks: {"error":"..."}
	reStripJSONError = regexp.MustCompile(`\{"error"\s*:\s*"[^"]*"\}`)
	// Multiple newlines cleanup
	reMultiNewline = regexp.MustCompile(`\n{2,}`)
)

// textToolCall represents a parsed text-based tool call from model output.
type textToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// parseTextToolCalls extracts tool calls from model text output.
// Handles multiple formats:
//   - Block: <toolcall>{"name":"web_search","arguments":{"query":"..."}}</toolcall>
//   - Inline: <tool_call{"name":"web_search","arguments":{"query":"..."}}
//   - Function: <function=web_search>(query="latest news...")
func parseTextToolCalls(content string) []textToolCall {
	var calls []textToolCall

	// Strategy 1: <function=NAME>(ARGS) format (Qwen models)
	funcMatches := reFunctionCall.FindAllStringSubmatch(content, -1)
	for _, match := range funcMatches {
		if len(match) < 3 {
			continue
		}
		name := strings.TrimSpace(match[1])
		argsStr := strings.TrimSpace(match[2])
		if tc := parseFunctionCallArgs(name, argsStr); tc != nil {
			calls = append(calls, *tc)
		}
	}

	// Strategy 1b: <function=NAME> without parens — extract name, empty args
	if len(calls) == 0 {
		tagMatches := reFunctionTag.FindAllStringSubmatch(content, -1)
		for _, match := range tagMatches {
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if isValidToolName(name) {
				calls = append(calls, textToolCall{
					Name:      name,
					Arguments: map[string]interface{}{},
				})
			}
		}
	}

	// Strategy 1c: Garbled format — NAME>(key="value") where the tag is broken
	if len(calls) == 0 {
		calls = parseGarbledToolCalls(content)
	}

	// Strategy 2: <toolcall>{json}</toolcall> blocks
	if len(calls) == 0 {
		blockMatches := reToolCallBlock.FindAllStringSubmatch(content, -1)
		for _, match := range blockMatches {
			if len(match) < 2 {
				continue
			}
			inner := strings.TrimSpace(match[1])
			if tc := parseToolCallJSON(inner); tc != nil {
				calls = append(calls, *tc)
			}
		}
	}

	// Strategy 3: Inline <tool_call{...} or <toolcall{...} (no closing > before JSON)
	if len(calls) == 0 {
		calls = parseInlineToolCalls(content)
	}

	return calls
}

// parseGarbledToolCalls handles garbled tool call output where the XML tag is
// broken but the tool name and arguments are still present.
// e.g., "scrape_url>(url=\"https://...\")"
func parseGarbledToolCalls(content string) []textToolCall {
	var calls []textToolCall

	garbledMatches := reGarbledFuncCall.FindAllStringSubmatch(content, -1)
	for _, match := range garbledMatches {
		if len(match) < 3 {
			continue
		}
		key := strings.TrimSpace(match[1])
		value := strings.TrimSpace(match[2])
		args := map[string]interface{}{key: value}

		// Determine which tool based on content
		if strings.Contains(content, "scrape_url") {
			calls = append(calls, textToolCall{Name: ToolNameScrapeURL, Arguments: args})
		} else {
			calls = append(calls, textToolCall{Name: ToolNameWebSearch, Arguments: args})
		}
	}

	return calls
}

// isValidToolName checks if a name matches a known tool.
func isValidToolName(name string) bool {
	return name == ToolNameWebSearch || name == ToolNameScrapeURL
}

// parseFunctionCallArgs parses <function=NAME>(query="value") style arguments
// into a textToolCall. Handles both quoted and unquoted values, multiple params.
func parseFunctionCallArgs(name, argsStr string) *textToolCall {
	args := make(map[string]interface{})

	if argsStr == "" {
		return &textToolCall{Name: name, Arguments: args}
	}

	// Parse key=value or key="value" pairs, comma separated
	// Examples: query="latest news on Hanta virus", query=test, url="https://example.com"
	remaining := argsStr
	for remaining != "" {
		remaining = strings.TrimSpace(remaining)

		// Find key=
		eqIdx := strings.Index(remaining, "=")
		if eqIdx < 0 {
			break
		}
		key := strings.TrimSpace(remaining[:eqIdx])
		remaining = remaining[eqIdx+1:]

		if remaining == "" {
			args[key] = ""
			break
		}

		var value string
		if remaining[0] == '"' {
			// Quoted value — find closing quote
			endQuote := strings.Index(remaining[1:], "\"")
			if endQuote < 0 {
				// No closing quote — take the rest
				value = remaining[1:]
				remaining = ""
			} else {
				value = remaining[1 : endQuote+1]
				remaining = remaining[endQuote+2:]
			}
		} else {
			// Unquoted value — take until comma or end
			commaIdx := strings.Index(remaining, ",")
			if commaIdx < 0 {
				value = remaining
				remaining = ""
			} else {
				value = strings.TrimSpace(remaining[:commaIdx])
				remaining = remaining[commaIdx+1:]
			}
		}

		args[key] = value

		// Skip comma separator
		remaining = strings.TrimPrefix(remaining, ",")
	}

	return &textToolCall{Name: name, Arguments: args}
}

// parseInlineToolCalls finds tool calls where the tag is directly followed by JSON.
// Handles: <tool_call{"name":"web_search","arguments":{...}}
func parseInlineToolCalls(content string) []textToolCall {
	var calls []textToolCall

	// Find all positions where <tool_call or <toolcall appears
	marker := "<tool"
	pos := 0
	for {
		idx := strings.Index(content[pos:], marker)
		if idx < 0 {
			break
		}
		idx += pos // absolute position

		// Check if this is actually <tool_call or <toolcall
		rest := content[idx:]
		if !strings.HasPrefix(rest, "<tool_call") && !strings.HasPrefix(rest, "<toolcall") {
			pos = idx + 1
			continue
		}

		// Find the first { after the marker
		braceStart := strings.Index(rest, "{")
		if braceStart < 0 {
			pos = idx + 1
			continue
		}

		jsonStart := idx + braceStart
		// Find the matching closing brace
		jsonEnd := findJSONEnd(content[jsonStart:])
		if jsonEnd < 0 {
			pos = idx + 1
			continue
		}

		jsonStr := content[jsonStart : jsonStart+jsonEnd]
		if tc := parseToolCallJSON(jsonStr); tc != nil {
			calls = append(calls, *tc)
			pos = jsonStart + jsonEnd
		} else {
			pos = idx + 1
		}
	}

	return calls
}

// parseToolCallJSON tries to parse a JSON object as a tool call.
func parseToolCallJSON(data string) *textToolCall {
	// Sometimes the JSON has trailing noise — try to extract just the JSON object
	data = strings.TrimSpace(data)

	// Find the first { and try to parse from there
	start := strings.Index(data, "{")
	if start < 0 {
		return nil
	}
	data = data[start:]

	var tc textToolCall
	if err := json.Unmarshal([]byte(data), &tc); err != nil {
		// Try to find the end of the JSON object by counting braces
		end := findJSONEnd(data)
		if end > 0 {
			if err2 := json.Unmarshal([]byte(data[:end]), &tc); err2 != nil {
				logger.Debug("Failed to parse text tool call JSON", "error", err, "data", data[:min(200, len(data))])
				return nil
			}
		} else {
			logger.Debug("Failed to parse text tool call JSON", "error", err, "data", data[:min(200, len(data))])
			return nil
		}
	}

	if tc.Name == "" {
		return nil
	}

	return &tc
}

// findJSONEnd finds the end of a JSON object by matching braces.
func findJSONEnd(s string) int {
	depth := 0
	inStr := false
	escape := false

	for i, ch := range s {
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && inStr {
			escape = true
			continue
		}
		if ch == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		if ch == '{' {
			depth++
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}

	return -1
}

// hasTextToolCalls checks if content contains text-based tool call markers.
func hasTextToolCalls(content string) bool {
	return strings.Contains(content, "<toolcall") ||
		strings.Contains(content, "<tool_call") ||
		strings.Contains(content, "<tool call") ||
		strings.Contains(content, "<function=")
}

// stripToolCallTags removes any tool call markers from a final response.
// Applies increasingly aggressive patterns to catch well-formed tags,
// garbled XML artifacts, base64 data URIs, and JSON error blocks.
func stripToolCallTags(content string) string {
	cleaned := content

	// Well-formed tags
	cleaned = reStripToolTags.ReplaceAllString(cleaned, "")
	cleaned = reStripToolOpen.ReplaceAllString(cleaned, "")
	cleaned = reStripFuncCall.ReplaceAllString(cleaned, "")
	cleaned = reStripFuncTag.ReplaceAllString(cleaned, "")

	// Garbled tool call artifacts
	cleaned = reStripToolArtifact.ReplaceAllString(cleaned, "")
	cleaned = reStripGarbledCall.ReplaceAllString(cleaned, "")

	// Data artifacts (base64 images, JSON errors)
	cleaned = reStripBase64Image.ReplaceAllString(cleaned, "")
	cleaned = reStripJSONError.ReplaceAllString(cleaned, "")

	// Clean up multiple consecutive whitespace/newlines
	cleaned = reMultiNewline.ReplaceAllString(cleaned, "\n")
	return strings.TrimSpace(cleaned)
}

// ChatWithTools sends a chat request to llama.cpp with tool calling support.
// It runs a multi-turn loop: if the LLM returns tool_calls, the tools are
// executed and results fed back until the LLM produces a final text response.
// Only the final user message and assistant response are cached.
func ChatWithTools(irc *state.State) (string, error) { //nolint:gocyclo // tool calling loop inherently has many branches
	mcpConfig := &irc.Config.Mcp
	llamacppConfig := irc.Config.LlamaCpp

	// Handle reset command
	if irc.Message() == "reset" {
		if !text.DeleteChatCache(irc.UserAiChatCacheKey()) {
			return "", fmt.Errorf("failed to reset chat cache")
		}
		return "Cache reset", nil
	}

	// Load system prompt
	systemPrompt, err := text.GetPrompt("ai_tools.md")
	if err != nil {
		logger.Warn("Failed to load AI tools system prompt, using default", "error", err)
		systemPrompt = "You are a helpful AI assistant with access to web search and URL scraping tools. Use them when needed to answer questions accurately. Cite your sources."
	}

	message := text.AppendFullStop(irc.Message())

	// Build conversation history from cache
	chatHistory := text.GetChatCache(irc.UserAiChatCacheKey())

	// Append user message to cache
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	// Build the conversation for the tool calling loop.
	// We use interface{} slice to handle both regular messages and tool messages.
	conversation := buildConversation(systemPrompt, chatHistory, message)

	// Get configurable limits
	maxIter := mcpConfig.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}
	timeoutSecs := mcpConfig.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultTimeoutSecs
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// Tool calling loop
	apiURL := helpers.MakeUrlWithPort(llamacppConfig.Url, llamacppConfig.Port) + "v1/chat/completions"
	tools := DefaultMcpTools()

	for iteration := 0; iteration < maxIter; iteration++ {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("tool calling loop timed out after %d seconds", timeoutSecs)
		default:
		}

		logger.Debug("Tool calling iteration", "iteration", iteration+1, "max", maxIter)

		// Build the request JSON directly to support mixed message types
		messagesJSON, err := json.Marshal(conversation)
		if err != nil {
			return "", fmt.Errorf("failed to marshal conversation: %w", err)
		}

		// Use raw messages to support mixed message types
		reqJSON := fmt.Sprintf(`{"model":%q,"stream":false,"repeat_penalty":1.1,"presence_penalty":0.4,"frequency_penalty":0.4,"max_tokens":2048,"tools":%s,"tool_choice":"auto","messages":%s}`,
			llamacppConfig.DefaultModel,
			mustMarshal(tools),
			messagesJSON,
		)

		httpReq := request.Request{
			Url:     apiURL,
			Method:  "POST",
			Headers: []request.Headers{{Key: "Content-Type", Value: "application/json"}},
			Payload: json.RawMessage(reqJSON),
		}

		var resp McpResponse
		if err := httpReq.Call(&resp); err != nil {
			return "", fmt.Errorf("llama.cpp request failed: %w", err)
		}

		if len(resp.Choices) == 0 {
			return "", errors.New("no choices returned from llama.cpp")
		}

		choice := resp.Choices[0]

		// Check if the LLM wants to call tools (structured OpenAI format)
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			logger.Debug("LLM requested tool calls (structured)", "count", len(choice.Message.ToolCalls))

			// Add assistant message with tool calls to conversation
			conversation = append(conversation, choice.Message)

			// Execute each tool call and add results
			for _, tc := range choice.Message.ToolCalls {
				logger.Debug("Executing tool", "tool", tc.Function.Name, "args", tc.Function.Arguments)

				result := ExecuteTool(ctx, tc.Function.Name, tc.Function.Arguments, mcpConfig)
				logger.Debug("Tool result", "tool", tc.Function.Name, "result_len", len(result))

				conversation = append(conversation, ToolMessage{
					Role:       roleTool,
					Content:    result,
					ToolCallID: tc.ID,
				})
			}

			// Continue loop to send results back to LLM
			continue
		}

		// Check for text-based tool calls in content (Qwen models via llama.cpp
		// output <toolcall>{"name":"web_search","arguments":{...}}</toolcall> as text
		// instead of structured tool_calls in the API response).
		if choice.Message.Content != nil && hasTextToolCalls(*choice.Message.Content) {
			textCalls := parseTextToolCalls(*choice.Message.Content)
			if len(textCalls) > 0 {
				logger.Debug("LLM requested tool calls (text-based)", "count", len(textCalls))

				// Add assistant's tool call text to conversation so the model
				// sees its own output in the next iteration
				conversation = append(conversation, map[string]string{
					keyRole:    roleAssistant,
					keyContent: *choice.Message.Content,
				})

				// Execute each parsed tool call
				for _, tc := range textCalls {
					argsJSON, err := json.Marshal(tc.Arguments)
					if err != nil {
						logger.Warn("Failed to marshal text tool call args", "error", err)
						continue
					}

					logger.Debug("Executing text-based tool", "tool", tc.Name, "args", string(argsJSON))

					result := ExecuteTool(ctx, tc.Name, string(argsJSON), mcpConfig)
					logger.Debug("Text-based tool result", "tool", tc.Name, "result_len", len(result))

					// Add tool result as user message (Qwen expects "tool" or "user" role
					// for results after text-based tool calls)
					conversation = append(conversation, map[string]string{
						keyRole:    roleTool,
						keyContent: result,
						keyName:    tc.Name,
					})
				}

				// Continue loop to send results back to LLM
				continue
			}

			// Had tool call markers but couldn't parse them — strip and treat as final
			logger.Warn("Found tool call markers but failed to parse, stripping tags from response")
		}

		// Final response — LLM is done
		if choice.Message.Content != nil && *choice.Message.Content != "" {
			response := strings.TrimSpace(*choice.Message.Content)

			// Safety: strip any remaining tool call tags from the final response
			response = stripToolCallTags(response)

			if response == "" {
				return "", errors.New("LLM returned only tool call tags with no text content")
			}

			// Cache the final assistant response
			text.AppendChatCache(irc.UserAiChatCacheKey(), "assistant", response, irc.Config.AiBird.AiChatContextLimit)

			return response, nil
		}

		// Empty content with no tool calls — shouldn't happen but handle gracefully
		return "", errors.New("LLM returned empty response with no tool calls")
	}

	return "", fmt.Errorf("tool calling loop reached max iterations (%d)", maxIter)
}

// buildConversation creates the initial conversation message list for the tool
// calling loop. It uses interface{} to allow mixed message types (McpMessage
// for system/assistant messages, text.Message for user/history, ToolMessage
// for tool results).
func buildConversation(systemPrompt string, chatHistory []text.Message, currentUserMessage string) []interface{} {
	var conversation []interface{}

	// System prompt
	conversation = append(conversation, McpMessage{
		Role:    roleSystem,
		Content: StringPtr(systemPrompt),
	})

	// Chat history (regular user/assistant messages)
	for _, msg := range chatHistory {
		conversation = append(conversation, map[string]string{
			keyRole:    msg.Role,
			keyContent: msg.Content,
		})
	}

	// Current user message
	conversation = append(conversation, map[string]string{
		keyRole:    roleUser,
		keyContent: currentUserMessage,
	})

	return conversation
}

// mustMarshal marshals to JSON or returns an empty array on failure.
func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		logger.Error("Failed to marshal", "error", err)
		return json.RawMessage("[]")
	}
	return data
}
