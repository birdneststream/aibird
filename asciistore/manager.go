package asciistore

import (
	"aibird/logger"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

var (
	instance *ASCIIStore
	once     sync.Once
)

func GetManager() *ASCIIStore {
	once.Do(func() {
		instance = NewASCIIStore()
	})
	return instance
}

func (s *ASCIIStore) RecordToService(user, network, channel, recordingUrl string) (string, error) {
	art, exists := s.Retrieve(user, network, channel)
	if !exists {
		return "No ASCII art found. Generate some art with !aiscii first.", fmt.Errorf("no ASCII art found for user %s", user)
	}

	if recordingUrl == "" {
		return "Recording URL not configured.", fmt.Errorf("recording URL not configured")
	}

	filename := s.GenerateFilename(art)
	artText := s.FormatArtForRecording(art)

	// Create raw HTTP request to avoid JSON encoding
	url := strings.TrimRight(recordingUrl, "/") + "/" + filename
	
	// Create HTTP request with raw text body
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader([]byte(artText)))
	if err != nil {
		return "Failed to create request", err
	}
	
	httpReq.Header.Set("Content-Type", "text/plain")
	
	logger.Debug("Recording ASCII art", "url", url, "filename", filename, "user", user)
	
	// Execute the request
	client := &http.Client{}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		logger.Error("Failed to record ASCII art", "error", err, "url", url)
		return "Failed to record art :(", err
	}
	defer httpResp.Body.Close()

	// Read response body
	var responseBody []byte
	responseBody, err = io.ReadAll(httpResp.Body)
	if err != nil {
		logger.Error("Failed to read response", "error", err)
		return "Failed to read response", err
	}
	
	responseText := string(responseBody)

	// Clear the stored ASCII art after successful recording to prevent duplicates
	s.Clear(user, network, channel)

	logger.Info("Successfully recorded ASCII art", "filename", filename, "user", user, "response", responseText)
	logger.Debug("Cleared stored ASCII art after recording", "user", user, "network", network, "channel", channel)
	return "Art saved to " + responseText, nil
}

func (s *ASCIIStore) GenerateFilename(art *ASCIIArt) string {
	// Sanitize prompt for filename
	filename := s.sanitizeForFilename(art.Prompt)
	if filename == "" {
		filename = "ascii-art"
	}

	// Add timestamp suffix to ensure uniqueness
	timestamp := art.Timestamp.Format("20060102-150405")
	return filename + "-" + timestamp
}

func (s *ASCIIStore) FormatArtForRecording(art *ASCIIArt) string {
	var sb strings.Builder

	// The lines are already properly formatted from FormatIRCArtForIRC, just join them
	for _, line := range art.Lines {
		sb.WriteString(line + "\n")
	}

	return sb.String()
}

func (s *ASCIIStore) sanitizeForFilename(input string) string {
	// Convert to lowercase
	filename := strings.ToLower(input)

	// Replace spaces with dashes
	filename = strings.ReplaceAll(filename, " ", "-")

	// Remove special characters except alphanumeric and dashes
	reg := regexp.MustCompile(`[^a-z0-9\-]`)
	filename = reg.ReplaceAllString(filename, "")

	// Limit length to 32 characters (leaving room for timestamp suffix)
	if len(filename) > 32 {
		filename = filename[:32]
	}

	// Remove leading/trailing dashes
	filename = strings.Trim(filename, "-")

	return filename
}
