package llamacpp

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"aibird/birdbase"
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
func ChatRequest(irc state.State) (string, error) {
	llamacppConfig := irc.Config.LlamaCpp

	if irc.Message() == "reset" {
		if text.DeleteChatCache(irc.UserAiChatCacheKey()) {
			return "Cache reset", nil
		}
	}

	message := text.AppendFullStop(irc.Message())

	requestBody := &LlamaCppRequest{
		Model:            llamacppConfig.DefaultModel,
		Stream:           false,
		RepeatPenalty:    1.2,
		PresencePenalty:  1.5,
		FrequencyPenalty: 2.0,
		Messages:         []text.Message{},
	}

	// Get the chat history from the cache if it exists
	var chatHistory []text.Message
	if birdbase.Has(irc.UserAiChatCacheKey()) {
		chatHistory = text.GetChatCache(irc.UserAiChatCacheKey())
	}

	// Append the new user message to the cache before making the request
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	// Build messages: system prompt + history + current message
	systemMessage := text.Message{
		Role:    "system",
		Content: irc.User.GetBasePrompt(),
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
