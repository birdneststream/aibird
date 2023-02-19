package main

import (
	"context"
	"strconv"
	"strings"

	gogpt "github.com/sashabaranov/go-gpt3"
	"gopkg.in/irc.v3"
)

func completion(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context, model string, cost float64) {
	var responseString string

	req := gogpt.CompletionRequest{
		Model:       model,
		MaxTokens:   config.OpenAI.Tokens,
		Prompt:      message,
		Temperature: config.OpenAI.Temperature,
	}

	if model == gogpt.CodexCodeDavinci002 {
		req = gogpt.CompletionRequest{
			Model:            model,
			MaxTokens:        config.OpenAI.Tokens,
			Prompt:           message,
			Temperature:      0,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
		}
	}

	// Process a completion request
	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing: " + message,
		},
	})

	// Perform the actual API request to openAI
	resp, err := aiClient.CreateCompletion(ctx, req)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})
		return
	}

	// resp.Usage.TotalTokens / 1000 * cost
	total := strconv.FormatFloat((float64(resp.Usage.TotalTokens)/1000)*cost, 'f', 5, 64)

	responseString = strings.TrimSpace(resp.Choices[0].Text) + " ($" + total + ")"

	chunkToIrc(c, m, responseString)
}

func dalle(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context, size string) {
	req := gogpt.ImageRequest{
		Prompt: message,
		Size:   size,
		N:      1,
	}

	// Alert the irc chan that the bot is processing
	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing Dall-E: " + message,
		},
	})

	resp, err := aiClient.CreateImage(ctx, req)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})
		return
	}

	daleResponse := saveDalleRequest(message, resp.Data[0].URL)

	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			m.Prefix.Name + ": " + daleResponse,
		},
	})
}
