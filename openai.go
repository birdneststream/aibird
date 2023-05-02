package main

import (
	"log"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"

	gogpt "github.com/sashabaranov/go-openai"
	"github.com/yunginnanet/girc-atomic"
)

func completion(c *girc.Client, e girc.Event, message string, model string, cost float64) {
	var responseString string

	req := gogpt.CompletionRequest{
		Model:       model,
		MaxTokens:   config.OpenAI.Tokens,
		Prompt:      message,
		Temperature: config.OpenAI.Temperature,
	}

	// Process a completion request
	sendToIrc(c, e, "Processing: "+message)

	// Perform the actual API request to openAI
	resp, err := aiClient().CreateCompletion(ctx, req)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	// resp.Usage.TotalTokens / 1000 * cost
	total := strconv.FormatFloat((float64(resp.Usage.TotalTokens)/1000)*cost, 'f', 5, 64)

	responseString = resp.Choices[0].Text + " ($" + total + ")"

	sendToIrc(c, e, responseString)
}

// Annoying reply to chats
func replyToChats(c *girc.Client, e girc.Event, message string) {
	req := gogpt.CompletionRequest{
		Model:       gogpt.GPT3TextDavinci003,
		MaxTokens:   config.OpenAI.Tokens,
		Prompt:      "As an " + config.AiBird.ChatPersonality + " reply to the following irc chats: " + message + ".",
		Temperature: config.OpenAI.Temperature,
	}

	// Perform the actual API request to openAI
	resp, err := aiClient().CreateCompletion(ctx, req)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	sendToIrc(c, e, resp.Choices[0].Text)
}

func conversation(c *girc.Client, e girc.Event, model string, conversation []gogpt.ChatCompletionMessage) {
	sendToIrc(c, e, "Processing "+model+": "+conversation[len(conversation)-1].Content)

	req := gogpt.ChatCompletionRequest{
		Model:       model,
		MaxTokens:   config.OpenAI.Tokens,
		Messages:    conversation,
		Temperature: config.OpenAI.Temperature,
	}

	if model == gogpt.GPT4 {
		key := config.OpenAI.Gpt4Key
		resp, err := gogpt.NewClient(key).CreateChatCompletion(ctx, req)
		if err != nil {
			handleApiError(c, e, err)
			return
		}
		for _, choice := range resp.Choices {
			// for each ChatCompletionMessage
			sendToIrc(c, e, choice.Message.Content)
			return
		}
	} else {
		// Perform the actual API request to openAI
		resp, err := aiClient().CreateChatCompletion(ctx, req)
		if err != nil {
			handleApiError(c, e, err)
			return
		}
		for _, choice := range resp.Choices {
			// for each ChatCompletionMessage
			sendToIrc(c, e, choice.Message.Content)
			return
		}
	}

}

func chatGptContext(c *girc.Client, e girc.Event, name string, message []gogpt.ChatCompletionMessage) {
	req := gogpt.ChatCompletionRequest{
		Model:       gogpt.GPT3Dot5Turbo,
		MaxTokens:   config.OpenAI.Tokens,
		Messages:    message,
		Temperature: config.OpenAI.Temperature,
	}

	// Perform the actual API request to openAI
	resp, err := aiClient().CreateChatCompletion(ctx, req)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	// for each ChatCompletionChoice
	for _, choice := range resp.Choices {
		// for each ChatCompletionMessage
		sendToIrc(c, e, choice.Message.Content)

		key := []byte(name + "_" + e.Params[0] + "_chats_cache_gpt_" + e.Source.Name)
		message := "AI: " + choice.Message.Content
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		birdBase.Put(key, []byte(string(chatList)+"\n"+message))
	}
}

func dalle(c *girc.Client, e girc.Event, message string, size string) {
	req := gogpt.ImageRequest{
		Prompt: message,
		Size:   size,
		N:      1,
	}

	// Alert the irc chan that the bot is processing
	sendToIrc(c, e, "Processing Dall-E: "+message)

	resp, err := aiClient().CreateImage(ctx, req)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	daleResponse := saveDalleRequest(message, resp.Data[0].URL)

	sendToIrc(c, e, e.Source.Name+": "+daleResponse)
}

func saveDalleRequest(prompt string, url string) string {
	// Clean the filename, there has to be a better way to do this
	slug := cleanFileName(prompt)

	randValue := rand.Int63n(10000)
	// Place a random number on the end to (maybe almost) avoid overwriting duplicate requests
	fileName := slug + "_" + strconv.FormatInt(randValue, 4) + ".png"

	downloadFile(url, fileName)

	// append the current pwd to fileName
	fileName = filepath.Base(fileName)

	// download image
	content := fileHole("https://filehole.org/", fileName)

	return string(content)
}

func aiClient() *gogpt.Client {
	key := config.OpenAI.nextApiKey()
	whatKey = key
	return gogpt.NewClient(key)
}

func handleApiError(c *girc.Client, e girc.Event, err error) {
	sendToIrc(c, e, err.Error())

	// err.Error() contains You exceeded your current quota
	if strings.Contains(err.Error(), "You exceeded your current quota") {
		log.Println("Key " + whatKey + " has exceeded its quota")
	}
}
