package asciistore

import (
	"aibird/http/request"
	"aibird/logger"
	"fmt"
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
	artText := s.FormatArt(art)

	req := &request.Request{
		Url:     strings.TrimRight(recordingUrl, "/") + "/" + filename,
		Method:  "POST",
		Payload: artText,
	}

	// Add content-type header for text
	req.AddHeader("Content-Type", "text/plain")

	logger.Debug("Recording ASCII art", "url", req.Url, "filename", filename, "user", user)

	var response string
	err := req.Call(&response)
	if err != nil {
		logger.Error("Failed to record ASCII art", "error", err, "url", req.Url)
		return "Failed to record art :(", err
	}

	// Clear the stored ASCII art after successful recording to prevent duplicates
	s.Clear(user, network, channel)

	logger.Info("Successfully recorded ASCII art", "filename", filename, "user", user, "response", response)
	logger.Debug("Cleared stored ASCII art after recording", "user", user, "network", network, "channel", channel)
	return "Art saved to " + response, nil
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

func (s *ASCIIStore) FormatArt(art *ASCIIArt) string {
	var sb strings.Builder

	// Add metadata as comments
	sb.WriteString("# Generated ASCII Art\n")
	sb.WriteString("# Prompt: " + art.Prompt + "\n")
	sb.WriteString(fmt.Sprintf("# Created by: %s on %s %s\n", art.User, art.Network, art.Channel))
	sb.WriteString("# Created at: " + art.Timestamp.Format("2006-01-02 15:04:05") + "\n")
	sb.WriteString("\n")

	// Add ASCII art lines
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
