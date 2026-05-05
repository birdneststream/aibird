package exifparser

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aibird/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init(logger.Config{Level: "error", Format: "text"})
}

func TestIsTextChunk_ValidTypes(t *testing.T) {
	assert.True(t, isTextChunk("tEXt"))
	assert.True(t, isTextChunk("zTXt"))
	assert.True(t, isTextChunk("iTXt"))
}

func TestIsTextChunk_InvalidTypes(t *testing.T) {
	assert.False(t, isTextChunk("IHDR"))
	assert.False(t, isTextChunk("IDAT"))
	assert.False(t, isTextChunk("IEND"))
	assert.False(t, isTextChunk(""))
	assert.False(t, isTextChunk("random"))
}

func TestParseIRCArtJSON_Valid(t *testing.T) {
	input := `{"irc": "line1\nline2", "ansi": "ansi content"}`
	lines, err := parseIRCArtJSON(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"line1", "line2"}, lines)
}

func TestParseIRCArtJSON_MissingIRC(t *testing.T) {
	input := `{"ansi": "ansi content"}`
	_, err := parseIRCArtJSON(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IRC art data is empty")
}

func TestParseIRCArtJSON_InvalidJSON(t *testing.T) {
	_, err := parseIRCArtJSON("{not valid json}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse IRC art JSON")
}

func TestParseIRCArtJSON_EmptyJSON(t *testing.T) {
	_, err := parseIRCArtJSON("")
	require.Error(t, err)
}

func TestParseIRCArtJSON_NoJSONMarker(t *testing.T) {
	_, err := parseIRCArtJSON("no json here")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no JSON found")
}

func TestExtractIRCArtFromPNG_NonexistentFile(t *testing.T) {
	_, err := ExtractIRCArtFromPNG("/nonexistent/path/to/file.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse PNG structure")
}

func TestExtractIRCArtFromPNG_NotAPNG(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "not-a-png.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("this is not a png"), 0644))

	_, err := ExtractIRCArtFromPNG(tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse PNG structure")
}

func TestExtractIRCArtFromPNG_PNGWithoutTextChunks(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "minimal.png")

	// Create a minimal valid PNG using image/png package
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	f, err := os.Create(tmpFile)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())

	_, err = ExtractIRCArtFromPNG(tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no IRC art found in PNG")
}
