package ui

import (
	"math"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type starSpec struct {
	xRatio float64 // 0.0 to 1.0 across terminal width
	yRatio float64 // 0.0 to 1.0 across terminal height
	char   rune    // '·', '✦', '✧', '˙', '°', '*'
	phase  float64
	speed  float64
	color  string
}

// 32 deterministic star seeds distributed in the cosmic margins
var zenStars = []starSpec{
	{0.05, 0.08, '·', 0.0, 0.07, "#ffffff"},
	{0.18, 0.05, '✦', 1.8, 0.05, "#93c5fd"},
	{0.84, 0.07, '·', 2.5, 0.08, "#ffffff"},
	{0.94, 0.12, '✧', 3.9, 0.04, "#fef08a"},
	{0.04, 0.22, '˙', 0.9, 0.06, "#ffffff"},
	{0.12, 0.36, '·', 4.3, 0.09, "#a7f3d0"},
	{0.88, 0.32, '·', 1.2, 0.07, "#ffffff"},
	{0.96, 0.42, '✦', 5.1, 0.05, "#e2e8f0"},
	{0.06, 0.55, '✧', 2.8, 0.06, "#fef08a"},
	{0.15, 0.68, '·', 3.5, 0.08, "#ffffff"},
	{0.85, 0.62, '˙', 0.6, 0.05, "#ffffff"},
	{0.95, 0.74, '·', 5.0, 0.07, "#bae6fd"},
	{0.22, 0.12, '°', 2.0, 0.05, "#ffffff"},
	{0.78, 0.14, '·', 3.2, 0.07, "#ffffff"},
	{0.02, 0.18, '·', 0.4, 0.08, "#ffffff"},
	{0.98, 0.20, '·', 2.1, 0.06, "#ffffff"},
	{0.10, 0.48, '✦', 4.6, 0.05, "#fef08a"},
	{0.89, 0.52, '✧', 1.8, 0.07, "#a7f3d0"},
	{0.03, 0.65, '·', 5.6, 0.06, "#ffffff"},
	{0.97, 0.64, '˙', 4.0, 0.09, "#ffffff"},
	{0.20, 0.78, '·', 0.8, 0.07, "#ffffff"},
	{0.80, 0.80, '✦', 2.3, 0.05, "#93c5fd"},
	{0.11, 0.16, '·', 3.8, 0.07, "#ffffff"},
	{0.86, 0.24, '·', 1.4, 0.05, "#ffffff"},
	{0.05, 0.34, '✧', 3.0, 0.06, "#fef08a"},
	{0.93, 0.36, '·', 4.2, 0.08, "#ffffff"},
	{0.17, 0.60, '˙', 0.5, 0.05, "#ffffff"},
	{0.82, 0.48, '·', 3.4, 0.07, "#ffffff"},
	{0.07, 0.82, '·', 1.9, 0.06, "#ffffff"},
	{0.91, 0.85, '·', 5.3, 0.05, "#ffffff"},
	{0.48, 0.04, '·', 0.7, 0.06, "#ffffff"},
	{0.52, 0.94, '✦', 3.1, 0.05, "#fef08a"},
}

// renderBurningFuse builds a minimalist burning fuse progress bar:
// solid and firm at the tail, warming up to a glowing ember at the playback head,
// with an incandescent flickering spark when playing, and subtle unburnt cord ahead.
// Free of any timestamps, text, or emojis.
func renderBurningFuse(fraction float64, barW int, isPlaying bool, frame int, accentHex string) string {
	if barW <= 0 {
		return ""
	}
	fraction = max(0.0, min(1.0, fraction))
	headX := int(fraction * float64(barW))
	if headX >= barW {
		headX = barW - 1
	}

	// Spark color and character at the active burning tip
	var sparkColor, sparkChar string
	if isPlaying {
		sparkSine := 0.5 + 0.5*math.Sin(float64(frame)*0.45)
		sparkColor = LerpHex("#ffffff", "#fef08a", sparkSine)
		sparkChar = "✦"
	} else {
		sparkColor = "#71717a"
		sparkChar = "·"
	}

	var sb strings.Builder
	for x := 0; x < barW; x++ {
		if x < headX {
			// Solid burned fuse: gradient from solid base to glowing ember
			t := float64(x) / float64(max(1, headX))
			colorHex := LerpHex(accentHex, "#86efac", t*0.85)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex))
			sb.WriteString(style.Render("━"))
		} else if x == headX {
			// Incandescent spark / ember tip
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(sparkColor)).Bold(isPlaying)
			sb.WriteString(style.Render(sparkChar))
		} else {
			// Unburnt cord ahead: subtle dim line fading into darkness
			t := float64(x-headX) / float64(max(1, barW-headX))
			colorHex := LerpHex("#27272a", "#0f0f11", t*0.80)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex))
			sb.WriteString(style.Render("─"))
		}
	}
	return sb.String()
}

// renderCoverArtZenView renders the clean, full-screen Zen Sanctuary:
// Giant framed album art + track & artist + burning fuse progress bar + cosmic starry background.
// Completely free of player card, volume, devices, widgets, timestamps, or emojis.
func (m Model) renderCoverArtZenView(width, height int, fit func(string) string) tea.View {
	targetHeight := max(1, height-1)
	lines := make([]string, targetHeight)

	// 1. Reactive background canvas (only "dark" mode uses the theme background)
	canvas := NewCanvas(m.bgMode, m.coverData, m.theme.BgBase, m.theme.Text, m.theme.Muted, m.zenFrame)
	topColor := canvas.Top

	// 2. Responsive square cover dimensions
	// Reserve 6 rows for: 2 borders, 1 blank, 1 track, 1 artist, 1 fuse bar
	maxCoverH := targetHeight - 8
	coverH := min(16, max(8, maxCoverH))
	coverW := coverH * 2
	if coverW > width-8 {
		coverW = max(16, ((width-8)/2)*2)
		coverH = coverW / 2
	}

	// 3. Track, Artist, and Playback Position
	trackName := "SpotifyGo"
	artistName := "Zen Mode"
	if m.state.Item != nil {
		trackName = m.state.Item.Name
		var names []string
		for _, a := range m.state.Item.Artists {
			names = append(names, a.Name)
		}
		if len(names) > 0 {
			artistName = strings.Join(names, ", ")
		}
		if m.state.Item.Album.Name != "" {
			artistName += " — " + m.state.Item.Album.Name
			if yr := m.state.Item.ReleaseYear(); yr != "" {
				artistName += " (" + yr + ")"
			}
		}
	}

	position := m.currentPositionMS()
	fraction := 0.0
	if m.state.Item != nil && m.state.Item.DurationMS > 0 {
		position = min(position, m.state.Item.DurationMS)
		fraction = float64(position) / float64(m.state.Item.DurationMS)
	}

	padTop := max(1, (targetHeight-(coverH+7))/2)
	padLeftCoverW := max(0, (width-coverW-2)/2)
	padLeftCover := strings.Repeat(" ", padLeftCoverW)

	// Audio-reactive rhythm border: subtle breathing with music pulse (~115 BPM)
	borderBaseTone := "#52525b"
	borderColor := computeRhythmBorderColor(LerpHex(topColor, borderBaseTone, 0.35), m.state.IsPlaying, position, m.zenFrame)
	boxBorder := lipgloss.NewStyle().Foreground(lipgloss.Color(borderColor))

	coverLines := m.getBigCoverLines(coverW, coverH)

	// 4. Burning Fuse Progress Bar
	barW := min(42, max(24, coverW))
	fuseBar := renderBurningFuse(fraction, barW, m.state.IsPlaying, m.zenFrame, m.theme.Accent)
	padLeftBarW := max(0, (width-barW)/2)
	padLeftBar := strings.Repeat(" ", padLeftBarW)

	// 5. Star coordinates lookup mapped to screen grid
	type starPoint struct {
		x     int
		char  rune
		color string
	}
	starsAtRow := make(map[int][]starPoint)
	for _, s := range zenStars {
		sy := int(s.yRatio * float64(targetHeight))
		sx := int(s.xRatio * float64(width))
		twinkle := math.Sin(float64(m.zenFrame)*s.speed + s.phase)
		if twinkle > 0.15 {
			alpha := (twinkle - 0.15) / 0.85
			starColor := s.color
			starBase := "#ffffff"
			starsAtRow[sy] = append(starsAtRow[sy], starPoint{
				x:     sx,
				char:  s.char,
				color: LerpHex(starBase, starColor, alpha),
			})
		}
	}

	// Sort each row's stars by X to guarantee left-to-right rendering
	for r := range starsAtRow {
		sort.Slice(starsAtRow[r], func(i, j int) bool {
			return starsAtRow[r][i].x < starsAtRow[r][j].x
		})
	}

	// 6. Build lines
	for y := 0; y < targetHeight; y++ {
		contentRow := y - padTop
		var lineContent string
		contentStart, contentEnd := -1, -1

		switch {
		case contentRow == 0:
			// Box top border
			contentStart = padLeftCoverW
			contentEnd = padLeftCoverW + coverW + 2
			lineContent = padLeftCover + boxBorder.Render("╭"+strings.Repeat("─", coverW)+"╮")
		case contentRow >= 1 && contentRow <= coverH:
			// Cover line
			contentStart = padLeftCoverW
			contentEnd = padLeftCoverW + coverW + 2
			idx := contentRow - 1
			if len(coverLines) > idx {
				lineContent = padLeftCover + boxBorder.Render("│") + coverLines[idx] + boxBorder.Render("│")
			} else {
				lineContent = padLeftCover + boxBorder.Render("│") + strings.Repeat(" ", coverW) + boxBorder.Render("│")
			}
		case contentRow == coverH+1:
			// Box bottom border
			contentStart = padLeftCoverW
			contentEnd = padLeftCoverW + coverW + 2
			lineContent = padLeftCover + boxBorder.Render("╰"+strings.Repeat("─", coverW)+"╯")
		case contentRow == coverH+3:
			// Track Name
			baseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Text)).Bold(true)
			if m.zenTransActive && m.zenTransFrame < ZenTransitionMaxFrames {
				progress := float64(m.zenTransFrame) / float64(ZenTransitionMaxFrames)
				lineContent, contentStart, contentEnd = renderSweptText(trackName, width, progress, baseStyle, true)
			} else {
				nameW := ansi.StringWidth(trackName)
				contentStart = max(0, (width-nameW)/2)
				contentEnd = contentStart + nameW
				lineContent = strings.Repeat(" ", contentStart) + baseStyle.Render(trackName)
			}
		case contentRow == coverH+4:
			// Artist Name
			baseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Secondary))
			if m.zenTransActive && m.zenTransFrame < ZenTransitionMaxFrames {
				progress := float64(m.zenTransFrame) / float64(ZenTransitionMaxFrames)
				lineContent, contentStart, contentEnd = renderSweptText(artistName, width, progress, baseStyle, false)
			} else {
				artW := ansi.StringWidth(artistName)
				contentStart = max(0, (width-artW)/2)
				contentEnd = contentStart + artW
				lineContent = strings.Repeat(" ", contentStart) + baseStyle.Render(artistName)
			}
		case contentRow == coverH+6:
			// Burning Fuse Progress Bar
			contentStart = padLeftBarW
			contentEnd = padLeftBarW + barW
			lineContent = padLeftBar + fuseBar
		default:
			lineContent = ""
		}

		// Inject twinkling stars into empty margins
		if stars, ok := starsAtRow[y]; ok && len(stars) > 0 {
			var sb strings.Builder
			currX := 0
			// If lineContent is empty, whole row is sky
			if lineContent == "" {
				for _, st := range stars {
					if st.x >= currX && st.x < width {
						sb.WriteString(strings.Repeat(" ", st.x-currX))
						starStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(st.color))
						sb.WriteString(starStyle.Render(string(st.char)))
						currX = st.x + 1
					}
				}
				if currX < width {
					sb.WriteString(strings.Repeat(" ", width-currX))
				}
				lineContent = sb.String()
			} else if contentStart >= 0 {
				// Inject stars strictly on the left of contentStart and right of contentEnd
				var leftSb strings.Builder
				leftX := 0
				for _, st := range stars {
					if st.x >= leftX && st.x < contentStart {
						leftSb.WriteString(strings.Repeat(" ", st.x-leftX))
						starStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(st.color))
						leftSb.WriteString(starStyle.Render(string(st.char)))
						leftX = st.x + 1
					}
				}
				if leftX < contentStart {
					leftSb.WriteString(strings.Repeat(" ", contentStart-leftX))
				}

				var rightSb strings.Builder
				rightX := contentEnd
				for _, st := range stars {
					if st.x >= rightX && st.x < width {
						rightSb.WriteString(strings.Repeat(" ", st.x-rightX))
						starStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(st.color))
						rightSb.WriteString(starStyle.Render(string(st.char)))
						rightX = st.x + 1
					}
				}
				if rightX < width {
					rightSb.WriteString(strings.Repeat(" ", width-rightX))
				}

				// Extract raw center content without initial padLeft
				centerOnly := ansi.Truncate(lineContent, width, "")
				if strings.HasPrefix(centerOnly, strings.Repeat(" ", contentStart)) {
					centerOnly = centerOnly[contentStart:]
				}
				lineContent = leftSb.String() + centerOnly + rightSb.String()
			}
		}

		lines[y] = fit(lineContent)
	}

	// Paint the reactive canvas last so the scrim only lands on rows with content
	// (the starry sky and empty margins keep the full animated gradient).
	lines = canvas.PaintLines(lines)

	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.ReportFocus = true
	return view
}

// computeRhythmBorderColor calculates a subtle, elegant audio-reactive border color
// that pulses minimally in sync with the song's playback progress (~100-120 BPM simulation).
// In pause state, it rests serenely in a calm, dimmed tone.
func computeRhythmBorderColor(baseColor string, isPlaying bool, positionMS int, frame int) string {
	if !isPlaying {
		return LerpHex(baseColor, "#3f3f46", 0.40)
	}

	// 115 BPM rhythm pulse (~520ms per beat)
	beatPeriod := 520
	phase := float64(positionMS%beatPeriod) / float64(beatPeriod)

	// Exponential attack & decay beat pulse
	pulse := math.Exp(-3.5 * phase)

	// Gentle ambient wave modulation
	wave := 0.5 + 0.5*math.Sin(2.0*math.Pi*float64(frame)/32.0)

	// Minimal light boost: +15% to +22% brightness, keeping it subtle and zen
	boost := 0.16*pulse + 0.06*wave

	// Elevate toward a soft luminous silver-white / tone
	glowTone := LerpHex(baseColor, "#ffffff", 0.65)
	return LerpHex(baseColor, glowTone, boost)
}

// renderSweptText renders text with a luminous beam scan reveal (inspired by the intro scan).
// It features a staggered cascade between title and artist, with a radiant incandescent head and warm trail.
func renderSweptText(text string, width int, globalProgress float64, baseStyle lipgloss.Style, isTitle bool) (string, int, int) {
	if text == "" {
		return "", 0, 0
	}
	if globalProgress >= 1.0 {
		styled := centerText(text, width, baseStyle)
		w := ansi.StringWidth(styled)
		start := max(0, (width-w)/2)
		return styled, start, start + w
	}

	runes := []rune(text)
	textLen := len(runes)
	if textLen == 0 {
		return "", 0, 0
	}

	// Staggered timing: Title sweeps from 0.04 to 0.82; Artist sweeps from 0.20 to 0.95
	var startT, endT float64
	if isTitle {
		startT, endT = 0.04, 0.82
	} else {
		startT, endT = 0.20, 0.95
	}

	if globalProgress < startT {
		// Not started yet: completely hidden
		totalW := ansi.StringWidth(text)
		pad := max(0, (width-totalW)/2)
		return strings.Repeat(" ", pad+totalW), pad, pad + totalW
	}

	textProgress := (globalProgress - startT) / (endT - startT)
	if textProgress >= 1.0 {
		styled := centerText(text, width, baseStyle)
		w := ansi.StringWidth(styled)
		start := max(0, (width-w)/2)
		return styled, start, start + w
	}

	// Beam travels across string length + margin
	headPos := int(textProgress * float64(textLen+4))

	var sb strings.Builder
	for i, ch := range runes {
		switch {
		case i < headPos-3:
			// Fully scanned and settled: base style
			sb.WriteString(baseStyle.Render(string(ch)))
		case i == headPos-3 || i == headPos-2:
			// Warm trailing glow: golden amber for title, radiant cyan for artist
			trailCol := "#fef08a"
			if !isTitle {
				trailCol = "#67e8f9"
			}
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(trailCol)).Bold(true)
			sb.WriteString(style.Render(string(ch)))
		case i == headPos-1:
			// Inner beam halo: brilliant light
			haloCol := "#ffffff"
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(haloCol)).Bold(true)
			sb.WriteString(style.Render(string(ch)))
		case i == headPos:
			// Beam head: incandescent white core
			headStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Bold(true)
			sb.WriteString(headStyle.Render(string(ch)))
		case i == headPos+1:
			// Faint anticipation spark at the threshold
			anticipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
			sb.WriteString(anticipStyle.Render("·"))
		default:
			// Ahead of beam: asleep / hidden
			sb.WriteString(" ")
		}
	}

	renderedText := sb.String()
	totalW := ansi.StringWidth(text)
	pad := max(0, (width-totalW)/2)

	line := strings.Repeat(" ", pad) + renderedText
	return line, pad, pad + totalW
}
