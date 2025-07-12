package image

import (
	"aibird/logger"
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func ToJpeg(imageBytes []byte) ([]byte, error) {
	// DetectContentType detects the content type
	contentType := http.DetectContentType(imageBytes)

	switch contentType {
	case "image/png":
		// Decode the PNG image bytes
		img, err := png.Decode(bytes.NewReader(imageBytes))

		if err != nil {
			return nil, err
		}

		buf := new(bytes.Buffer)

		// encode the image as a JPEG file
		if err := jpeg.Encode(buf, img, nil); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("unable to convert %#v to jpeg", contentType)
}

func ConvertPngToJpg(fileName string) string {
	if strings.HasSuffix(fileName, ".png") {
		// Validate file path to prevent path traversal
		if strings.Contains(fileName, "..") || strings.Contains(fileName, "/") {
			logger.Error("Invalid file path", "file", fileName)
			return "Failed to convert PNG to JPG"
		}

		imageBytes, err := os.ReadFile(fileName)

		if err != nil {
			logger.Error("Failed to read image file", "file", fileName, "error", err)
			return "Failed to convert PNG to JPG"
		}

		// Convert the PNG image to JPEG
		jpegBytes, err := ToJpeg(imageBytes)

		if err != nil {
			logger.Error("Failed to convert image", "file", fileName, "error", err)
			return "Failed to convert PNG to JPG"
		}

		fileName = strings.TrimSuffix(fileName, ".png") + ".jpg"
		err = os.WriteFile(fileName, jpegBytes, 0600) // Use restrictive permissions

		if err != nil {
			logger.Error("Failed to write JPEG file", "file", fileName, "error", err)
			return "Failed to convert PNG to JPG"
		}

		_ = os.Remove(strings.TrimSuffix(fileName, ".jpg") + ".png")
	}

	return fileName
}

func ExtractURLs(input string) ([]string, error) {
	regex, err := regexp.Compile(`https?://[^\s/$.?#].[^\s]*`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile the regex: %v", err)
	}

	// Extract all URLs from input
	urls := regex.FindAllString(input, -1)

	return urls, nil
}

func IsImageURL(rawURL string) bool {
	// Validate URL to prevent SSRF attacks
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		logger.Error("Invalid URL scheme", "url", rawURL)
		return false
	}

	resp, err := http.Head(rawURL)
	if err != nil {
		logger.Error("Error checking image URL", "url", rawURL, "error", err)
		return false
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "image/")
}
