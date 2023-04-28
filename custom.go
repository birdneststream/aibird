package main

import (
	"strings"

	gogpt "github.com/sashabaranov/go-gpt3"
	"github.com/yunginnanet/girc-atomic"
)

func birdmap(e girc.Event, message string, c *girc.Client, aiClient *gogpt.Client) {
	prompt := "Simulate an nmap scan of host " + message + " and return the results. The nmap results must include funny bird names for unix services. For example 'SecureSeedStorage' and 'SparrowSecureSSH."

	req := gogpt.CompletionRequest{
		Model:            gogpt.GPT3TextDavinci003,
		MaxTokens:        config.OpenAI.Tokens,
		Prompt:           prompt,
		Temperature:      0.87,
		FrequencyPenalty: 0,
		PresencePenalty:  0,
	}

	_ = c.Cmd.Reply(e, "Running birdmap scan for: "+message+" please wait...")

	resp, err := aiClient.CreateCompletion(ctx, req)

	if err != nil {
		_ = c.Cmd.Reply(e, err.Error())
		return
	}

	responseString := strings.TrimSpace(resp.Choices[0].Text)
	for _, line := range strings.Split(responseString, "\n") {
		// Write the final message
		chunkToIrc(c, e, line)
	}
}

// aiscii function, hopefully will prevent ping timeouts
func aiscii(e girc.Event, message string, c *girc.Client, aiClient *gogpt.Client) {
	var asciiName string // ai generated name
	var responseString string

	parts := strings.SplitN(message, " ", 2)

	if parts[0] == "--save" {
		message = parts[1]
	}

	prompt := "Use the UTF-8 drawing characters and mIRC color codes (using ) to make a monospaced text art 80 characters wide and 30 characters height depicting '" + message + "'."

	req := gogpt.CompletionRequest{
		Model:            gogpt.GPT3TextDavinci003,
		MaxTokens:        config.OpenAI.Tokens,
		Prompt:           prompt,
		Temperature:      0.87,
		FrequencyPenalty: 0,
		PresencePenalty:  0,
	}

	_ = c.Cmd.Reply(e, "Processing mIRC aiscii art (it can take a while): "+message)

	resp, err := aiClient.CreateCompletion(ctx, req)

	if err != nil {
		_ = c.Cmd.Reply(e, err.Error())
		return
	}

	responseString = strings.TrimSpace(resp.Choices[0].Text)

	if parts[0] == "--save" {
		message = parts[1]
		// Generate a title for the art
		req = gogpt.CompletionRequest{
			Model:            gogpt.GPT3TextDavinci002,
			MaxTokens:        128,
			Prompt:           "Write a short three word title for your mirc ascii art based on '" + message + "'. Use only alphabetical characters and spaces only.",
			Temperature:      0.8,
			TopP:             1,
			FrequencyPenalty: 0.6,
			PresencePenalty:  0.3,
		}

		resp, err := aiClient.CreateCompletion(ctx, req)
		if err != nil {
			_ = c.Cmd.Reply(e, err.Error())
			return
		}
		asciiName = strings.TrimSpace(resp.Choices[0].Text)

		// get alphabet letters from asciiName only
		asciiName := cleanFileName(asciiName)

		_ = c.Cmd.Reply(e, "@record "+asciiName)
	}

	// for each new line break in response choices write to channel
	for _, line := range strings.Split(responseString, "\n") {
		// Write the final message
		_ = c.Cmd.Reply(e, line)
	}

	message = "As a snobby reddit intellectual artist, shortly explain your new artistic masterpiece '" + message + "'" + " to the masses."

	req = gogpt.CompletionRequest{
		Model:       gogpt.GPT3TextDavinci002,
		MaxTokens:   256,
		Prompt:      message,
		Temperature: 1.1,
	}

	resp, err = aiClient.CreateCompletion(ctx, req)
	if err != nil {
		_ = c.Cmd.Reply(e, err.Error())
		return
	}

	responseString = strings.TrimSpace(resp.Choices[0].Text)

	chunkToIrc(c, e, responseString)

	if parts[0] == "--save" {
		_ = c.Cmd.Reply(e, "@end")
	}
}
