package exifparser

import (
	"encoding/json"
	"fmt"
	"strings"

	pngstructure "github.com/dsoprea/go-png-image-structure/v2"

	"aibird/logger"
)

// IRCArtData represents the JSON structure containing IRC and ANSI art data
type IRCArtData struct {
	IRC  string `json:"irc"`
	ANSI string `json:"ansi"`
}

// ExtractIRCArtFromPNG extracts IRC art data from PNG comment chunks.
// It primarily looks for iTXt chunks with "Comment" keyword containing JSON data.
// Returns a slice of IRC art lines or an error if extraction fails.
func ExtractIRCArtFromPNG(filePath string) ([]string, error) {
	// Parse PNG structure to access chunks
	pmp := pngstructure.NewPngMediaParser()
	intfc, err := pmp.ParseFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PNG structure: %w", err)
	}

	// Extract IRC art from PNG text chunks (primary method)
	cs, ok := intfc.(*pngstructure.ChunkSlice)
	if !ok {
		return nil, fmt.Errorf("failed to access PNG chunks")
	}

	// Search through text chunks for Comment data
	for _, chunk := range cs.Chunks() {
		if isTextChunk(chunk.Type) {
			if lines, err := extractFromTextChunk(chunk); err == nil {
				return lines, nil
			}
		}
	}

	return nil, fmt.Errorf("no IRC art found in PNG")
}

// isTextChunk checks if a chunk type is a text chunk (tEXt, zTXt, or iTXt)
func isTextChunk(chunkType string) bool {
	return chunkType == "tEXt" || chunkType == "zTXt" || chunkType == "iTXt"
}

// extractFromTextChunk attempts to extract IRC art from a single text chunk
func extractFromTextChunk(chunk *pngstructure.Chunk) ([]string, error) {
	textContent := string(chunk.Data)

	// Check for Comment keyword (format: "Comment\0content")
	const commentPrefix = "Comment\x00"
	if !strings.HasPrefix(textContent, commentPrefix) {
		return nil, fmt.Errorf("chunk does not contain Comment keyword")
	}

	commentData := textContent[len(commentPrefix):]
	return parseIRCArtJSON(commentData)
}

// parseIRCArtJSON extracts IRC art from JSON-formatted comment data
func parseIRCArtJSON(commentData string) ([]string, error) {
	// Find JSON content (starts with '{')
	jsonStart := strings.Index(commentData, "{")
	if jsonStart < 0 {
		return nil, fmt.Errorf("no JSON found in comment data")
	}

	jsonContent := commentData[jsonStart:]

	// Parse JSON into IRCArtData structure
	var artData IRCArtData
	if err := json.Unmarshal([]byte(jsonContent), &artData); err != nil {
		return nil, fmt.Errorf("failed to parse IRC art JSON: %w", err)
	}

	if artData.IRC == "" {
		return nil, fmt.Errorf("IRC art data is empty")
	}

	logger.Debug("Successfully extracted IRC art from PNG", "size", len(artData.IRC))

	// Split IRC art into lines for transmission
	return strings.Split(artData.IRC, "\n"), nil
}
