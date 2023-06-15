package main

import (
	"bytes"
	"encoding/hex"
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
