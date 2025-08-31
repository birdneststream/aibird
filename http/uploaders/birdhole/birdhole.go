package birdhole

import (
	"encoding/json"
	"os"
	"strconv"

	"aibird/http/request"
	"aibird/image"
	"aibird/settings"
)

func BirdHole(fileName string, message string, fields []request.Fields, config settings.BirdholeConfig) (string, error) {
	fileName = image.ConvertPngToJpg(fileName)

	baseFields := []request.Fields{
		{Key: "urllen", Value: strconv.Itoa(config.UrlLen)},
		{Key: "expiry", Value: strconv.Itoa(config.Expiry)},
		{Key: "description", Value: message},
	}

	// Merge additional fields
	allFields := append(baseFields, fields...)

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
	err := birdHoleUpload.Call(&response)
	if err != nil {
		return "", err
	} else {
		var jsonResponse map[string]string
		err = json.Unmarshal([]byte(response), &jsonResponse)
		if err != nil {
			return "", err
		}

		_ = os.Remove(fileName)

		return jsonResponse["url"], nil
	}
}

// BirdHolePNG uploads a PNG file without converting to JPG, preserving PNG metadata
func BirdHolePNG(fileName string, message string, fields []request.Fields, config settings.BirdholeConfig) (string, error) {
	// Skip PNG to JPG conversion for aiscii to preserve metadata

	baseFields := []request.Fields{
		{Key: "urllen", Value: strconv.Itoa(config.UrlLen)},
		{Key: "expiry", Value: strconv.Itoa(config.Expiry)},
		{Key: "description", Value: message},
	}

	// Merge additional fields
	allFields := append(baseFields, fields...)

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
	err := birdHoleUpload.Call(&response)
	if err != nil {
		return "", err
	} else {
		var jsonResponse map[string]string
		err = json.Unmarshal([]byte(response), &jsonResponse)
		if err != nil {
			return "", err
		}

		_ = os.Remove(fileName)

		return jsonResponse["url"], nil
	}
}
