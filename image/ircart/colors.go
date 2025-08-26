package ircart

import (
	"fmt"
	"image/color"
)

// IRCColor represents an IRC color with its code and RGB values
type IRCColor struct {
	Code int
	R, G, B uint8
}

// IRCColorPalette contains the exact 99 IRC colors used by the ComfyUI node
var IRCColorPalette = []IRCColor{
	{0, 255, 255, 255}, {1, 0, 0, 0}, {2, 0, 0, 255}, {3, 0, 255, 0}, {4, 255, 0, 0},
	{5, 165, 42, 42}, {6, 255, 0, 255}, {7, 255, 165, 0}, {8, 255, 255, 0}, {9, 144, 238, 144},
	{10, 0, 255, 255}, {11, 173, 216, 230}, {12, 173, 216, 255}, {13, 255, 192, 203}, {14, 128, 128, 128},
	{15, 211, 211, 211}, {16, 71, 0, 0}, {17, 71, 33, 0}, {18, 71, 71, 0}, {19, 50, 71, 0},
	{20, 0, 71, 0}, {21, 0, 71, 44}, {22, 0, 71, 71}, {23, 0, 39, 71}, {24, 0, 0, 71},
	{25, 46, 0, 71}, {26, 71, 0, 71}, {27, 71, 0, 42}, {28, 116, 0, 0}, {29, 116, 58, 0},
	{30, 116, 116, 0}, {31, 81, 116, 0}, {32, 0, 116, 0}, {33, 0, 116, 73}, {34, 0, 116, 116},
	{35, 0, 64, 116}, {36, 0, 0, 116}, {37, 75, 0, 116}, {38, 116, 0, 116}, {39, 116, 0, 69},
	{40, 181, 0, 0}, {41, 181, 99, 0}, {42, 181, 181, 0}, {43, 125, 181, 0}, {44, 0, 181, 0},
	{45, 0, 181, 113}, {46, 0, 181, 181}, {47, 0, 99, 181}, {48, 0, 0, 181}, {49, 117, 0, 181},
	{50, 181, 0, 181}, {51, 181, 0, 107}, {52, 255, 0, 0}, {53, 255, 140, 0}, {54, 255, 255, 0},
	{55, 178, 255, 0}, {56, 0, 255, 0}, {57, 0, 255, 160}, {58, 0, 255, 255}, {59, 0, 140, 255},
	{60, 0, 0, 255}, {61, 165, 0, 255}, {62, 255, 0, 255}, {63, 255, 0, 152}, {64, 255, 89, 89},
	{65, 255, 180, 89}, {66, 255, 255, 113}, {67, 207, 255, 96}, {68, 111, 255, 111}, {69, 101, 255, 201},
	{70, 109, 255, 255}, {71, 89, 180, 255}, {72, 89, 89, 255}, {73, 196, 89, 255}, {74, 255, 102, 255},
	{75, 255, 89, 188}, {76, 255, 156, 156}, {77, 255, 211, 156}, {78, 255, 255, 156}, {79, 226, 255, 156},
	{80, 156, 255, 156}, {81, 156, 255, 219}, {82, 156, 255, 255}, {83, 156, 211, 255}, {84, 156, 156, 255},
	{85, 220, 156, 255}, {86, 255, 156, 255}, {87, 255, 148, 211}, {88, 0, 0, 0}, {89, 19, 19, 19},
	{90, 40, 40, 40}, {91, 54, 54, 54}, {92, 77, 77, 77}, {93, 101, 101, 101}, {94, 129, 129, 129},
	{95, 159, 159, 159}, {96, 188, 188, 188}, {97, 226, 226, 226}, {98, 255, 255, 255},
}


// rgbToIRCCodeMap creates a direct lookup map from RGB values to IRC color codes
var rgbToIRCCodeMap = make(map[uint32]int)

// initRGBLookupMap initializes the direct RGB to IRC code mapping
func initRGBLookupMap() {
	if len(rgbToIRCCodeMap) > 0 {
		return // Already initialized
	}
	
	for _, ircColor := range IRCColorPalette {
		// Pack RGB into a uint32 for fast lookup: (R << 16) | (G << 8) | B
		rgbKey := uint32(ircColor.R)<<16 | uint32(ircColor.G)<<8 | uint32(ircColor.B)
		rgbToIRCCodeMap[rgbKey] = ircColor.Code
	}
}

// FindClosestIRCColor finds IRC color using direct RGB lookup (ComfyUI pre-quantizes to exact colors)
func FindClosestIRCColor(r, g, b uint8) IRCColor {
	initRGBLookupMap()
	
	// Pack RGB into lookup key
	rgbKey := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	
	// Direct lookup - should always find exact match since ComfyUI quantized to these colors
	if colorCode, exists := rgbToIRCCodeMap[rgbKey]; exists {
		return IRCColor{Code: colorCode, R: r, G: g, B: b}
	}
	
	// Fallback to closest match (shouldn't be needed with quantized input)
	// Use simple RGB distance for speed
	minDistance := float64(999999)
	var closestColor IRCColor
	
	for _, ircColor := range IRCColorPalette {
		dr := float64(r) - float64(ircColor.R)
		dg := float64(g) - float64(ircColor.G)
		db := float64(b) - float64(ircColor.B)
		distance := dr*dr + dg*dg + db*db // Squared distance for speed

		if distance < minDistance {
			minDistance = distance
			closestColor = ircColor
		}
	}

	return closestColor
}

// FindClosestIRCColorFromColor finds the closest IRC color from a Go color.Color
func FindClosestIRCColorFromColor(c color.Color) IRCColor {
	r, g, b, _ := c.RGBA()
	// Convert from 16-bit to 8-bit
	return FindClosestIRCColor(uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// FormatIRCColor formats an IRC color code for use in IRC messages
// Returns the control character (0x03) followed by the color code
func FormatIRCColor(colorCode int) string {
	return fmt.Sprintf("\x03%d", colorCode)
}