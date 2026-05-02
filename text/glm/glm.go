package glm

import (
	"fmt"
	"strings"

	"aibird/birdbase"
	"aibird/http/request"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"
	"aibird/text"
)

// Request handles a GLM chat request from IRC
func Request(irc state.State) (string, error) {
	// Handle special commands like "reset"
	if didHandle, response := handleResetCommand(irc); didHandle {
		return response, nil
	}

	// Prepare the data for the request
	message := text.AppendFullStop(irc.Message())
	requestBody := buildRequestBody(irc, message)

	// Append the new user message to the cache
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	// Build and execute the HTTP request
	httpRequest := buildHttpRequest(irc.Config.Glm, requestBody)
	var response GlmResponse
	if err := httpRequest.Call(&response); err != nil {
		return "", err
	}

	// Process the response and update the cache
	return processResponse(irc, &response)
}

// handleResetCommand checks for and handles the "reset" command.
func handleResetCommand(irc state.State) (bool, string) {
	if irc.Message() != "reset" {
		return false, ""
	}
	if err := birdbase.Delete(irc.UserAiChatCacheKey()); err != nil {
		logger.Error("Failed to delete user AI chat cache", "user", irc.User.NickName, "error", err)
	}
	return true, "Cache reset"
}

// buildRequestBody creates the request body for the GLM API call.
func buildRequestBody(irc state.State, message string) *GlmRequestBody {
	body := &GlmRequestBody{
		Model:  irc.Config.Glm.DefaultModel,
		Stream: false,
		Thinking: &GlmThinking{
			Type: "disabled",
		},
		Messages: []text.Message{
			{Role: "system", Content: irc.User.GetBasePrompt()},
		},
	}

	if history := text.GetChatCache(irc.UserAiChatCacheKey()); history != nil {
		body.Messages = append(body.Messages, history...)
	}

	body.Messages = append(body.Messages, text.Message{Role: "user", Content: message})
	return body
}

// buildHttpRequest constructs the request.Request object for the API call.
func buildHttpRequest(config settings.GlmConfig, payload *GlmRequestBody) request.Request {
	return request.Request{
		Url:    config.Url,
		Method: "POST",
		Headers: []request.Headers{
			{Key: "Content-Type", Value: "application/json"},
			{Key: "Authorization", Value: "Bearer " + config.ApiKey},
		},
		Payload: payload,
	}
}

// processResponse handles the API response, updates the cache, and returns the final message.
func processResponse(irc state.State, response *GlmResponse) (string, error) {
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("GLM returned an empty response")
	}

	apiResponse := strings.TrimSpace(response.Choices[0].Message.Content)
	text.AppendChatCache(irc.UserAiChatCacheKey(), "assistant", apiResponse, irc.Config.AiBird.AiChatContextLimit)

	return apiResponse, nil
}

// SingleRequest makes a single GLM request without chat history (for headlines, etc.)
func SingleRequest(prompt string, config settings.GlmConfig) (string, error) {
	return SingleRequestWithSystem("", prompt, config)
}

// SingleRequestWithSystem makes a single GLM request with a system prompt and user prompt.
// If systemPrompt is empty, only the user prompt is sent.
func SingleRequestWithSystem(systemPrompt, userPrompt string, config settings.GlmConfig) (string, error) {
	messages := []text.Message{}
	if systemPrompt != "" {
		messages = append(messages, text.Message{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, text.Message{Role: "user", Content: userPrompt})

	body := &GlmRequestBody{
		Model:  config.DefaultModel,
		Stream: false,
		Thinking: &GlmThinking{
			Type: "disabled",
		},
		Messages: messages,
	}

	httpRequest := buildHttpRequest(config, body)
	var response GlmResponse
	if err := httpRequest.Call(&response); err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("GLM returned an empty response")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

// GenerateLyrics generates song lyrics using GLM
func GenerateLyrics(message string, config settings.GlmConfig) (string, error) {
	systemPrompt, err := text.GetPrompt("lyrics.md")
	if err != nil {
		return "", err
	}

	userPrompt := "Generate lyrics for a song about: " + message

	body := &GlmRequestBody{
		Model:  config.DefaultModel,
		Stream: false,
		Thinking: &GlmThinking{
			Type: "disabled",
		},
		Messages: []text.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}

	httpRequest := buildHttpRequest(config, body)
	var response GlmResponse
	if err := httpRequest.Call(&response); err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("GLM returned an empty response")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

// ImageRequest generates an image using CogView-4
func ImageRequest(prompt string, size string, config settings.GlmConfig) (string, error) {
	if config.ImageUrl == "" {
		return "", fmt.Errorf("GLM image URL not configured")
	}

	if size == "" {
		size = "1024x1024"
	}

	body := &GlmImageRequestBody{
		Model:  config.ImageModel,
		Prompt: prompt,
		Size:   size,
	}

	logger.Debug("GLM image request", "url", config.ImageUrl, "model", config.ImageModel, "prompt", prompt, "size", size)

	httpRequest := request.Request{
		Url:    config.ImageUrl,
		Method: "POST",
		Headers: []request.Headers{
			{Key: "Content-Type", Value: "application/json"},
			{Key: "Authorization", Value: "Bearer " + config.ApiKey},
		},
		Payload: body,
	}

	var response GlmImageResponse
	if err := httpRequest.Call(&response); err != nil {
		logger.Error("GLM image HTTP error", "error", err)
		return "", err
	}

	// Check for API error response
	if response.Error != nil {
		logger.Error("GLM image API error", "code", response.Error.Code, "message", response.Error.Message)
		return "", fmt.Errorf("GLM API error: %s - %s", response.Error.Code, response.Error.Message)
	}

	if len(response.Data) == 0 {
		logger.Error("GLM image returned empty data", "response", response)
		return "", fmt.Errorf("GLM image returned no data")
	}

	logger.Debug("GLM image success", "url", response.Data[0].URL)
	return response.Data[0].URL, nil
}
