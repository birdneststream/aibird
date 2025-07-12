package gemini

import (
	"aibird/irc/state"
	"aibird/settings"
	"aibird/text"
	"context"
	"errors"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// newClient creates and returns a new genai.Client
func newClient(ctx context.Context, apiKey string) (*genai.Client, error) {
	return genai.NewClient(ctx, option.WithAPIKey(apiKey))
}

// processResponse extracts the first text content part from the genai response.
func processResponse(resp *genai.GenerateContentResponse) (string, error) {
	if resp == nil || len(resp.Candidates) == 0 {
		return "", errors.New("no candidates found in response")
	}
	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					return string(txt), nil
				}
			}
		}
	}
	return "", errors.New("no text content found in response")
}

// toGenaiContent converts a slice of text.Message to a slice of genai.ContentHistory.
func toGenaiContent(messages []text.Message) (history []*genai.Content) {
	for _, msg := range messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		history = append(history, &genai.Content{
			Parts: []genai.Part{genai.Text(msg.Content)},
			Role:  role,
		})
	}
	return history
}

func Request(irc state.State) (string, error) {
	config := irc.Config.Gemini

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

	ctx := context.Background()
	client, err := newClient(ctx, config.ApiKey)
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash-lite-preview-06-17")
	chat := model.StartChat()

	// Get history and append the current message BEFORE sending
	chatHistory := text.GetChatCache(irc.UserAiChatCacheKey())
	chat.History = toGenaiContent(chatHistory)

	// Append the new user message to our cache first
	text.AppendChatCache(irc.UserAiChatCacheKey(), "user", message, irc.Config.AiBird.AiChatContextLimit)

	resp, err := chat.SendMessage(ctx, genai.Text(message))
	if err != nil {
		// If something fails, remove the user message we just added
		text.TruncateLastMessage(irc.UserAiChatCacheKey())
		return "", err
	}

	response, err := processResponse(resp)
	if err != nil {
		return "", err
	}

	// Append the assistant's response to our cache
	text.AppendChatCache(irc.UserAiChatCacheKey(), "assistant", response, irc.Config.AiBird.AiChatContextLimit)

	return response, nil
}

func SingleRequest(prompt string, config settings.GeminiConfig) (string, error) {
	ctx := context.Background()
	client, err := newClient(ctx, config.ApiKey)
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash-lite-preview-06-17")
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	return processResponse(resp)
}

func GenerateLyrics(message string, config settings.GeminiConfig) (string, error) {
	ctx := context.Background()
	client, err := newClient(ctx, config.ApiKey)
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash-lite-preview-06-17")
	systemPrompt, err := text.GetPrompt("lyrics.md")
	if err != nil {
		return "", err
	}
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text(systemPrompt),
		},
	}
	userPrompt := "Generate lyrics for a song about: " + message
	resp, err := model.GenerateContent(ctx, genai.Text(userPrompt))
	if err != nil {
		return "", err
	}

	return processResponse(resp)
}
