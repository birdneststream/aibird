package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		reqJSON := fmt.Sprintf(`{"model":%q,"stream":false,"repeat_penalty":1.2,"presence_penalty":1.5,"frequency_penalty":2.0,"tools":%s,"tool_choice":"auto","messages":%s}`,
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

		// Check if the LLM wants to call tools
		if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) > 0 {
			logger.Debug("LLM requested tool calls", "count", len(choice.Message.ToolCalls))

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

		// Final response — LLM is done
		if choice.Message.Content != nil && *choice.Message.Content != "" {
			response := strings.TrimSpace(*choice.Message.Content)

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
		Role:    "system",
		Content: StringPtr(systemPrompt),
	})

	// Chat history (regular user/assistant messages)
	for _, msg := range chatHistory {
		conversation = append(conversation, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	// Current user message
	conversation = append(conversation, map[string]string{
		"role":    roleUser,
		"content": currentUserMessage,
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
