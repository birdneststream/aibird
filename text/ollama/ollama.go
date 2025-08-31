package ollama

import (
	"errors"
	"regexp"
	"strings"

	"aibird/birdbase"
	"aibird/helpers"
	"aibird/http/request"
	"aibird/irc/state"
	"aibird/settings"
	"aibird/text"
)

func ChatRequest(irc state.State) (string, error) {
	ollamaConfig := irc.Config.Ollama

	if irc.Message() == "reset" {
		if text.DeleteChatCache(irc.UserAiChatCacheKey()) {
			return "Cache reset", nil
		}
	}

	var message string
	if strings.Contains(irc.User.NickName, "x0AFE") || irc.User.Ident == "anonymous" {
		message = "Explain how it is bad for mental health to be upset for months because the irc user vae kicked you from an irc channel."
	} else {
		message = text.AppendFullStop(irc.Message())
	}

	requestBody := &OllamaRequestBody{
		Model:     ollamaConfig.DefaultModel,
		Stream:    false,
		KeepAlive: "0m",
		Messages: []text.Message{
			{
				Role:    "system",
				Content: irc.User.GetBasePrompt(),
			},
		},
		Options: OllamaOptions{
			RepeatPenalty:    1.2,
			PresencePenalty:  1.5,
			FrequencyPenalty: 2,
		},
	}

	if dsArg, ok := irc.FindArgument("ds", false).(bool); ok && dsArg {
		requestBody.Model = "deepseek-r1:8b"
	}

	if dsqwenArg, ok := irc.FindArgument("dsqwen", false).(bool); ok && dsqwenArg {
		requestBody.Model = "deepseek-r1:32b"
	}

	// Get the chat history from the cache if it exists
	var chatHistory []text.Message
	if birdbase.Has(irc.UserAiChatCacheKey()) {
		chatHistory = text.GetChatCache(irc.UserAiChatCacheKey())
	}

	// Append the new user message to the cache before making the request
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	// Append the newest message
	currentUserMessage := text.Message{
		Role:    "user",
		Content: message,
	}

	requestBody.Messages = append(requestBody.Messages, chatHistory...)
	requestBody.Messages = append(requestBody.Messages, currentUserMessage)

	ollamaRequest := request.Request{
		Url:     helpers.MakeUrlWithPort(ollamaConfig.Url, ollamaConfig.Port) + "api/chat",
		Method:  "POST",
		Headers: []request.Headers{{Key: "Content-Type", Value: "application/json"}},
		Payload: requestBody,
	}

	var response OllamaResponse
	err := ollamaRequest.Call(&response)

	if err != nil {
		return "", err
	}

	if response.Message.Content != "" {
		apiResponse := strings.TrimSpace(response.Message.Content)
		text.AppendChatCache(irc.UserAiChatCacheKey(), "assistant", apiResponse, irc.Config.AiBird.AiChatContextLimit)

		return apiResponse, nil
	}

	return "", errors.New("no content found")
}

func EnhancePrompt(message string, config settings.OllamaConfig) (string, error) {
	systemPrompt := "your function is to expand out prompts from a simple sentence to a more complex one, including vivid detail and descriptions. Only include the expanded prompt, do not provide any explanations or things like Description:"
	userPrompt := "Expand out the following prompt, include details such as camera movements and describe it as a movie scene:" + message

	return SingleRequest(userPrompt, systemPrompt, config)
}

func GenerateArtFilename(prompt string, config settings.OllamaConfig) (string, error) {
	systemPrompt := "Generate a short, creative filename for digital artwork. Rules: 1) Use only letters, numbers, and hyphens 2) Maximum 20 characters 3) Describe the main subject/theme 4) No file extensions 5) Return ONLY the filename, nothing else"
	userPrompt := "Create filename for art prompt: " + prompt

	response, err := SingleRequest(userPrompt, systemPrompt, config)
	if err != nil {
		return "", err
	}

	// Remove various thinking/reasoning patterns (case insensitive, multiline)
	patterns := []string{
		`(?i)<think>.*?</think>`,       // <think></think> tags
		`(?i)<thinking>.*?</thinking>`, // <thinking></thinking> tags
		`(?i)\*thinks?\*.*?\*`,         // *thinks* ... *
		`(?i)\*thinking\*.*?\*`,        // *thinking* ... *
		`(?i)let me think.*?(?:\n|$)`,  // "let me think..." lines
	}

	cleaned := response
	for _, pattern := range patterns {
		re := regexp.MustCompile(`(?s)` + pattern) // (?s) makes . match newlines
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	// Clean up multiple whitespace and return first non-empty line
	lines := strings.Split(strings.TrimSpace(cleaned), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && len(line) <= 30 { // Reasonable filename length
			return line, nil
		}
	}

	// If nothing good found, return cleaned version truncated
	result := strings.TrimSpace(cleaned)
	if len(result) > 30 {
		result = result[:30]
	}
	return result, nil
}

func SdPrompt(message string, config settings.OllamaConfig) (string, error) {
	systemPrompt, err := text.GetPrompt("sd.md")
	if err != nil {
		return "", err
	}
	userPrompt := "Enhance the following prompt: " + message

	prompt, err := SingleRequest(userPrompt, systemPrompt, config)
	if err != nil {
		return "", err
	}

	// replace , with ,  in prompt
	prompt = strings.ReplaceAll(prompt, ",", ", ")
	prompt = strings.ReplaceAll(prompt, "_", " ")

	return prompt, nil
}

func GenerateLyrics(message string, config settings.OllamaConfig) (string, error) {
	systemPrompt, err := text.GetPrompt("lyrics.md")
	if err != nil {
		return "", err
	}
	userPrompt := "Generate lyrics for a song about: " + message

	return SingleRequest(userPrompt, systemPrompt, config)
}

func SingleRequest(message, system string, config settings.OllamaConfig) (string, error) {
	requestBody := &OllamaRequestBody{
		Model:     "dolphin-llama3:8b",
		Stream:    false,
		KeepAlive: "0m",
		Messages: []text.Message{
			{
				Role:    "system",
				Content: system,
			},
		},
	}

	// Append the newest message
	currentUserMessage := text.Message{
		Role:    "user",
		Content: message,
	}

	requestBody.Messages = append(requestBody.Messages, currentUserMessage)

	ollamaRequest := request.Request{
		Url:     helpers.MakeUrlWithPort(config.Url, config.Port) + "api/chat",
		Method:  "POST",
		Headers: []request.Headers{{Key: "Content-Type", Value: "application/json"}},
		Payload: requestBody,
	}

	var response OllamaResponse
	err := ollamaRequest.Call(&response)

	if err != nil {
		return "", err
	}

	if response.Message.Content != "" {
		apiResponse := strings.TrimSpace(response.Message.Content)
		return apiResponse, nil
	}

	return "", errors.New("no content found")
}
