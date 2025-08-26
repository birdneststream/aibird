package ircart

import (
	"aibird/logger"
	"fmt"
	"image/png"
	"os"
	"strings"
)

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
			
			// Add initial color reset for IRC client compatibility
			line.WriteString("\x03")

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

				// Add color codes only if they changed
				needsUpdate := fgColor != lastFgColor || bgColor != lastBgColor
				if needsUpdate {
					if fgColor == -1 {
						// No foreground color - use same as background for irssi compatibility
						line.WriteString(fmt.Sprintf("\x03%d,%d", bgColor, bgColor))
					} else if bgColor == fgColor {
						// Same fg/bg - use explicit format for irssi compatibility
						line.WriteString(fmt.Sprintf("\x03%d,%d", fgColor, bgColor))
					} else {
						// Different fg/bg - use compact format
						line.WriteString(fmt.Sprintf("\x03%d,%d", fgColor, bgColor))
					}
					lastFgColor = fgColor
					lastBgColor = bgColor
				}

				line.WriteString(char)
			}

			// Add color reset at end of line
			line.WriteString("\x03")
			ircLines = append(ircLines, line.String())
		}
	} else {
		// Full block rendering: each character represents one block
		for row := 0; row < blocksY; row++ {
			var line strings.Builder
			var lastColorCode = -1
			
			// Add initial color reset for IRC client compatibility
			line.WriteString("\x03")

			for col := 0; col < blocksX; col++ {
				// Sample pixel from the center of the block
				centerX := col * BLOCK_WIDTH + BLOCK_WIDTH/2
				centerY := row * BLOCK_HEIGHT + BLOCK_HEIGHT/2

				// Get color from the pre-quantized image (direct lookup)
				ircColor := GetIRCColorFromRGB(img.At(centerX, centerY))

				// Add background color code only if it changed
				if ircColor.Code != lastColorCode {
					// Use explicit fg,bg format for irssi compatibility
					line.WriteString(fmt.Sprintf("\x03%d,%d", ircColor.Code, ircColor.Code))
					lastColorCode = ircColor.Code
				}

				line.WriteString(" ")
			}

			// Add color reset at end of line
			line.WriteString("\x03")
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