package ui

import (
	"fmt"
)

// This file centralizes the friendly, Spanish wording used by toasts and status messages.
//
// House rules:
//   - Everything the UI says to the user is Spanish and warm, never robotic.
//   - A message states what actually happened. Optimistic actions use the gerund
//     ("Cambiando…") because Spotify has not confirmed them yet.

const (
	// Transport feedback.
	toastPlaying = "▶ Reproduciendo"
	toastPaused  = "⏸ En pausa"
	toastNext    = "⏭ Siguiente canción"
	toastPrev    = "⏮ Canción anterior"

	// Navigation and views.
	toastPlaylists = "♪ Tus playlists"
	toastZen       = "♪ Carátula grande (pulsa 'z' o Esc para volver)"
	toastCommands  = "Comandos: /background, /login, /update, /changelog, /version, /help"

	// Settings.
	toastThemeApplied   = "Tema %s listo ♪"
	toastBackgroundSet  = "Modo %s activado"
	toastKeepForLater   = "Sin prisa, lo dejamos para luego"
	toastUpToDate       = "Estás al día, %s"
	toastThemeCancelled = "Tema sin cambios"

	// Status pills (header), kept here so tests and wording stay in sync.
	statusNext = "Buscando la siguiente…"
	statusPrev = "Volviendo a la anterior…"
)

// volumeToast renders the friendly volume feedback shared by the volume keys.
func volumeToast(percent int) string {
	return fmt.Sprintf("🔊 Volumen al %d%%", percent)
}

// backgroundLabel names a background mode the way the picker shows it.
func backgroundLabel(mode string) string {
	switch mode {
	case "flow":
		return "flow (solo Zen)"
	case "dark":
		return "dark del tema"
	default:
		return "gradient"
	}
}

// Background picker order: flow (the default) first, then gradient, then dark.
const bgPickerOptions = 3

// bgPickerIndex maps a background mode to its row in the picker.
func bgPickerIndex(mode string) int {
	switch mode {
	case "dark":
		return 2
	case "gradient", "default":
		return 1
	default: // flow
		return 0
	}
}

// bgPickerMode maps a picker row back to the background mode it applies.
func bgPickerMode(index int) string {
	switch index {
	case 2:
		return "dark"
	case 1:
		return "gradient"
	default:
		return "flow"
	}
}
