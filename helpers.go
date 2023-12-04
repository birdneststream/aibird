package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/sha3"
)

func cleanFileName(fileName string) string {
	if len(fileName) > 220 {
		fileName = fileName[:220]
	}

	fileName = strings.ReplaceAll(fileName, " ", "-")
	fileName = strings.ReplaceAll(fileName, "/", "-")
	fileName = strings.ReplaceAll(fileName, "\\", "-")
	fileName = strings.ReplaceAll(fileName, ":", "-")
	fileName = strings.ReplaceAll(fileName, "*", "-")
	fileName = strings.ReplaceAll(fileName, "?", "-")
	fileName = strings.ReplaceAll(fileName, "\"", "-")
	fileName = strings.ReplaceAll(fileName, "<", "-")
	fileName = strings.ReplaceAll(fileName, ">", "-")
	fileName = strings.ReplaceAll(fileName, "|", "-")
	fileName = strings.ReplaceAll(fileName, ".", "-")
	fileName = strings.ReplaceAll(fileName, ",", "-")
	fileName = strings.ReplaceAll(fileName, ";", "-")
	fileName = strings.ReplaceAll(fileName, "'", "-")
	fileName = strings.ReplaceAll(fileName, "!", "-")
	fileName = strings.ReplaceAll(fileName, "@", "-")
	fileName = strings.ReplaceAll(fileName, "#", "-")
	fileName = strings.ReplaceAll(fileName, "$", "-")
	fileName = strings.ReplaceAll(fileName, "%", "-")
	fileName = strings.ReplaceAll(fileName, "^", "-")
	fileName = strings.ReplaceAll(fileName, "&", "-")
	fileName = strings.ReplaceAll(fileName, "(", "-")
	fileName = strings.ReplaceAll(fileName, ")", "-")
	fileName = strings.ReplaceAll(fileName, "_", "-")
	fileName = strings.ReplaceAll(fileName, "=", "-")
	fileName = strings.ReplaceAll(fileName, "+", "-")
	fileName = strings.ReplaceAll(fileName, "`", "-")
	fileName = strings.ReplaceAll(fileName, "~", "-")
	fileName = strings.ReplaceAll(fileName, "[", "-")
	fileName = strings.ReplaceAll(fileName, "]", "-")
	fileName = strings.ReplaceAll(fileName, "{", "-")
	fileName = strings.ReplaceAll(fileName, "}", "-")

	return strings.ToLower(fileName)
}

func cleanArtName(fileName string) string {
	return strings.Trim(strings.ReplaceAll(fileName, "--", "-"), "-")
}

func recordArt(fileName string, art string) (string, bool) {
	url := config.RecordingUrl
	if url == "" {
		fmt.Println("recording url not configured so not saving art.")
		return "", false
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", strings.TrimRight(url, "/")+"/"+fileName, strings.NewReader(art))
	if err != nil {
		fmt.Println(err)
		return "failed to record art :(", false
	}
	res, err := client.Do(req)
	if err != nil || res.StatusCode != 200 {
		fmt.Println(err)
		return "failed to record art :(", false
	}
	body, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	if err != nil {
		fmt.Println(err)
		return "maybe failed to record art? try " + fileName + " :(", false
	}
	return "art saved to " + string(body), true
}

func fileHole(url string, fileName string) string {
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	file, errFile1 := os.Open(fileName)
	defer file.Close()
	part1,
		errFile1 := writer.CreateFormFile("file", filepath.Base(fileName))
	_, errFile1 = io.Copy(part1, file)
	if errFile1 != nil {
		fmt.Println(errFile1)

	}
	_ = writer.WriteField("expiry", "432000")
	_ = writer.WriteField("url_len", "5")
	err := writer.Close()
	if err != nil {
		fmt.Println(err)

	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)

	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)

	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)

	}
	fmt.Println(string(body))

	return string(body)
}

func downloadFile(URL, fileName string) error {
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.New("Received non 200 response code")
	}
	//Create a empty file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the fiel
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}

func cacheKey(key string, what string) []byte {
	hash := sha3.Sum224([]byte(key))
	hashString := hex.EncodeToString(hash[:])

	return []byte(what + hashString)
}

// This one doesn't rely on e.Params which can change depending on what event has occurred.
func isInList(name string, channel string, what string, user string, host string) bool {
	return birdBase.Has(cacheKey(name+channel+user+host, what))
}

// Remove user modes from nicks
func cleanFromModes(nick string) string {
	nick = strings.ReplaceAll(nick, "@", "")
	nick = strings.ReplaceAll(nick, "+", "")
	nick = strings.ReplaceAll(nick, "~", "")
	nick = strings.ReplaceAll(nick, "&", "")
	nick = strings.ReplaceAll(nick, "%", "")
	return nick
}

func pasteEe(message string, name string) string {

	if config.AiBird.PasteEeKey == "" {
		return ""
	}

	url := "https://api.paste.ee/v1/pastes"
	method := "POST"

	type PasteEe struct {
		Description string `json:"description"`
		Sections    []struct {
			Name     string `json:"name"`
			Syntax   string `json:"syntax"`
			Contents string `json:"contents"`
		} `json:"sections"`
	}

	// convert struct to json
	pasteEe := PasteEe{
		Description: name,
		Sections: []struct {
			Name     string `json:"name"`
			Syntax   string `json:"syntax"`
			Contents string `json:"contents"`
		}{
			{
				Name:     name,
				Syntax:   "text",
				Contents: message,
			},
		},
	}

	pasteEeJson, err := json.Marshal(pasteEe)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	payload := bytes.NewBuffer(pasteEeJson)

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return ""
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Auth-Token", config.AiBird.PasteEeKey)
	req.Header.Add("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	// {"id":"wGSc7","link":"https:\/\/paste.ee\/p\/wGSc7","success":true}
	type PasteEeResponse struct {
		Id      string `json:"id"`
		Link    string `json:"link"`
		Success bool   `json:"success"`
	}

	pasteEeResponse := PasteEeResponse{}
	err = json.Unmarshal(body, &pasteEeResponse)

	if err != nil {
		fmt.Println(err)
		return ""
	}

	if pasteEeResponse.Success {
		return pasteEeResponse.Link
	}

	return ""
}
