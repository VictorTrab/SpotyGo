package ui

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/VictorTrab/SpotyGo/internal/theme"
)

var asciiLogoBlock = []string{
	`███████╗██████╗  ██████╗ ████████╗██╗███████╗██╗   ██╗ ██████╗  ██████╗ `,
	`██╔════╝██╔══██╗██╔═══██╗╚══██╔══╝██║██╔════╝╚██╗ ██╔╝██╔════╝ ██╔═══██╗`,
	`███████╗██████╔╝██║   ██║   ██║   ██║█████╗   ╚████╔╝ ██║  ███╗██║   ██║`,
	`╚════██║██╔═══╝ ██║   ██║   ██║   ██║██╔══╝    ╚██╔╝  ██║   ██║██║   ██║`,
	`███████║██║     ╚██████╔╝   ██║   ██║██║        ██║   ╚██████╔╝╚██████╔╝`,
	`╚══════╝╚═╝      ╚═════╝    ╚═╝   ╚═╝╚═╝        ╚═╝    ╚═════╝  ╚═════╝ `,
}

// Compact 3-line version for tight terminal widths (< 80)
var asciiLogoCompact = []string{
	` ___ ___  ___ _____ ___ ___ _   _  ____  ___ `,
	`/__ | _ \/ _ \  |   |  | __| | | |/ ___/ _ \ `,
	`\_/ |  _/ (_) | |  _|_ |_|  \__, |\____\___/ `,
}

// RenderBeamIntro renders the animated beam scanner across the SPOTIFYGO banner.
// It dynamically animates a traveling luminous beam inspired by sysc-Go's beam-text effect.
func RenderBeamIntro(th theme.Theme, frame int, width, height int) []string {
	var banner []string
	splitX := 56 // boundary between SPOTIFY and GO in block logo
	logoW := 74

	if width < 80 {
		banner = asciiLogoCompact
		splitX = 33
		logoW = 46
	} else {
		banner = asciiLogoBlock
	}

	accentColor := th.Accent
	secondaryColor := th.Secondary
	dimColor := "#1e293b"      // dormant dark letter
	glowColor := "#ffffff"     // beam core brilliant white
	trailingColor := "#86efac" // neon green trailing beam

	// Sweep phase: 0..65 frames (~2s)
	// Ambient shimmer phase: 66..165 frames (~2s to 5s)
	sweepFrames := 65

	var renderedBanner []string
	for _, rawLine := range banner {
		var b strings.Builder
		runes := []rune(rawLine)
		for x, ch := range runes {
			if ch == ' ' {
				b.WriteRune(' ')
				continue
			}

			var colorHex string
			var bold bool

			if frame <= sweepFrames {
				// Phase 1: Beam scanner sweeps left to right
				beamProgress := float64(frame) / float64(sweepFrames)
				beamX := int(beamProgress*float64(logoW+20)) - 10
				dist := x - beamX

				if math.Abs(float64(dist)) <= 1 {
					// Core beam: brilliant white glow
					colorHex = glowColor
					bold = true
				} else if dist < 0 && dist >= -4 {
					// Trailing edge of beam: neon flash
					colorHex = trailingColor
					bold = true
				} else if dist < -4 {
					// Already charged: theme color
					if x < splitX {
						colorHex = accentColor
					} else {
						colorHex = secondaryColor
					}
					bold = true
				} else if dist > 0 && dist <= 4 {
					// Leading edge: soft approach
					colorHex = "#15803d"
				} else {
					// Unreached: dormant silhouette
					colorHex = dimColor
				}
			} else {
				// Phase 2: All letters charged + pulsing periodic shimmer wave
				baseColor := accentColor
				if x >= splitX {
					baseColor = secondaryColor
				}
				bold = true

				// Gentle periodic ambient shimmer wave
				shimmerCycle := 45
				shimmerOffset := (frame - sweepFrames) % shimmerCycle
				shimmerX := int(float64(shimmerOffset)/float64(shimmerCycle)*float64(logoW+20)) - 10
				sDist := math.Abs(float64(x - shimmerX))

				if sDist <= 1 {
					colorHex = glowColor
				} else if sDist <= 3 {
					colorHex = trailingColor
				} else {
					colorHex = baseColor
				}
			}

			style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex))
			if bold {
				style = style.Bold(true)
			}
			b.WriteString(style.Render(string(ch)))
		}
		renderedBanner = append(renderedBanner, b.String())
	}

	return renderedBanner
}
