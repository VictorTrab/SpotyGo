package ui

import (
	"fmt"
	"math"
)

// This file owns the reactive application canvas.
//
// Theme independence is deliberate: only the "dark" mode consumes the palette
// background (theme.BgBase). The "flow", "gradient" and "default" canvases own
// their colors, so switching themes restyles text, accent, borders and pills
// without ever flattening or recoloring the animated backdrop.
//
// Contrast is preserved with a single WCAG luminance ceiling applied identically
// to every row. An earlier version scrimmed only the rows carrying UI content,
// which kept text legible but drew a visible dark band behind cards and lists;
// a uniform ceiling keeps the same guarantee with one continuous gradient.

const (
	// flowFloor is the deep canvas tone the ambient gradient breathes over. It is
	// intentionally independent from the theme palette.
	flowFloor = "#0a0a12"

	// MinTextContrast and MinMutedContrast are the WCAG ratios the canvas honors
	// for the theme's primary and muted foregrounds.
	MinTextContrast  = 4.5
	MinMutedContrast = 4.5

	// canvasLuminance caps the luminance of every canvas row. It is a fixed value
	// (not derived from the theme) so the flow never changes with the palette, and
	// it is low enough that even the dimmest shipped muted color (#CBD5E1) keeps
	// 4.5:1 contrast. Text therefore stays readable no matter which album color
	// drives the gradient.
	canvasLuminance = 0.10

	// minPaletteSaturation rejects washed-out artwork colors. A white or grey
	// cover would otherwise turn the animated canvas into a flat grey surface.
	minPaletteSaturation = 40

	// flowCycleFrames is the ambient breathing period in Zen animation frames (~7s).
	flowCycleFrames = 60.0
)

// Canvas paints the reactive application background for a single frame.
type Canvas struct {
	Mode  string
	Top   string
	Sec   string
	Floor string

	Frame int

	flow bool
}

// NewCanvas builds the background painter for the current frame.
func NewCanvas(mode string, cover *CoverData, themeBg, themeText, themeMuted string, frame int) Canvas {
	c := Canvas{Mode: mode, Frame: frame}
	switch mode {
	case "dark":
		bg := themeBg
		if bg == "" {
			bg = "#0c0d0e"
		}
		c.Top, c.Sec, c.Floor = bg, bg, bg
	case "flow":
		c.Top, c.Sec, c.Floor = flowPalette(cover)
		c.flow = true
	default: // "default", "gradient"
		c.Top, c.Sec, c.Floor = gradientPalette(cover)
	}
	return c
}

// RowColor returns the final canvas color for one row.
//
// Every row uses the exact same rule, so the canvas is one continuous gradient
// with no visible bands behind cards or lists.
func (c Canvas) RowColor(t float64) string {
	raw := c.rawRowColor(t)
	if c.Mode == "dark" {
		return raw
	}
	return ensureContrast(raw, canvasLuminance)
}

// Paint applies the canvas background to a single rendered terminal row.
func (c Canvas) Paint(line string, t float64) string {
	return ApplyRowBackground(line, c.RowColor(t))
}

// PaintLines applies the canvas to a full screen buffer.
func (c Canvas) PaintLines(lines []string) []string {
	if c.Mode == "dark" {
		for i := range lines {
			lines[i] = ApplyRowBackground(lines[i], c.Floor)
		}
		return lines
	}
	for i := range lines {
		lines[i] = c.Paint(lines[i], normalizedRow(i, len(lines)))
	}
	return lines
}

// rawRowColor is the unconstrained animated canvas color at normalized row t.
func (c Canvas) rawRowColor(t float64) string {
	t = math.Max(0, math.Min(1, t))
	switch {
	case c.Mode == "dark":
		return c.Floor
	case c.flow:
		phase := 2 * math.Pi * float64(c.Frame) / flowCycleFrames
		breath := 0.5 + 0.5*math.Sin(phase)
		flowTop := LerpHex(c.Top, c.Sec, breath)
		wave := 0.10 * math.Sin(2*math.Pi*(float64(c.Frame)/flowCycleFrames-t*0.8))
		factor := math.Min(1.0, math.Max(0.0, t*1.05+wave))
		return LerpHex(flowTop, c.Floor, factor)
	default:
		// Vertical gradient that spans the whole screen instead of collapsing into
		// the floor on the upper third.
		return LerpHex(c.Top, c.Floor, math.Min(1.0, t*1.05))
	}
}

// flowPalette derives the animated flow colors from the album art only.
//
// The previous contrast patch blended 75-85% toward the theme background, which
// erased the artwork hue and made the "dynamic" background look static. Blending
// is now bounded (<= 42%) so saturated motion survives the contrast ceiling.
func flowPalette(cover *CoverData) (top, sec, floor string) {
	floor = flowFloor
	top, sec = "#1b1b3a", "#2a1040"
	if cover == nil {
		return
	}
	dominant := vividColor(cover.DominantHex)
	secondary := vividColor(cover.SecondaryHex)
	if dominant == "" {
		dominant = secondary
		secondary = ""
	}
	switch {
	case dominant != "" && secondary != "" && secondary != dominant:
		top = LerpHex(dominant, floor, 0.30)
		sec = LerpHex(secondary, floor, 0.42)
	case dominant != "":
		top = LerpHex(dominant, floor, 0.30)
		sec = LerpHex(top, floor, 0.35)
	}
	return
}

// gradientPalette is the calm, non-animated variant used by "default"/"gradient".
func gradientPalette(cover *CoverData) (top, sec, floor string) {
	floor = flowFloor
	sec = floor
	top = LerpHex("#27272a", floor, 0.50)
	if cover != nil {
		if dominant := vividColor(cover.DominantHex); dominant != "" {
			top = LerpHex(dominant, floor, 0.45)
		}
	}
	return
}

// vividColor returns a color only when it is saturated enough to keep the canvas
// colorful, so monochrome artwork falls back to the built-in palette.
func vividColor(hexColor string) string {
	if hexColor == "" || saturationOf(hexColor) < minPaletteSaturation {
		return ""
	}
	return hexColor
}

// normalizedRow maps a row index onto the [0,1] canvas axis.
func normalizedRow(i, total int) float64 {
	if total <= 1 {
		return 0
	}
	return float64(i) / float64(total-1)
}

// ensureContrast darkens a color (preserving its hue) until its luminance fits
// inside the ceiling. This is the mathematical guarantee that keeps UI text
// readable no matter how vivid the animated canvas is.
func ensureContrast(hexColor string, ceiling float64) string {
	r, g, b, ok := parseHexRGB(hexColor)
	if !ok {
		return hexColor
	}
	if luminanceRGB(r, g, b) <= ceiling {
		return hexColor
	}
	// Luminance decreases monotonically as the RGB scale shrinks, so a bisection
	// finds the largest scale that still satisfies the ceiling.
	lo, hi := 0.0, 1.0
	for i := 0; i < 10; i++ {
		mid := (lo + hi) / 2
		if luminanceScaled(r, g, b, mid) <= ceiling {
			lo = mid
		} else {
			hi = mid
		}
	}
	return scaleHexRGB(r, g, b, lo)
}

// contrastRatio returns the WCAG contrast ratio between two hex colors.
func contrastRatio(a, b string) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance returns the WCAG relative luminance of a hex color.
func relativeLuminance(hexColor string) float64 {
	r, g, b, ok := parseHexRGB(hexColor)
	if !ok {
		return 0
	}
	return luminanceRGB(r, g, b)
}

func luminanceRGB(r, g, b uint8) float64 {
	return 0.2126*linearChannel(r) + 0.7152*linearChannel(g) + 0.0722*linearChannel(b)
}

func linearChannel(v uint8) float64 {
	c := float64(v) / 255.0
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func luminanceScaled(r, g, b uint8, factor float64) float64 {
	f := math.Max(0, math.Min(1, factor))
	return luminanceRGB(uint8(float64(r)*f), uint8(float64(g)*f), uint8(float64(b)*f))
}

func scaleHexRGB(r, g, b uint8, factor float64) string {
	f := math.Max(0, math.Min(1, factor))
	return fmt.Sprintf("#%02x%02x%02x", uint8(float64(r)*f), uint8(float64(g)*f), uint8(float64(b)*f))
}

// colorDistance returns the summed absolute RGB difference between two hex
// colors, used to assert that the canvas actually moves between frames.
func colorDistance(a, b string) int {
	r1, g1, b1, ok1 := parseHexRGB(a)
	r2, g2, b2, ok2 := parseHexRGB(b)
	if !ok1 || !ok2 {
		return 0
	}
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return abs(int(r1)-int(r2)) + abs(int(g1)-int(g2)) + abs(int(b1)-int(b2))
}

// saturationOf returns max-min of the RGB channels, a cheap vividness proxy.
func saturationOf(hexColor string) int {
	r, g, b, ok := parseHexRGB(hexColor)
	if !ok {
		return 0
	}
	hi := max(r, g, b)
	lo := min(r, g, b)
	return int(hi) - int(lo)
}
