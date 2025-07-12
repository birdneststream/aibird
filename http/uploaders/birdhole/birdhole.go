package birdhole

import (
	"aibird/http/request"
	"aibird/image"
	"aibird/settings"
	"encoding/json"
	"os"
	"strconv"
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
