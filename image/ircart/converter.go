package ircart

import (
	"aibird/image/exifparser"
	"aibird/logger"
	"fmt"
	"image/png"
	"os"
	"strings"
)

// ExtractOrConvertIRCArt attempts to extract IRC art from PNG EXIF data first,
// then falls back to pixel-based conversion if EXIF data is unavailable
func ExtractOrConvertIRCArt(pngFilePath string, useHalfblocks bool) ([]string, error) {
	logger.Debug("Attempting to extract IRC art from EXIF data", "file", pngFilePath)
	
	// First, try to extract IRC art from EXIF UserComment
	ircArtLines, err := exifparser.ExtractIRCArtFromPNG(pngFilePath)
	if err == nil && len(ircArtLines) > 0 {
		logger.Debug("Successfully extracted IRC art from EXIF", "lines", len(ircArtLines))
		return ircArtLines, nil
	}
	
	// Log the EXIF extraction attempt result
	logger.Debug("EXIF extraction failed, falling back to pixel conversion", "error", err)
	
	// Fall back to pixel-based conversion
	return ConvertPNGToIRCArt(pngFilePath, useHalfblocks)
}

// ConvertPNGToIRCArt converts a PNG file to IRC art format
// Expects a pre-processed image from ComfyUI with perfect 8x15 pixel blocks
// Returns a slice of strings, each representing one line of IRC art
func ConvertPNGToIRCArt(pngFilePath string, useHalfblocks bool) ([]string, error) {
	logger.Debug("Starting PNG to IRC art conversion (ComfyUI pre-processed)", "file", pngFilePath)

	// Open and decode the PNG file
	file, err := os.Open(pngFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PNG file: %w", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PNG file: %w", err)
	}

	bounds := img.Bounds()
	imgWidth := bounds.Max.X - bounds.Min.X
	imgHeight := bounds.Max.Y - bounds.Min.Y

	logger.Debug("Image loaded", "width", imgWidth, "height", imgHeight)

	// ComfyUI block dimensions - fixed and perfect
	const BLOCK_WIDTH = 8
	const BLOCK_HEIGHT = 15

	var ircLines []string
	
	// Calculate dimensions based on perfect blocks from ComfyUI
	blocksX := imgWidth / BLOCK_WIDTH
	var blocksY int
	if useHalfblocks {
		// Half blocks use 7.5 pixel height, so double the vertical resolution
		blocksY = imgHeight * 2 / BLOCK_HEIGHT
	} else {
		blocksY = imgHeight / BLOCK_HEIGHT
	}

	logger.Debug("Block calculations", "blocksX", blocksX, "blocksY", blocksY, "blockWidth", BLOCK_WIDTH, "blockHeight", BLOCK_HEIGHT)

	if useHalfblocks {
		// Halfblock rendering: each character represents two vertically stacked half blocks
		for row := 0; row < blocksY; row += 2 { // Process pairs of half-blocks
			var line strings.Builder
			var lastFgColor, lastBgColor = -1, -1

			for col := 0; col < blocksX; col++ {
				// Sample pixels from the center of each half-block
				topX := col * BLOCK_WIDTH + BLOCK_WIDTH/2
				topY := row * BLOCK_HEIGHT/2 + BLOCK_HEIGHT/4 // Center of top half-block
				
				bottomX := col * BLOCK_WIDTH + BLOCK_WIDTH/2  
				bottomY := (row+1) * BLOCK_HEIGHT/2 + BLOCK_HEIGHT/4 // Center of bottom half-block

				// Get colors from the pre-quantized image (direct lookup)
				topIrcColor := GetIRCColorFromRGB(img.At(topX, topY))
				
				var bottomIrcColor IRCColor
				if row+1 < blocksY {
					bottomIrcColor = GetIRCColorFromRGB(img.At(bottomX, bottomY))
				} else {
					// If we're at the last odd row, use the same color for bottom
					bottomIrcColor = topIrcColor
				}

				// Choose character and colors based on similarity
				var char string
				var fgColor, bgColor int

				if topIrcColor.Code == bottomIrcColor.Code {
					// Same color - use space with background color (more efficient)
					char = " "
					fgColor = -1 // No foreground color
					bgColor = topIrcColor.Code
				} else {
					// Different colors - use half block
					char = "▀" // Upper half block
					fgColor = topIrcColor.Code    // Foreground = top half
					bgColor = bottomIrcColor.Code // Background = bottom half
				}

				// Add color codes only if they changed (optimized for bytes)
				needsUpdate := fgColor != lastFgColor || bgColor != lastBgColor
				if needsUpdate {
					var actualFg int
					if fgColor == -1 {
						actualFg = bgColor // Use background color for foreground when no fg specified
					} else {
						actualFg = fgColor
					}
					
					// Use most compact format while maintaining irssi compatibility
					if actualFg < 10 && bgColor < 10 {
						// Both single digit - shortest format: \x035,7 (5 bytes)
						line.WriteString(fmt.Sprintf("\x03%d,%d", actualFg, bgColor))
					} else if actualFg < 10 {
						// Only fg is single digit: \x035,15 (6 bytes)  
						line.WriteString(fmt.Sprintf("\x03%d,%d", actualFg, bgColor))
					} else if bgColor < 10 {
						// Only bg is single digit: \x0315,5 (6 bytes)
						line.WriteString(fmt.Sprintf("\x03%d,%d", actualFg, bgColor))
					} else {
						// Both double digit: \x0315,20 (7 bytes)
						line.WriteString(fmt.Sprintf("\x03%d,%d", actualFg, bgColor))
					}
					lastFgColor = fgColor
					lastBgColor = bgColor
				}

				line.WriteString(char)
			}

			ircLines = append(ircLines, line.String())
		}
	} else {
		// Full block rendering: each character represents one block
		for row := 0; row < blocksY; row++ {
			var line strings.Builder
			var lastColorCode = -1

			for col := 0; col < blocksX; col++ {
				// Sample pixel from the center of the block
				centerX := col * BLOCK_WIDTH + BLOCK_WIDTH/2
				centerY := row * BLOCK_HEIGHT + BLOCK_HEIGHT/2

				// Get color from the pre-quantized image (direct lookup)
				ircColor := GetIRCColorFromRGB(img.At(centerX, centerY))

				// Add background color code only if it changed
				if ircColor.Code != lastColorCode {
					// Use most compact fg,bg format for irssi compatibility
					if ircColor.Code < 10 {
						// Single digit - shortest format: \x035,5 (5 bytes)
						line.WriteString(fmt.Sprintf("\x03%d,%d", ircColor.Code, ircColor.Code))
					} else {
						// Double digit: \x0315,15 (7 bytes)
						line.WriteString(fmt.Sprintf("\x03%d,%d", ircColor.Code, ircColor.Code))
					}
					lastColorCode = ircColor.Code
				}

				line.WriteString(" ")
			}

			ircLines = append(ircLines, line.String())
		}
	}

	logger.Debug("IRC art conversion completed", "lines", len(ircLines))
	return ircLines, nil
}

// FormatIRCArtForIRC formats IRC art lines for transmission over IRC
func FormatIRCArtForIRC(ircArtLines []string) []string {
	// The lines are already properly formatted, just return them
	return ircArtLines
}