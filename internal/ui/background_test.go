package ui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/theme"
)

var rowBackgroundPattern = regexp.MustCompile(`48;2;(\d+);(\d+);(\d+)`)

// rowBackgrounds extracts the first background color of every rendered row.
func rowBackgrounds(content string) []string {
	rows := strings.Split(content, "\n")
	out := make([]string, len(rows))
	for i, row := range rows {
		match := rowBackgroundPattern.FindStringSubmatch(row)
		if match == nil {
			continue
		}
		r, _ := strconv.Atoi(match[1])
		g, _ := strconv.Atoi(match[2])
		b, _ := strconv.Atoi(match[3])
		out[i] = fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	return out
}

// canvasView builds a realistic main view so the canvas wiring can be asserted.
func canvasView(t *testing.T, th theme.Theme, mode string) Model {
	t.Helper()
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	m.theme = th
	m.bgMode = mode
	m.coverData = &CoverData{DominantHex: "#1ED760", SecondaryHex: "#38BDF8"}
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 14000,
		Item: &spotify.Track{
			URI:        "spotify:track:x",
			Name:       "In Love",
			DurationMS: 232000,
			Artists:    []spotify.Artist{{Name: "Aaron May"}},
			Album:      spotify.Album{Name: "CHASE", ReleaseDate: "2019-05-01"},
		},
		Device: &spotify.Device{ID: "phone", Name: "iPhone"},
	}
	next, _ := m.Update(playlistsMsg{
		page: spotify.PlaylistPage{Items: []spotify.Playlist{{ID: "one", Name: "Canciones que te gustan"}}},
		seq:  m.playlistSeq,
	})
	return next.(Model)
}

func TestMainViewKeepsFlowPreferenceStaticWithThemeIsolation(t *testing.T) {
	dark := canvasView(t, theme.ThemeSpotifyDark, "flow")
	nord := canvasView(t, theme.ThemeNord, "flow")
	otherFrame := canvasView(t, theme.ThemeSpotifyDark, "flow")
	otherFrame.zenFrame = 37

	darkRows := rowBackgrounds(dark.View().Content)
	nordRows := rowBackgrounds(nord.View().Content)
	otherFrameRows := rowBackgrounds(otherFrame.View().Content)
	if len(darkRows) != len(nordRows) {
		t.Fatalf("view row count changed with the theme: %d != %d", len(darkRows), len(nordRows))
	}
	for i := range darkRows {
		if darkRows[i] != nordRows[i] {
			t.Fatalf("normal background row %d depends on the theme: %s != %s", i, darkRows[i], nordRows[i])
		}
		if darkRows[i] != otherFrameRows[i] {
			t.Fatalf("normal background row %d animated with Zen's clock: %s != %s", i, darkRows[i], otherFrameRows[i])
		}
	}
}

func TestMainViewHasNoLuminanceBands(t *testing.T) {
	// Both a dim cover (the very case that shipped the dark band) and a very bright
	// one, where the contrast ceiling clips hardest.
	covers := map[string]*CoverData{
		"dark":   {DominantHex: "#2a1a5e", SecondaryHex: "#3b2a7a"},
		"bright": {DominantHex: "#FFD700", SecondaryHex: "#00FF88"},
	}
	for name, cover := range covers {
		m := canvasView(t, theme.ThemeSpotifyDark, "flow")
		m.coverData = cover
		rows := rowBackgrounds(m.View().Content)

		if len(rows) < 5 {
			t.Fatalf("%s: expected a full screen of rows, got %d", name, len(rows))
		}
		for i := 1; i < len(rows); i++ {
			if rows[i] == "" || rows[i-1] == "" {
				continue
			}
			// The canvas must be one continuous gradient: no row may jump in
			// luminance, which is what produced the dark band behind cards and lists.
			if delta := math.Abs(relativeLuminance(rows[i]) - relativeLuminance(rows[i-1])); delta > 0.02 {
				t.Fatalf("%s: luminance band between rows %d and %d: %.4f -> %.4f", name, i-1, i, relativeLuminance(rows[i-1]), relativeLuminance(rows[i]))
			}
		}
	}
}

func TestCanvasIgnoresRowContent(t *testing.T) {
	// Rows with and without text share the exact same color at a given position.
	lines := []string{"", ApplyRowBackground("In Love", "#000000"), "", ApplyRowBackground("Aaron May", "#000000"), ""}
	c := NewCanvas("flow", &CoverData{DominantHex: "#1ED760", SecondaryHex: "#38BDF8"}, "#0c0d0e", "#FFFFFF", "#CBD5E1", 4)
	painted := c.PaintLines(append([]string(nil), lines...))

	for i := range painted {
		if want := c.RowColor(normalizedRow(i, len(painted))); !strings.Contains(painted[i], backgroundSeq(want)) {
			t.Fatalf("row %d did not use the shared canvas color %s", i, want)
		}
	}
}

func backgroundSeq(hexColor string) string {
	r, g, b, _ := parseHexRGB(hexColor)
	return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
}

func TestMainViewDarkModeUsesOnlyTheThemeBackground(t *testing.T) {
	for _, th := range theme.All() {
		m := canvasView(t, th, "dark")
		for i, bg := range rowBackgrounds(m.View().Content) {
			if bg != th.BgBase {
				t.Fatalf("%s dark mode row %d used %s instead of %s", th.ID, i, bg, th.BgBase)
			}
		}
	}
}

func TestZenViewUsesTheReactiveCanvas(t *testing.T) {
	m := canvasView(t, theme.ThemeSpotifyDark, "flow")
	m.width, m.height = 100, 30
	m.state.Item.Album.Images = nil
	m.setView(viewCoverArt)

	rows := rowBackgrounds(m.View().Content)
	unique := make(map[string]bool)
	maxL, minL := 0.0, 1.0
	for _, bg := range rows {
		if bg == "" {
			continue
		}
		unique[bg] = true
		l := relativeLuminance(bg)
		maxL = math.Max(maxL, l)
		minL = math.Min(minL, l)
	}
	if len(unique) < 5 {
		t.Fatalf("zen canvas is flat: only %d distinct row colors", len(unique))
	}
	if maxL-minL < 0.03 {
		t.Fatalf("zen canvas has no depth: %.3f..%.3f", minL, maxL)
	}
}

func TestCanvasFlowIgnoresThemeBackground(t *testing.T) {
	cover := &CoverData{DominantHex: "#FF007F", SecondaryHex: "#00F0FF"}
	dark := theme.ThemeSpotifyDark
	cyber := theme.ThemeCyberpunk

	flowA := NewCanvas("flow", cover, dark.BgBase, dark.Text, dark.Muted, 12)
	flowB := NewCanvas("flow", cover, cyber.BgBase, cyber.Text, cyber.Muted, 12)

	for i := 0; i <= 20; i++ {
		tv := float64(i) / 20
		if got, want := flowA.RowColor(tv), flowB.RowColor(tv); got != want {
			t.Fatalf("flow canvas must not depend on the theme (t=%.2f): %s != %s", tv, got, want)
		}
	}
}

func TestCanvasDarkModeUsesThemeBackground(t *testing.T) {
	cover := &CoverData{DominantHex: "#FF007F", SecondaryHex: "#00F0FF"}
	for _, th := range theme.All() {
		c := NewCanvas("dark", cover, th.BgBase, th.Text, th.Muted, 5)
		for i := 0; i <= 10; i++ {
			tv := float64(i) / 10
			if got := c.RowColor(tv); got != th.BgBase {
				t.Fatalf("dark mode must paint the theme background for %s, got %s", th.ID, got)
			}
		}
	}
}

func TestCanvasGuaranteesTextContrastInEveryTheme(t *testing.T) {
	// Bright, saturated artwork is the worst case for a light-on-dark terminal.
	cover := &CoverData{DominantHex: "#FFD700", SecondaryHex: "#00FF88"}

	for _, th := range theme.All() {
		for _, mode := range []string{"flow", "default", "gradient", "dark"} {
			c := NewCanvas(mode, cover, th.BgBase, th.Text, th.Muted, 9)
			for i := 0; i <= 40; i++ {
				bg := c.RowColor(float64(i) / 40)

				if ratio := contrastRatio(th.Text, bg); ratio < MinTextContrast-0.01 {
					t.Fatalf("%s/%s: text contrast %.2f at t=%.2f (bg %s)", th.ID, mode, ratio, float64(i)/40, bg)
				}
				if ratio := contrastRatio(th.Muted, bg); ratio < MinMutedContrast-0.01 {
					t.Fatalf("%s/%s: muted contrast %.2f at t=%.2f (bg %s)", th.ID, mode, ratio, float64(i)/40, bg)
				}
			}
		}
	}
}

func TestFlowCanvasStaysVividAndAnimated(t *testing.T) {
	cover := &CoverData{DominantHex: "#1ED760", SecondaryHex: "#38BDF8"}
	th := theme.ThemeSpotifyDark
	build := func(frame int) Canvas {
		return NewCanvas("flow", cover, th.BgBase, th.Text, th.Muted, frame)
	}

	// Vivid: the palette kept its saturation instead of collapsing into the floor.
	// The previous contrast patch blended 75% toward the theme background, which
	// left the animated backdrop effectively grey.
	first := build(0)
	if sat := saturationOf(first.RowColor(0)); sat < 40 {
		t.Fatalf("flow top row lost its color: saturation %d (%s)", sat, first.RowColor(0))
	}

	// Animated: the ambient breathing has to move the color noticeably.
	movedEarly, movedLate := false, false
	base := first.RowColor(0)
	for frame := 1; frame <= 60; frame++ {
		moved := colorDistance(base, build(frame).RowColor(0)) >= 12
		if frame <= 15 && moved {
			movedEarly = true
		}
		if frame >= 45 && moved {
			movedLate = true
		}
	}
	if !movedEarly || !movedLate {
		t.Fatalf("flow canvas looks static across frames (early=%v late=%v)", movedEarly, movedLate)
	}

	// The gradient must span the whole screen instead of dying out on the top third.
	last := build(0).RowColor(1)
	if colorDistance(base, last) < 12 {
		t.Fatalf("flow gradient is flat from top to bottom: %s vs %s", base, last)
	}

	// Mid-screen motion must be visible: that region sits behind the playlist rows.
	best := 0
	for _, tv := range []float64{0.2, 0.4, 0.6, 0.8} {
		if d := colorDistance(build(0).RowColor(tv), build(30).RowColor(tv)); d > best {
			best = d
		}
	}
	if best < 15 {
		t.Fatalf("flow animation is imperceptible mid-screen (best delta %d)", best)
	}
}

func TestFlowCanvasRejectsMonochromeArtwork(t *testing.T) {
	th := theme.ThemeSpotifyDark
	white := &CoverData{DominantHex: "#FFFFFF", SecondaryHex: "#F2F2F2"}
	c := NewCanvas("flow", white, th.BgBase, th.Text, th.Muted, 0)

	// A white cover must not turn the animated backdrop into a flat grey surface.
	if sat := saturationOf(c.RowColor(0)); sat < 25 {
		t.Fatalf("monochrome artwork washed the canvas out: saturation %d (%s)", sat, c.RowColor(0))
	}
	if l := relativeLuminance(c.RowColor(0)); l > canvasLuminance+0.001 {
		t.Fatalf("canvas exceeded the luminance ceiling: %.3f", l)
	}
}

func TestEnsureContrastPreservesHue(t *testing.T) {
	darkened := ensureContrast("#FFD700", canvasLuminance)
	if relativeLuminance(darkened) > canvasLuminance {
		t.Fatalf("ensureContrast left the color too bright: %s", darkened)
	}
	if saturationOf(darkened) < 40 {
		t.Fatalf("ensureContrast bleached the hue away: %s", darkened)
	}
	// Already-dark colors must pass through untouched.
	if got := ensureContrast("#0a0a12", canvasLuminance); got != "#0a0a12" {
		t.Fatalf("dark colors must be preserved, got %s", got)
	}
}

func TestCanvasPaintLinesKeepsEveryRow(t *testing.T) {
	c := NewCanvas("flow", nil, "#0c0d0e", "#FFFFFF", "#CBD5E1", 0)
	lines := []string{"hello", "", "world"}
	painted := c.PaintLines(lines)
	if len(painted) != len(lines) {
		t.Fatalf("expected %d painted rows, got %d", len(lines), len(painted))
	}
	for i, line := range painted {
		if !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("row %d was not background-painted", i)
		}
	}
}
