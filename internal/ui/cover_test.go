package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestScaleImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill top half red, bottom half blue
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	for y := 50; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}

	scaled := scaleImage(src, 10, 10)
	if scaled.Bounds().Dx() != 10 || scaled.Bounds().Dy() != 10 {
		t.Fatalf("expected 10x10 bounds, got %dx%d", scaled.Bounds().Dx(), scaled.Bounds().Dy())
	}
}

func TestProcessCoverImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			src.Set(x, y, color.RGBA{R: 120, G: 60, B: 200, A: 255})
		}
	}

	cover := processCoverImage(src, 10, 5)
	if len(cover.Lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(cover.Lines))
	}
	if len(cover.BigLines) != BigCoverHeightChars {
		t.Fatalf("expected %d big lines, got %d", BigCoverHeightChars, len(cover.BigLines))
	}
	for i, l := range cover.Lines {
		if !strings.Contains(l, "▀") {
			t.Errorf("line %d does not contain half-block character: %q", i, l)
		}
		if !strings.Contains(l, "\x1b[38;2;") || !strings.Contains(l, "\x1b[48;2;") {
			t.Errorf("line %d missing ANSI TrueColor escapes: %q", i, l)
		}
	}
	for i, l := range cover.BigLines {
		if !strings.Contains(l, "▀") {
			t.Errorf("big line %d does not contain half-block character: %q", i, l)
		}
	}

	if cover.DominantHex == "" || !strings.HasPrefix(cover.DominantHex, "#") {
		t.Errorf("expected dominant hex starting with #, got %q", cover.DominantHex)
	}
}

func TestLerpHex(t *testing.T) {
	c1 := "#000000"
	c2 := "#ffffff"

	mid := LerpHex(c1, c2, 0.5)
	if mid != "#7f7f7f" {
		t.Errorf("expected #7f7f7f at 0.5, got %q", mid)
	}

	start := LerpHex(c1, c2, 0.0)
	if start != c1 {
		t.Errorf("expected %s at 0.0, got %s", c1, start)
	}

	end := LerpHex(c1, c2, 1.0)
	if end != c2 {
		t.Errorf("expected %s at 1.0, got %s", c2, end)
	}
}

func TestApplyRowBackground(t *testing.T) {
	rawLine := "Hello \x1b[31mRed\x1b[0m World"
	styled := ApplyRowBackground(rawLine, "#123456")

	if !strings.HasPrefix(styled, "\x1b[48;2;18;52;86m") {
		t.Errorf("expected prefix with background escape, got %q", styled)
	}
	if !strings.Contains(styled, "\x1b[0m\x1b[48;2;18;52;86m") {
		t.Errorf("expected reset to re-assert background escape, got %q", styled)
	}
	if !strings.HasSuffix(styled, "\x1b[0m") {
		t.Errorf("expected suffix reset escape, got %q", styled)
	}
}

func TestExtractCoverPalette_FaithfulColors(t *testing.T) {
	// Malibu Nights simulation: Top half magenta/pink (#e91e63), bottom half red (#d32f2f)
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			src.Set(x, y, color.RGBA{R: 233, G: 30, B: 99, A: 255}) // Magenta/Pink
		}
	}
	for y := 20; y < 40; y++ {
		for x := 0; x < 40; x++ {
			src.Set(x, y, color.RGBA{R: 211, G: 47, B: 47, A: 255}) // Crimson Red
		}
	}

	domColor, domHex, secHex := extractCoverPalette(src)
	if domHex == "" || secHex == "" {
		t.Fatalf("expected non-empty dominant and secondary hex, got %q and %q", domHex, secHex)
	}

	// Dominant color MUST be magenta/reddish (Red channel should dominate, Green channel MUST be low)
	if domColor.G > domColor.R || domColor.G > 80 {
		t.Errorf("expected low green channel for magenta/red cover, got R=%d G=%d B=%d (hex: %s)", domColor.R, domColor.G, domColor.B, domHex)
	}

	// Secondary hex must also not be green
	var r2, g2, b2 uint8
	fmt.Sscanf(stringsTrimHex(secHex), "%02x%02x%02x", &r2, &g2, &b2)
	if g2 > r2 {
		t.Errorf("secondary color should be red/pink, but green dominates: R=%d G=%d B=%d (hex: %s)", r2, g2, b2, secHex)
	}
}

func TestExtractCoverPalette_MonochromeNoGreen(t *testing.T) {
	// Pure black and white album cover (e.g. grayscale)
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			val := uint8((x + y) * 2)
			src.Set(x, y, color.RGBA{R: val, G: val, B: val, A: 255})
		}
	}

	domColor, domHex, secHex := extractCoverPalette(src)
	if domHex == "" || secHex == "" {
		t.Fatalf("expected non-empty hex, got %q, %q", domHex, secHex)
	}

	// Green channel must NOT dominate (should be subtle dark neutral slate, definitely not Spotify green #1db954)
	if domColor.G > 50 {
		t.Errorf("monochrome album should have dark neutral slate, got R=%d G=%d B=%d (hex: %s)", domColor.R, domColor.G, domColor.B, domHex)
	}
}

func TestZenStarrySkyTwinkle(t *testing.T) {
	// Verify that the starry sky seeds are well-formed and distributed within [0, 1]
	if len(zenStars) == 0 {
		t.Fatalf("expected non-empty zenStars array")
	}

	for i, s := range zenStars {
		if s.xRatio < 0.0 || s.xRatio > 1.0 {
			t.Errorf("star %d xRatio out of bounds: %f", i, s.xRatio)
		}
		if s.yRatio < 0.0 || s.yRatio > 1.0 {
			t.Errorf("star %d yRatio out of bounds: %f", i, s.yRatio)
		}
		if s.char == 0 {
			t.Errorf("star %d missing rune character", i)
		}
		if s.color == "" {
			t.Errorf("star %d missing color", i)
		}
	}
}

func TestRenderBlurredCoverLines(t *testing.T) {
	// Create high-contrast test image (white cross on black background)
	src := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if x == 10 || y == 10 {
				src.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				src.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
			}
		}
	}

	sharpLines := RenderCoverLines(src, 10, 5)
	if len(sharpLines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(sharpLines))
	}

	// At progress 1.0, blurred lines MUST match sharp lines identically
	fullProgressLines := RenderBlurredCoverLines(src, nil, 10, 5, 1.0)
	for i := range sharpLines {
		if fullProgressLines[i] != sharpLines[i] {
			t.Errorf("line %d at progress 1.0 does not match sharp rendering", i)
		}
	}

	// At progress 0.0, blurred lines MUST be diffuse (not identical to sharp)
	diffuseLines := RenderBlurredCoverLines(src, nil, 10, 5, 0.0)
	if len(diffuseLines) != 5 {
		t.Fatalf("expected 5 diffuse lines, got %d", len(diffuseLines))
	}
	// Verify that diffuse lines are valid ANSI strings
	for i, l := range diffuseLines {
		if !strings.Contains(l, "▀") {
			t.Errorf("diffuse line %d missing half-block characters", i)
		}
	}
}

func TestComputeRhythmBorderColor(t *testing.T) {
	base := "#52525b"

	// When paused, border should be resting in calm dimmed state
	pausedColor := computeRhythmBorderColor(base, false, 0, 0)
	if pausedColor == "" {
		t.Fatal("expected non-empty paused color")
	}

	// When playing at beat peak (position 0ms of 520ms beat cycle)
	beatPeakColor := computeRhythmBorderColor(base, true, 0, 0)

	// When playing at beat trough (position 400ms of 520ms beat cycle)
	beatTroughColor := computeRhythmBorderColor(base, true, 400, 0)

	if beatPeakColor == beatTroughColor {
		t.Errorf("expected rhythmic illumination change between beat peak and trough, got %s for both", beatPeakColor)
	}
}

func TestRenderSweptText(t *testing.T) {
	text := "Pvta Luna"
	width := 40
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true)

	// Progress 1.0: Full text must be present
	completedLine, start1, end1 := renderSweptText(text, width, 1.0, style, true)
	if !strings.Contains(completedLine, "Pvta Luna") {
		t.Errorf("expected full text at progress 1.0, got: %s", completedLine)
	}
	if start1 >= end1 {
		t.Errorf("invalid text bounds: start=%d, end=%d", start1, end1)
	}

	// Progress 0.0: Text is not yet revealed (spaces ahead of beam)
	startLine, _, _ := renderSweptText(text, width, 0.0, style, true)
	if strings.Contains(startLine, "Pvta Luna") {
		t.Errorf("expected text to not be fully revealed at start, got: %s", startLine)
	}

	// Progress 0.5: Middle of sweep contains beam head glow (\x1b[38;2;255;255;255m)
	midLine, _, _ := renderSweptText(text, width, 0.5, style, true)
	if !strings.Contains(midLine, "\x1b[") {
		t.Errorf("expected ANSI formatting in swept line, got: %s", midLine)
	}
}

func TestParseHexRGB(t *testing.T) {
	tests := []struct {
		input string
		wantR uint8
		wantG uint8
		wantB uint8
		ok    bool
	}{
		{"#1db954", 0x1d, 0xb9, 0x54, true},
		{"1db954", 0x1d, 0xb9, 0x54, true},
		{"#ffffff", 255, 255, 255, true},
		{"#000000", 0, 0, 0, true},
		{"#ABCDEF", 0xab, 0xcd, 0xef, true},
		{"invalid", 0, 0, 0, false},
		{"#123", 0, 0, 0, false},
		{"", 0, 0, 0, false},
	}

	for _, tc := range tests {
		r, g, b, ok := parseHexRGB(tc.input)
		if ok != tc.ok {
			t.Errorf("parseHexRGB(%q) ok = %v, want %v", tc.input, ok, tc.ok)
		}
		if ok && (r != tc.wantR || g != tc.wantG || b != tc.wantB) {
			t.Errorf("parseHexRGB(%q) = (%d,%d,%d), want (%d,%d,%d)", tc.input, r, g, b, tc.wantR, tc.wantG, tc.wantB)
		}
	}
}

func TestCoverManagerLRUEviction(t *testing.T) {
	cm := &CoverManager{
		memory: make(map[string]CoverData),
	}
	// Fill cache with MaxMemoryCovers + 5 items
	for i := 0; i < MaxMemoryCovers+5; i++ {
		key := fmt.Sprintf("url_%d", i)
		cm.putMemory(key, CoverData{DominantHex: "#ffffff"})
	}

	cm.mu.RLock()
	count := len(cm.memory)
	orderCount := len(cm.order)
	cm.mu.RUnlock()

	if count > MaxMemoryCovers {
		t.Errorf("expected memory cache size <= %d, got %d", MaxMemoryCovers, count)
	}
	if orderCount > MaxMemoryCovers {
		t.Errorf("expected order slice size <= %d, got %d", MaxMemoryCovers, orderCount)
	}

	// Verify the oldest entry was evicted
	if _, ok := cm.getMemory("url_0"); ok {
		t.Errorf("expected url_0 to be evicted from memory cache")
	}
	// Verify the newest entry is in cache
	if _, ok := cm.getMemory(fmt.Sprintf("url_%d", MaxMemoryCovers+4)); !ok {
		t.Errorf("expected newest entry to be in memory cache")
	}
}
