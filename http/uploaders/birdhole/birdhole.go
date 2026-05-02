package birdhole

import (
	"encoding/json"
	"os"
	"strconv"

	"aibird/http/request"
	"aibird/image"
	"aibird/settings"
)

// BirdHole uploads a file, converting PNG to JPG first.
func BirdHole(fileName string, message string, fields []request.Fields, config settings.BirdholeConfig) (string, error) {
	return upload(fileName, message, fields, config, true)
}

// BirdHolePNG uploads a PNG file without converting to JPG, preserving metadata.
func BirdHolePNG(fileName string, message string, fields []request.Fields, config settings.BirdholeConfig) (string, error) {
	return upload(fileName, message, fields, config, false)
}

// upload is the shared implementation for BirdHole and BirdHolePNG.
func upload(fileName string, message string, fields []request.Fields, config settings.BirdholeConfig, convertToJpg bool) (string, error) {
	if convertToJpg {
		fileName = image.ConvertPngToJpg(fileName)
	}

	allFields := append([]request.Fields{
		{Key: "urllen", Value: strconv.Itoa(config.UrlLen)},
		{Key: "expiry", Value: strconv.Itoa(config.Expiry)},
		{Key: "description", Value: message},
	}, fields...)

	birdHoleUpload := request.Request{
		Url:    config.Host + ":" + config.Port + config.EndPoint,
		Method: "POST",
		Headers: []request.Headers{
			{Key: "X-Auth-Token", Value: config.Key},
		},
		Fields:   allFields,
		FileName: fileName,
	}

	var response string
	if err := birdHoleUpload.Call(&response); err != nil {
		return "", err
	}

	var jsonResponse map[string]string
	if err := json.Unmarshal([]byte(response), &jsonResponse); err != nil {
		return "", err
	}

	_ = os.Remove(fileName)

	return jsonResponse["url"], nil
}
