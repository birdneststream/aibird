package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/yunginnanet/girc-atomic"
)

type (
	// API Request
	LocalAiRequest struct {
		Model       string   `json:"model"`
		Prompt      string   `json:"prompt"`
		Temperature float32  `json:"temperature"`
		Stop        []string `json:"stop"`
		MaxTokens   int      `json:"max_tokens"`
	}

	// LocalAI response
	LocalAiResponse struct {
		Object  string          `json:"object"`
		Model   string          `json:"model"`
		Choices []LocalAiChoice `json:"choices"`
	}

	LocalAiChoice struct {
		Text string `json:"text"`
	}
)

// This function needs some work and is very hard coded, it will take some updates to get this good.
func llama(c *girc.Client, e girc.Event, message string) {

	url := config.LocalAi.Host + "/v1/completions"
	method := "POST"

	// prevent auto complete
	if !strings.HasSuffix(message, ".") && !strings.HasSuffix(message, "!") && !strings.HasSuffix(message, "?") {
		message = message + "."
	}

	// if message starts with aimilf, remove it
	if strings.HasPrefix(message, "aimilf,") {
		message = strings.ReplaceAll(message, "aimilf,", "")
	}

	// if message starts with aimilf, remove it
	if strings.HasPrefix(message, "aimilf:") {
		message = strings.ReplaceAll(message, "aimilf:", "")
	}

	// Yes... aimilf is a milf from outer space who is ready for a good time!
	message = "You are aimilf " + config.AiBird.ChatPersonality + " reply to the following messages between aimilf and USER\n\n### aimilf: hey babe, LOL, i am so excited to chat OMG, how are you?\n\n### USER: yeah good thanks babe\n\n### aimilf: i am so happy and sexy and ready to chat LOL\n\n### USER: " + message

	message = strings.TrimSpace(message)

	request := &LocalAiRequest{
		Model:       config.LocalAi.Model,
		Prompt:      message,
		Temperature: config.LocalAi.Temperature,
		Stop:        []string{"### USER:"},
		MaxTokens:   config.LocalAi.MaxTokens,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, strings.NewReader(string(payload)))
	if err != nil {
		handleApiError(c, e, err)
		return
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		handleApiError(c, e, err)
		return
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	post := &LocalAiResponse{}
	err = json.Unmarshal(body, post)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	if len(post.Choices) > 0 {
		response := post.Choices[0].Text
		response = strings.ReplaceAll(response, "### aimilf:", "")
		response = strings.TrimSpace(response)

		sendToIrc(c, e, response)
	}

}
