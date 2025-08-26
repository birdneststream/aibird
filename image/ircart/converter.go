package ircart

import (
	"aibird/logger"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
)

const (
	// IRC art output dimensions - base dimensions for halfblock mode
	OUTPUT_WIDTH  = 60   // Width for halfblock mode
	OUTPUT_HEIGHT = 20   // Height for halfblock mode
)

// ConvertPNGToIRCArt converts a PNG file to IRC art format
// Returns a slice of strings, each representing one line of IRC art
func ConvertPNGToIRCArt(pngFilePath string, useHalfblocks bool) ([]string, error) {
	logger.Debug("Starting PNG to IRC art conversion", "file", pngFilePath)

	// Open and decode the PNG file
	file, err := os.Open(pngFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PNG file: %w", err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PNG: %w", err)
	}

	bounds := img.Bounds()
	imgWidth := bounds.Max.X - bounds.Min.X
	imgHeight := bounds.Max.Y - bounds.Min.Y

	logger.Debug("Image loaded", "width", imgWidth, "height", imgHeight)

	var ircLines []string
	
	// Adjust dimensions based on halfblock mode
	width := OUTPUT_WIDTH
	height := OUTPUT_HEIGHT
	if !useHalfblocks {
		// Use original doubled dimensions for non-halfblock mode  
		width = 80   // Double the original halfblock width (40 * 2)
		height = 28  // Double the original halfblock height (14 * 2)
	}

	if useHalfblocks {
		// Halfblock rendering: double vertical resolution
		blockWidth := float64(imgWidth) / float64(width)
		blockHeight := float64(imgHeight) / float64(height*2) // Double vertical resolution

		// Process each row of the output
		for row := 0; row < height; row++ {
			var line strings.Builder
			var lastFgColor, lastBgColor = -1, -1

			// Process each character position in the row
			for col := 0; col < width; col++ {
				// Calculate blocks for top and bottom half of this character
				startX := int(float64(col) * blockWidth)
				endX := int(float64(col+1) * blockWidth)
				
				// Top half
				topStartY := int(float64(row*2) * blockHeight)
				topEndY := int(float64(row*2+1) * blockHeight)
				
				// Bottom half  
				bottomStartY := int(float64(row*2+1) * blockHeight)
				bottomEndY := int(float64(row*2+2) * blockHeight)

				// Ensure we don't go out of bounds
				if endX > imgWidth {
					endX = imgWidth
				}
				if topEndY > imgHeight {
					topEndY = imgHeight
				}
				if bottomEndY > imgHeight {
					bottomEndY = imgHeight
				}

				// Analyze colors for top and bottom halves
				topColor := analyzeBlockColor(img, startX, topStartY, endX, topEndY)
				bottomColor := analyzeBlockColor(img, startX, bottomStartY, endX, bottomEndY)

				// Find closest IRC colors
				topIrcColor := FindClosestIRCColorFromColor(topColor)
				bottomIrcColor := FindClosestIRCColorFromColor(bottomColor)

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

				// Add color codes only if they changed - optimize for shorter codes
				needsUpdate := fgColor != lastFgColor || bgColor != lastBgColor
				if needsUpdate {
					if fgColor == -1 {
						// No foreground color - just background
						line.WriteString(fmt.Sprintf("\x03,%d", bgColor))
					} else if bgColor == fgColor {
						// Same fg/bg - just use foreground color (shorter)
						line.WriteString(FormatIRCColor(fgColor))
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
		// Original full-block rendering
		blockWidth := float64(imgWidth) / float64(width)
		blockHeight := float64(imgHeight) / float64(height)

		// Process each row of the output
		for row := 0; row < height; row++ {
			var line strings.Builder
			var lastColorCode = -1

			// Process each character position in the row
			for col := 0; col < width; col++ {
				// Calculate the source image block for this character position
				startX := int(float64(col) * blockWidth)
				endX := int(float64(col+1) * blockWidth)
				startY := int(float64(row) * blockHeight)
				endY := int(float64(row+1) * blockHeight)

				// Ensure we don't go out of bounds
				if endX > imgWidth {
					endX = imgWidth
				}
				if endY > imgHeight {
					endY = imgHeight
				}

				// Analyze the dominant color in this block
				dominantColor := analyzeBlockColor(img, startX, startY, endX, endY)
				ircColor := FindClosestIRCColorFromColor(dominantColor)

				// Add background color code only if it changed
				if ircColor.Code != lastColorCode {
					// Use background color with space character (no foreground color)
					line.WriteString(fmt.Sprintf("\x03,%d", ircColor.Code))
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

// analyzeBlockColor analyzes a rectangular block of pixels and returns the dominant color
// Uses weighted average focusing on center pixels for better detail preservation
func analyzeBlockColor(img image.Image, startX, startY, endX, endY int) color.Color {
	var totalR, totalG, totalB, totalWeight float64

	centerX := float64(startX+endX) / 2.0
	centerY := float64(startY+endY) / 2.0

	// Sample pixels in the block with center weighting
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Convert from 16-bit to 8-bit
			pixelR := float64(r >> 8)
			pixelG := float64(g >> 8)
			pixelB := float64(b >> 8)

			// Weight center pixels more heavily for better detail preservation
			distFromCenter := ((float64(x) - centerX) * (float64(x) - centerX)) + 
							 ((float64(y) - centerY) * (float64(y) - centerY))
			weight := 1.0 / (1.0 + distFromCenter*0.1)

			totalR += pixelR * weight
			totalG += pixelG * weight
			totalB += pixelB * weight
			totalWeight += weight
		}
	}

	// Return weighted average color if we have pixels
	if totalWeight > 0 {
		avgR := uint8(totalR / totalWeight)
		avgG := uint8(totalG / totalWeight)
		avgB := uint8(totalB / totalWeight)
		return &color.RGBA{R: avgR, G: avgG, B: avgB, A: 255}
	}

	// Fallback to black
	return &color.RGBA{R: 0, G: 0, B: 0, A: 255}
}

// FormatIRCArtForIRC formats the IRC art lines for sending to IRC
// Splits long lines if needed and handles IRC message length limits
func FormatIRCArtForIRC(ircLines []string) []string {
	var formattedLines []string

	for _, line := range ircLines {
		// IRC messages should be under 512 bytes, be more conservative with halfblocks
		// Account for IRC overhead (nick, channel, etc.) - use 350 char limit
		maxLength := 350
		
		if len(line) <= maxLength {
			formattedLines = append(formattedLines, line)
		} else {
			// Split long lines and try to preserve color formatting
			for len(line) > maxLength {
				// Try to split at a color code boundary if possible
				splitPoint := maxLength
				for i := maxLength - 10; i < maxLength && i < len(line); i++ {
					if line[i] == '\x03' {
						splitPoint = i
						break
					}
				}
				
				formattedLines = append(formattedLines, line[:splitPoint])
				line = line[splitPoint:]
			}
			if len(line) > 0 {
				formattedLines = append(formattedLines, line)
			}
		}
	}

	return formattedLines
}