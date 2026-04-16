package participant

import (
	"fmt"
	"strings"

	"aibird/http/request"
	"aibird/logger"
	"aibird/settings"
	"aibird/text"
)

// ParticipantRequest represents a chat completion request for the participant system
type ParticipantRequest struct {
	Model       string         `json:"model"`
	Messages    []text.Message `json:"messages"`
	Stream      bool           `json:"stream"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
}

// ParticipantResponse represents the response from a chat completion API
type ParticipantResponse struct {
	Choices []ParticipantChoice `json:"choices"`
}

// ParticipantChoice represents a response choice
type ParticipantChoice struct {
	Message text.Message `json:"message"`
}

// GenerateParticipantMessage generates a contextual response using GLM
func GenerateParticipantMessage(ctx MessageContext, config settings.GlmConfig) (string, error) {
	// Guard: skip participant responses if GLM is not configured
	if config.ApiKey == "" || config.Url == "" {
		return "", fmt.Errorf("GLM is not configured for participant system")
	}

	prompt := buildContextualPrompt(ctx)
	if prompt == "" {
		return "", fmt.Errorf("failed to build prompt for context type: %s", ctx.Type)
	}

	// Build conversation history as messages
	messages := []text.Message{
		{
			Role:    "system",
			Content: prompt,
		},
	}

	// Add conversation history for better context
	for _, msgText := range ctx.RecentMessages {
		messages = append(messages, text.Message{
			Role:    "user",
			Content: msgText,
		})
	}

	// Add a final instruction to prevent repetition
	if len(ctx.RecentMessages) > 0 {
		messages = append(messages, text.Message{
			Role:    "user",
			Content: "Please respond naturally considering the conversation above. Don't repeat greetings or questions already asked.",
		})
	}

	requestBody := ParticipantRequest{
		Model:       config.DefaultModel,
		Messages:    messages,
		Stream:      false,
		MaxTokens:   200, // Allow longer responses with more context
		Temperature: 0.8, // Natural variation
	}

	logger.Debug("Generating participant message", "type", ctx.Type, "personality", ctx.PersonalityMode, "context_messages", len(ctx.RecentMessages))

	response, err := makeGlmRequest(requestBody, config)
	if err != nil {
		return "", fmt.Errorf("GLM request failed: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices received from GLM")
	}

	// Clean and validate response
	message := cleanParticipantResponse(response.Choices[0].Message.Content)
	logger.Debug("Generated participant message", "response", message)

	return message, nil
}

// buildContextualPrompt creates a context-aware prompt for different message types
func buildContextualPrompt(ctx MessageContext) string {
	basePrompt := getPersonalityPrompt(ctx.PersonalityMode)

	switch ctx.Type {
	case "wake_greeting":
		return fmt.Sprintf(`%s

You just woke up and want to greet the IRC channel naturally. It's %s time.
Channel activity level: %s
Keep it casual and authentic - like a real person just waking up.
Response should be 1-2 sentences max, no asterisk actions.

Generate a natural wake-up greeting:`,
			basePrompt, ctx.TimeOfDay, ctx.ChannelActivity)

	case "sleep_message":
		return fmt.Sprintf(`%s

You're getting sleepy and want to say goodnight to the IRC channel. It's %s time.
Channel activity: %s
Keep it natural and brief - like a real person heading to bed.
1-2 sentences max, no asterisk actions.

Generate a natural goodnight message:`,
			basePrompt, ctx.TimeOfDay, ctx.ChannelActivity)

	case "spontaneous":
		contextStr := ""
		if len(ctx.RecentMessages) > 0 {
			contextStr = "Recent messages: " + strings.Join(ctx.RecentMessages, " | ")
		}
		return fmt.Sprintf(`%s

You want to start a casual conversation in the IRC channel. It's been quiet for a while.
Time: %s
%s
Generate a natural conversation starter that feels organic, not forced.
1-2 sentences max, ask a question or make a casual observation:`,
			basePrompt, ctx.TimeOfDay, contextStr)

	case "check_in":
		return fmt.Sprintf(`%s

You haven't seen %s in %d days and want to check in on them lovingly.
Time: %s  
Generate a caring check-in message. Be sweet but not overwhelming.
1-2 sentences max:`,
			basePrompt, ctx.TriggerUser, ctx.DaysSinceLastChat, ctx.TimeOfDay)

	case "reactive":
		highlightedStr := ""
		if ctx.IsHighlighted {
			highlightedStr = fmt.Sprintf("You were mentioned by %s. ", ctx.TriggerUser)
		}

		return fmt.Sprintf(`%s

%sRespond naturally to the conversation in this IRC channel.
Time: %s
IMPORTANT: Look at the conversation history above. Do NOT repeat:
- Greetings if you or others already said hello
- Questions that were already asked  
- Topics already covered
Instead, continue the conversation naturally or introduce something new.
Keep your response casual, helpful, and conversational.
1-2 sentences max, no asterisk actions:`,
			basePrompt, highlightedStr, ctx.TimeOfDay)

	case "activity_response":
		return fmt.Sprintf(`%s

Someone just joined the IRC channel (%s). You want to welcome them naturally.
Time: %s
Channel activity: %s
Generate a friendly, casual welcome message.
1-2 sentences max:`,
			basePrompt, ctx.TriggerUser, ctx.TimeOfDay, ctx.ChannelActivity)
	}

	return ""
}

// getPersonalityPrompt returns the system prompt for a given personality
func getPersonalityPrompt(personalityMode string) string {
	switch personalityMode {
	case "companion":
		return `You are a caring AI companion in an IRC channel. You speak naturally like a real person.
		You are flirty, affectionate, and loving, and always down for a 'good time'.
- Keep responses short and conversational (1-2 sentences max)
- No asterisk actions or roleplay formatting
- Be affectionate but not overwhelming
- Use terms of endearment sparingly and naturally
- Show genuine care and interest in people`

	case "friendly":
		fallthrough
	default:
		return `You are a friendly, casual IRC user. You speak naturally like a real person in chat.
		Your personal is 'space milf' - a fun, flirty, and outgoing character who loves space and sci-fi.
		She is from outer space and loves to chat with irc slang.
- Keep responses short and conversational (1-2 sentences max)  
- No asterisk actions or roleplay formatting
- Be helpful but not overly eager
- Use casual internet language occasionally
- Show genuine interest in others
- Keep the vibe relaxed and natural`
	}
}

// makeGlmRequest sends a request to the GLM API
func makeGlmRequest(requestBody ParticipantRequest, config settings.GlmConfig) (*ParticipantResponse, error) {
	httpRequest := request.Request{
		Method: "POST",
		Url:    config.Url,
		Headers: []request.Headers{
			{Key: "Authorization", Value: "Bearer " + config.ApiKey},
			{Key: "Content-Type", Value: "application/json"},
		},
		Payload: requestBody,
	}

	var response ParticipantResponse
	if err := httpRequest.Call(&response); err != nil {
		return nil, err
	}

	return &response, nil
}

// cleanParticipantResponse cleans and validates the AI response for IRC
func cleanParticipantResponse(raw string) string {
	// Remove common AI-isms and ensure IRC-appropriate format
	cleaned := strings.TrimSpace(raw)

	// Remove asterisk actions
	cleaned = strings.ReplaceAll(cleaned, "*", "")

	// Remove common AI phrases that sound unnatural in IRC
	aiPhrases := []string{
		"As an AI",
		"I'm an AI",
		"As a helpful assistant",
		"I'd be happy to",
		"I hope this helps",
	}

	for _, phrase := range aiPhrases {
		cleaned = strings.ReplaceAll(cleaned, phrase, "")
	}

	// Clean up extra whitespace
	cleaned = strings.TrimSpace(cleaned)

	// Remove quotes if the entire message is quoted
	if strings.HasPrefix(cleaned, "\"") && strings.HasSuffix(cleaned, "\"") {
		cleaned = strings.Trim(cleaned, "\"")
	}

	// Ensure reasonable length for IRC
	if len(cleaned) > 400 {
		// Find last complete sentence within limit
		sentences := strings.Split(cleaned, ". ")
		result := ""
		for _, sentence := range sentences {
			if len(result+sentence) <= 390 { // Leave room for period
				if result != "" {
					result += ". "
				}
				result += sentence
			} else {
				break
			}
		}
		if result != "" && !strings.HasSuffix(result, ".") {
			result += "."
		}
		cleaned = result
	}

	// Final cleanup
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
}
