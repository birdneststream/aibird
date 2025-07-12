package openrouter

//
import (
	"aibird/birdbase"
	"aibird/helpers"
	"aibird/http/request"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"
	"aibird/text"
	"fmt"
	"strings"
)

// OpenRouterRequest coordinates the entire process of handling an OpenRouter request.
func OpenRouterRequest(irc state.State) (string, error) {
	// 1. Handle special commands like "reset"
	if didHandle, response := handleResetCommand(irc); didHandle {
		return response, nil
	}

	// 2. Prepare the data for the request
	message := text.AppendFullStop(irc.Message())
	requestBody := buildOpenRouterRequestBody(irc, message)

	// 3. Append the new user message to the cache
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	// 4. Build and execute the HTTP request
	httpRequest := buildHttpRequest(irc.Config.OpenRouter, requestBody)
	var response OpenRouterResponse
	if err := httpRequest.Call(&response); err != nil {
		return "", err
	}

	// 5. Process the response and update the cache
	return processOpenRouterResponse(irc, &response)
}

// handleResetCommand checks for and handles the "reset" command.
// It returns true and a message if the command was handled, false otherwise.
func handleResetCommand(irc state.State) (bool, string) {
	if irc.Message() != "reset" {
		return false, ""
	}
	if err := birdbase.Delete(irc.UserAiChatCacheKey()); err != nil {
		logger.Error("Failed to delete user AI chat cache", "user", irc.User.NickName, "error", err)
	}
	return true, "Cache reset"
}

// buildOpenRouterRequestBody creates the request body for the OpenRouter API call.
func buildOpenRouterRequestBody(irc state.State, message string) *OpenRouterRequestBody {
	body := &OpenRouterRequestBody{
		Model: irc.Config.OpenRouter.DefaultModel,
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
func buildHttpRequest(config settings.OpenRouterConfig, payload *OpenRouterRequestBody) request.Request {
	return request.Request{
		Url:    helpers.AppendSlashUrl(config.Url) + "chat/completions",
		Method: "POST",
		Headers: []request.Headers{
			{Key: "Content-Type", Value: "application/json"},
			{Key: "Authorization", Value: "Bearer " + config.ApiKey},
		},
		Payload: payload,
	}
}

// processOpenRouterResponse handles the API response, updates the cache, and returns the final message.
func processOpenRouterResponse(irc state.State, response *OpenRouterResponse) (string, error) {
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned an empty response")
	}

	apiResponse := strings.TrimSpace(response.Choices[0].Message.Content)
	text.AppendChatCache(irc.UserAiChatCacheKey(), "assistant", apiResponse, irc.Config.AiBird.AiChatContextLimit)

	return apiResponse, nil
}
