// Package llamacpp implements the llama.cpp text generation provider.
package llamacpp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"aibird/helpers"
	"aibird/http/request"
	"aibird/irc/state"
	"aibird/settings"
	"aibird/text"
)

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
