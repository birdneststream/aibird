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
		message += "."
	}

	return message
}

// readSanitizedFile reads a file from the given base directory after sanitizing
// the filename to prevent path traversal attacks. It validates the resolved path
// stays within the base directory.
func readSanitizedFile(baseDir, name string) (string, error) {
	// Reject path traversal in raw input before normalization
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return "", errors.New("invalid name: path separators or traversal detected")
	}

	// Normalize to just the filename component
	clean := filepath.Base(name)
	if clean == "." || clean == ".." {
		return "", errors.New("invalid name: empty or traversal")
	}

	// Resolve the base directory to an absolute path
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base directory: %w", err)
	}
	filePath := filepath.Join(absBaseDir, clean)

	// Verify the resolved path stays within the base directory
	resolvedPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}
	if !strings.HasPrefix(resolvedPath, absBaseDir) {
		return "", errors.New("invalid path: escapes base directory")
	}

	file, err := os.ReadFile(filePath) //nolint:gosec // G304: path sanitized by filepath.Base + prefix check above
	if err != nil {
		return "", err
	}

	return string(file), nil
}

func GetPersonalityFile(personality string) (string, error) {
	return readSanitizedFile("personalities", personality+".txt")
}

func GetPrompt(promptName string) (string, error) {
	return readSanitizedFile("text/prompts", promptName)
}
