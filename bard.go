package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/yunginnanet/girc-atomic"
)

type (
	BardRequest struct {
		SessionId string `json:"session_id"`
		Message   string `json:"message"`
	}

	BardResponse struct {
		Content string `json:"content"`
	}
)

func bard(c *girc.Client, e girc.Event, message string) {
	saveToPastEe := false

	if strings.Contains(message, "--save") {
		message = strings.Replace(message, "--save", "", -1)
		saveToPastEe = true
	}

	sendToIrc(c, e, "Processing Google Bard: "+message+"...")

	url := config.Bard.Host + "/ask"
	method := "POST"

	request := BardRequest{
		SessionId: config.Bard.SessionId,
		Message:   message,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	log.Println(string(payload))

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

	post := &BardResponse{}
	err = json.Unmarshal(body, post)
	if err != nil {
		handleApiError(c, e, err)
		return
	}

	if post.Content != "" {
		response := post.Content
		response = strings.TrimSpace(response)

		pasteEeLink := ""
		if saveToPastEe {
			pasteEeLink = "Saved to: " + pasteEe(response, message)
		}

		sendToIrc(c, e, response+"\n"+pasteEeLink)
	}
}
