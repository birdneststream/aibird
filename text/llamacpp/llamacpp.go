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

// EnhancePrompt expands a simple prompt into a more detailed one for image/video generation.
func EnhancePrompt(message string, config settings.LlamaCppConfig) (string, error) {
	systemPrompt := "your function is to expand out prompts from a simple sentence to a more complex one, including vivid detail and descriptions. Only include the expanded prompt, do not provide any explanations or things like Description:"
	userPrompt := "Expand out the following prompt, include details such as camera movements and describe it as a movie scene:" + message

	return SingleRequest(userPrompt, systemPrompt, config)
}

// EnhancePromptWithSystemPrompt expands a message using a pre-built system
// prompt directly. This is used when combining a base template with a style
// overlay (e.g. the H3 base craft prepended to a Simpsons style overlay).
// Thinking is disabled at the model level, so the system prompt carries only
// role and style instructions and the model returns just the expanded prompt.
// stripCodeFences is applied as a safety net since LLMs occasionally wrap
// output in a code block; the rest of the output is preserved verbatim for the
// downstream video workflow.
func EnhancePromptWithSystemPrompt(message, systemPrompt string, config settings.LlamaCppConfig) (string, error) {
	result, err := SingleRequest(message, systemPrompt, config)
	if err != nil {
		return "", err
	}
	return stripCodeFences(result), nil
}

// EnhancePromptWithTemplate expands a message using a named prompt template from
// text/prompts/. It is a thin wrapper around EnhancePromptWithSystemPrompt that
// loads the template file. Used by the !h3 / !sienfield / !garfield /
// !spongebob (and other style) video commands.
func EnhancePromptWithTemplate(message, templateFile string, config settings.LlamaCppConfig) (string, error) {
	systemPrompt, err := text.GetPrompt(templateFile)
	if err != nil {
		return "", fmt.Errorf("load prompt template %s: %w", templateFile, err)
	}
	return EnhancePromptWithSystemPrompt(message, systemPrompt, config)
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
