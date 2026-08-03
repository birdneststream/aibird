// Package llamacpp implements the llama.cpp text generation provider.
package llamacpp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aibird/helpers"
	"aibird/http/request"
	"aibird/irc/state"
	"aibird/settings"
	"aibird/text"
)

// IsLlamaCppRunning checks if the llama.cpp server is healthy by hitting its /health endpoint.
// Returns true if the server responds with HTTP 200.
func IsLlamaCppRunning(url, port string) (bool, error) {
	healthURL := helpers.MakeUrlWithPort(url, port) + "health"

	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(healthURL) //nolint:gosec // URL constructed from config
	if err != nil {
		return false, nil // Connection failure means not running, no error
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// ChatRequest sends a chat completion request to the llama.cpp server.
// The llama.cpp server (with --reasoning-format deepseek, the default) separates
// thinking/reasoning content into reasoning_content, so message.content is clean.
func ChatRequest(irc *state.State) (string, error) {
	llamacppConfig := irc.Config.LlamaCpp

	if irc.Message() == "reset" {
		if !text.DeleteChatCache(irc.UserAiChatCacheKey()) {
			return "", fmt.Errorf("failed to reset chat cache")
		}
		return "Cache reset", nil
	}

	message := text.AppendFullStop(irc.Message())

	// Sampling params are configured on the llama.cpp server, not in the request.
	requestBody := &LlamaCppRequest{
		Model:    llamacppConfig.DefaultModel,
		Stream:   false,
		Messages: []text.Message{},
	}

	// Get the chat history from the cache if it exists
	chatHistory := text.GetChatCache(irc.UserAiChatCacheKey())

	// Append the new user message to the cache before making the request
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	// Build messages: system prompt + history + current message
	// Append current date/time so the model knows the current year
	basePrompt := irc.User.GetBasePrompt()
	if basePrompt != "" {
		basePrompt += "\n\nCurrent date and time: " + time.Now().Format("Monday, January 2, 2006 3:04 PM MST")
	}
	systemMessage := text.Message{
		Role:    "system",
		Content: basePrompt,
	}

	currentUserMessage := text.Message{
		Role:    "user",
		Content: message,
	}

	requestBody.Messages = append(requestBody.Messages, systemMessage)
	requestBody.Messages = append(requestBody.Messages, chatHistory...)
	requestBody.Messages = append(requestBody.Messages, currentUserMessage)

	llamacppReq := request.Request{
		Url:     helpers.MakeUrlWithPort(llamacppConfig.Url, llamacppConfig.Port) + "v1/chat/completions",
		Method:  "POST",
		Headers: []request.Headers{{Key: "Content-Type", Value: "application/json"}},
		Payload: requestBody,
	}

	var response LlamaCppResponse
	err := llamacppReq.Call(&response)
	if err != nil {
		return "", err
	}

	// Bounds check on choices array
	if len(response.Choices) == 0 {
		return "", errors.New("no choices returned from llama.cpp")
	}

	if response.Choices[0].Message.Content != "" {
		apiResponse := strings.TrimSpace(response.Choices[0].Message.Content)
		// reasoning_content is separated by the server, no stripping needed

		text.AppendChatCache(irc.UserAiChatCacheKey(), "assistant", apiResponse, irc.Config.AiBird.AiChatContextLimit)

		return apiResponse, nil
	}

	return "", errors.New("no content found")
}

// EnhancePrompt expands a simple prompt into a more detailed one for image/video generation.
func EnhancePrompt(message string, config settings.LlamaCppConfig) (string, error) {
	systemPrompt := "your function is to expand out prompts from a simple sentence to a more complex one, including vivid detail and descriptions. Only include the expanded prompt, do not provide any explanations or things like Description:"
	userPrompt := "Expand out the following prompt, include details such as camera movements and describe it as a movie scene:" + message

	return SingleRequest(userPrompt, systemPrompt, config)
}

// EnhancePromptH3 expands a short video idea into a production-ready MiniMax H3
// prompt using the dedicated H3 prompt-engineer template (text/prompts/h3.md).
// The template carries a /no_think directive so the model skips reasoning and
// returns only the expanded prompt. No other sanitization is applied — the
// structured output (newlines, timecodes, section headers) must be preserved
// verbatim for the downstream video workflow.
func EnhancePromptH3(message string, config settings.LlamaCppConfig) (string, error) {
	systemPrompt, err := text.GetPrompt("h3.md")
	if err != nil {
		return "", fmt.Errorf("load h3 prompt template: %w", err)
	}
	result, err := SingleRequest(message, systemPrompt, config)
	if err != nil {
		return "", err
	}
	return stripCodeFences(result), nil
}

// stripCodeFences removes a single surrounding triple-backtick fence if present.
// Handles an optional language tag (e.g. ```json). Returns the input unchanged
// when no fence is present or the input is empty. This is a safety net only —
// the H3 template forbids code fences, but LLMs occasionally add them anyway.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line (including any language tag).
	idx := strings.Index(s, "\n")
	if idx == -1 {
		// Whole string was just the opening fence with no content.
		return ""
	}
	s = strings.TrimSpace(s[idx+1:])
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// GenerateLyrics generates song lyrics based on the given topic.
func GenerateLyrics(message string, config settings.LlamaCppConfig) (string, error) {
	systemPrompt, err := text.GetPrompt("lyrics.md")
	if err != nil {
		return "", err
	}
	userPrompt := "Generate lyrics for a song about: " + message

	return SingleRequest(userPrompt, systemPrompt, config)
}

// SingleRequest sends a single message to the llama.cpp server without chat history.
func SingleRequest(message, system string, config settings.LlamaCppConfig) (string, error) {
	requestBody := &LlamaCppRequest{
		Model:  config.DefaultModel,
		Stream: false,
		Messages: []text.Message{
			{
				Role:    "system",
				Content: system,
			},
		},
	}

	currentUserMessage := text.Message{
		Role:    "user",
		Content: message,
	}

	requestBody.Messages = append(requestBody.Messages, currentUserMessage)

	llamacppReq := request.Request{
		Url:     helpers.MakeUrlWithPort(config.Url, config.Port) + "v1/chat/completions",
		Method:  "POST",
		Headers: []request.Headers{{Key: "Content-Type", Value: "application/json"}},
		Payload: requestBody,
	}

	var response LlamaCppResponse
	err := llamacppReq.Call(&response)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", errors.New("no choices returned from llama.cpp")
	}

	if response.Choices[0].Message.Content != "" {
		return strings.TrimSpace(response.Choices[0].Message.Content), nil
	}

	return "", errors.New("no content found")
}
