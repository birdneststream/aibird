package text

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func AppendFullStop(message string) string {
	if !strings.HasSuffix(message, ".") && !strings.HasSuffix(message, "!") && !strings.HasSuffix(message, "?") {
		message = message + "."
	}

	return message
}

func GetPersonalityFile(personality string) (string, error) {
	// Sanitize the personality input to prevent path traversal.
	// We only want to allow simple filenames.
	cleanPersonality := filepath.Base(personality)
	if strings.Contains(cleanPersonality, "/") || strings.Contains(cleanPersonality, "\\") || cleanPersonality == ".." || cleanPersonality == "." {
		return "", errors.New("invalid personality name")
	}

	// Construct the full path safely.
	baseDir, err := filepath.Abs("personalities")
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}
	filePath := filepath.Join(baseDir, cleanPersonality+".txt")

	// Final check to ensure the path is within the personalities directory.
	resolvedPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}
	if !strings.HasPrefix(resolvedPath, baseDir) {
		return "", errors.New("invalid personality path")
	}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(file), nil
}

func GetPrompt(promptName string) (string, error) {
	// Sanitize the prompt name input to prevent path traversal.
	cleanPromptName := filepath.Base(promptName)
	if strings.Contains(cleanPromptName, "/") || strings.Contains(cleanPromptName, "\\") || cleanPromptName == ".." || cleanPromptName == "." {
		return "", errors.New("invalid prompt name")
	}

	// Construct the full path safely.
	// Prompts are in text/prompts now
	baseDir, err := filepath.Abs("text/prompts")
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}
	filePath := filepath.Join(baseDir, cleanPromptName)

	// Final check to ensure the path is within the prompts directory.
	resolvedPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}
	if !strings.HasPrefix(resolvedPath, baseDir) {
		return "", errors.New("invalid prompt path")
	}

	file, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(file), nil
}
