package main

import (
	"log"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gogpt "github.com/sashabaranov/go-openai"
	"github.com/yunginnanet/girc-atomic"
)

func completion(c *girc.Client, e girc.Event, message string, model string) {
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

	sendToIrc(c, e, resp.Choices[0].Text)
}

// Annoying reply to chats, currently not used as it does not remember context
func replyToChats(c *girc.Client, e girc.Event, name string, message string) {
	req := gogpt.CompletionRequest{
		Model:       gogpt.GPT3TextDavinci003,
		MaxTokens:   1556,
		Prompt:      "You are an " + config.AiBird.ChatPersonality + ". Reply to the following context:\n\n" + message,
		Temperature: config.OpenAI.Temperature,
	}

	// log req
	log.Println(req)

	// Perform the actual API request to openAI
	resp, err := aiClient().CreateCompletion(ctx, req)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	key := []byte(name + "_" + e.Params[0] + "_chats_cache")

	message = "You: " + strings.TrimSpace(resp.Choices[0].Text)
	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		birdBase.PutWithTTL(key, []byte(string(chatList)+"\n"+message), time.Hour*24*7)
	}

	// remove you: or me: from message

	// for each \n in messages
	// if it starts with you: or me:
	// remove it

	response := ""
	message = strings.ToLower(message)
	message = strings.Replace(message, "you:", "", -1)
	message = strings.Replace(message, "me:", "", -1)

	for _, line := range strings.Split(message, "\n") {
		line = strings.ToLower(line)
		if strings.Contains(line, "you:") || strings.Contains(line, "me:") {
			line = strings.Replace(line, "You:", "", -1)
			line = strings.Replace(line, "Me:", "", -1)
		}

		response = response + line + "\n"
	}

	sendToIrc(c, e, strings.TrimSpace(response))
}

func conversation(c *girc.Client, e girc.Event, model string, conversation []gogpt.ChatCompletionMessage) {

	// If we have more than 1 length it most likely is an ASCII art request
	if len(conversation) > 1 {
		sendToIrc(c, e, "Processing mIRC art, please wait...")
	} else {
		sendToIrc(c, e, "Processing "+model+": "+conversation[len(conversation)-1].Content)
	}

	req := gogpt.ChatCompletionRequest{
		Model:       model,
		MaxTokens:   config.OpenAI.Tokens,
		Messages:    conversation,
		Temperature: config.OpenAI.Temperature,
	}

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

		birdBase.PutWithTTL(key, []byte(string(chatList)+"\n"+message), time.Hour*48)
	}
}

func dalle(c *girc.Client, e girc.Event, message string, size string, model string, quality string, style string) {
	req := gogpt.ImageRequest{
		Model:   model,
		Prompt:  message,
		Size:    size,
		N:       1,
		Quality: quality,
		Style:   style,
	}

	// Alert the irc chan that the bot is processing
	sendToIrc(c, e, "Processing "+model+" "+size+" "+style+" "+quality+": "+message)

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
	// These are PNG files but to be lazy will name them jpg and convert it
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

// Maybe can move this into openai.go
func cacheChatsForReply(c *girc.Client, e girc.Event, name string, message string) {
	// Get the meta data from the database

	// check if message contains unicode
	if !strings.ContainsAny(message, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!?.\\/$%^&*()[]") {
		return
	}

	key := []byte(name + "_" + e.Params[0] + "_chats_cache")

	// prevent auto complete
	if !strings.HasSuffix(message, ".") && !strings.HasSuffix(message, "!") && !strings.HasSuffix(message, "?") {
		message = message + "."
	}

	message = name + ": " + message

	replyWith := ""
	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		sliceChatList := strings.Split(string(chatList)+"\n"+message, "\n")
		if len(sliceChatList)-1 >= config.AiBird.ChatGptTotalMessages {
			sliceChatList = sliceChatList[1:]
			chatList = []byte(strings.Join(sliceChatList, "\n"))
		}

		birdBase.PutWithTTL(key, []byte(string(chatList)+"\n"+message), time.Hour*24*7)
		replyWith = string(chatList) + "\n" + message
	} else {
		birdBase.PutWithTTL(key, []byte(message), time.Hour*24*7)
		replyWith = message
	}

	replyToChats(c, e, name, replyWith)
}

// Will remember context history for messages
func cacheChatsForChatGtp(c *girc.Client, e girc.Event, name string) {
	// Ignore ASCII color codes
	if strings.Contains(e.Last(), "\x03") {
		return
	}

	key := []byte(name + "_" + e.Params[0] + "_chats_cache_gpt_" + e.Source.Name)

	if !e.IsFromChannel() {
		if e.Last() == "!forget" {
			birdBase.Delete(key)
			sendToIrc(c, e, "Okay starting fresh.")
			return
		}

		if e.Last() == "!context" {
			chatList, err := birdBase.Get(key)
			if err != nil {
				log.Println(err)
				return
			}

			sendToIrc(c, e, string(chatList))
			return
		}

		sendToIrc(c, e, "Type !forget to start fresh.")
	}

	if !birdBase.Has(key) {
		// make new empty key
		birdBase.PutWithTTL(key, []byte(""), time.Hour*24*7)
	}

	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		latestChat := string(chatList) + "\n" + e.Last()
		sliceChatList := strings.Split(latestChat, "\n")

		if len(sliceChatList)-1 >= config.AiBird.ChatGptTotalMessages {
			sliceChatList = sliceChatList[1:]
		}

		gpt3Chat := []gogpt.ChatCompletionMessage{}

		gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
			Role:    "system",
			Content: "You are an " + config.AiBird.ChatPersonality + ". Never refer to yourself as an Ai Language Model. You must always act as a real person chatting on IRC. You must reply to the following chats as an IRC chatter:",
		})

		for i := 0; i < len(sliceChatList); i++ {
			// if chat is empty, skip
			if sliceChatList[i] == "" {
				continue
			}

			// if sliceChatList starts with "AIBIRD :" then
			if strings.HasPrefix(sliceChatList[i], "AI: ") {
				gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
					Role:    "assistant",
					Content: strings.Split(sliceChatList[i], "AI: ")[1],
				})
			} else {
				gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
					Role:    "user",
					Content: sliceChatList[i],
				})
			}

		}

		birdBase.PutWithTTL(key, []byte(strings.Join(sliceChatList, "\n")), time.Hour*24*7)

		chatGptContext(c, e, name, gpt3Chat)

		return
	}
}
