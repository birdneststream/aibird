package ircart

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

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
	m.Run()
}

func TestFindClosestIRCColor_ExactMatch(t *testing.T) {
	// Red appears twice in the palette: code 4 and code 52.
	// The lookup map stores the last occurrence, so code 52 wins.
	c := FindClosestIRCColor(255, 0, 0)
	assert.Equal(t, 52, c.Code)
	assert.Equal(t, uint8(255), c.R)
	assert.Equal(t, uint8(0), c.G)
	assert.Equal(t, uint8(0), c.B)
}

func TestFindClosestIRCColor_White(t *testing.T) {
	// White appears twice: code 0 and code 98. Code 98 wins in the map.
	c := FindClosestIRCColor(255, 255, 255)
	assert.Equal(t, 98, c.Code)
}

func TestFindClosestIRCColor_Black(t *testing.T) {
	// Black appears twice: code 1 and code 88. Code 88 wins in the map.
	c := FindClosestIRCColor(0, 0, 0)
	assert.Equal(t, 88, c.Code)
}

func TestFindClosestIRCColor_Fallback(t *testing.T) {
	// A color not in the palette should fall back to closest match
	c := FindClosestIRCColor(128, 128, 128)
	// 14 is {14, 127, 127, 127} — closest to 128,128,128
	assert.Equal(t, 14, c.Code)
}

func TestGetIRCColorFromRGB_ExactMatch(t *testing.T) {
	// Red maps to code 52 (last occurrence in palette)
	c := GetIRCColorFromRGB(color.RGBA{R: 255, G: 0, B: 0, A: 255})
	assert.Equal(t, 52, c.Code)
}

func TestGetIRCColorFromRGB_Fallback(t *testing.T) {
	c := GetIRCColorFromRGB(color.RGBA{R: 128, G: 128, B: 128, A: 255})
	assert.Equal(t, 14, c.Code)
}

func TestInitRGBLookupMap_Idempotent(t *testing.T) {
	// Reset the map to test idempotency
	rgbToIRCCodeMap = make(map[uint32]int)
	initRGBLookupMap()
	firstLen := len(rgbToIRCCodeMap)

	initRGBLookupMap()
	secondLen := len(rgbToIRCCodeMap)

	assert.Equal(t, firstLen, secondLen)
	// Palette has 99 entries but some RGB values are duplicated,
	// so the unique count in the map is less than 99.
	assert.Less(t, secondLen, len(IRCColorPalette))
	assert.Greater(t, secondLen, 0)
}

func createTestPNGForIRCArt(t *testing.T, dir string, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with red (IRC code 4)
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, red)
		}
	}

	filePath := filepath.Join(dir, "test_ircart.png")
	f, err := os.Create(filePath)
	require.NoError(t, err)
	defer f.Close()

	err = png.Encode(f, img)
	require.NoError(t, err)
	return filePath
}

func TestConvertPNGToIRCArt_FullBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a 16x30 image (2 blocks wide, 2 blocks tall)
	pngPath := createTestPNGForIRCArt(t, tmpDir, 16, 30)

	lines, err := ConvertPNGToIRCArt(pngPath, false)
	require.NoError(t, err)
	assert.Len(t, lines, 2)

	// Each line should contain IRC color codes and spaces
	for _, line := range lines {
		assert.NotEmpty(t, line)
		assert.Contains(t, line, "\x03")
	}
}

func TestConvertPNGToIRCArt_HalfBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	pngPath := createTestPNGForIRCArt(t, tmpDir, 16, 30)

	lines, err := ConvertPNGToIRCArt(pngPath, true)
	require.NoError(t, err)
	assert.NotEmpty(t, lines)

	for _, line := range lines {
		assert.NotEmpty(t, line)
		assert.Contains(t, line, "\x03")
	}
}

func TestExtractOrConvertIRCArt_EXIFFallback(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a plain PNG without EXIF IRC art data
	pngPath := createTestPNGForIRCArt(t, tmpDir, 16, 30)

	// Should fall back to ConvertPNGToIRCArt since there's no EXIF data
	lines, err := ExtractOrConvertIRCArt(pngPath, false)
	require.NoError(t, err)
	assert.NotEmpty(t, lines)
}
