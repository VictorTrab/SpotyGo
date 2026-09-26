package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/VictorTrab/SpotyGo/internal/logger"
)

// Theme encapsulates the visual styling of the SpotifyGo terminal UI.
type Theme struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Accent      string `json:"accent"`
	Secondary   string `json:"secondary"`
	Border      string `json:"border"`
	Text        string `json:"text"`
	Muted       string `json:"muted"`
	Dim         string `json:"dim"`
	Warning     string `json:"warning"`
	Error       string `json:"error"`
	Playing     string `json:"playing"`
	WaveTop     string `json:"wave_top"`
	WaveBot     string `json:"wave_bot"`
	PillKeyFg   string `json:"pill_key_fg"`
	PillKeyBg   string `json:"pill_key_bg"`
	BgBase      string `json:"bg_base"`
}

var (
	ThemeSpotifyDark = Theme{
		ID:          "spotify-dark",
		Name:        "Spotify Dark",
		Description: "Clásico verde Spotify con fondos carbón y slate",
		Accent:      "#1DB954",
		Secondary:   "#22D3EE",
		Border:      "#3F3F46",
		Text:        "#F8FAFC",
		Muted:       "#94A3B8",
		Dim:         "#27272A",
		Warning:     "#F5B841",
		Error:       "#EF4444",
		Playing:     "#38BDF8",
		WaveTop:     "#1DB954",
		WaveBot:     "#22D3EE",
		PillKeyFg:   "#1DB954",
		PillKeyBg:   "#182E1E",
		BgBase:      "#0c0d0e",
	}

	ThemeCyberpunk = Theme{
		ID:          "cyberpunk",
		Name:        "Cyberpunk Neon",
		Description: "Retrofuturista con fucsia neón, cian y amarillo",
		Accent:      "#FF007F",
		Secondary:   "#00F0FF",
		Border:      "#FF007F",
		Text:        "#FFFFFF",
		Muted:       "#A855F7",
		Dim:         "#4A0E4E",
		Warning:     "#FFE600",
		Error:       "#FF1744",
		Playing:     "#00F0FF",
		WaveTop:     "#FF007F",
		WaveBot:     "#00F0FF",
		PillKeyFg:   "#00F0FF",
		PillKeyBg:   "#2A0845",
		BgBase:      "#0d0221",
	}

	ThemeTokyoNight = Theme{
		ID:          "tokyo-night",
		Name:        "Tokyo Night",
		Description: "Paleta nocturna con violeta, rosa y cian suave",
		Accent:      "#BD93F9",
		Secondary:   "#FF79C6",
		Border:      "#6272A4",
		Text:        "#F8F8F2",
		Muted:       "#8BE9FD",
		Dim:         "#44475A",
		Warning:     "#F1FA8C",
		Error:       "#FF5555",
		Playing:     "#50FA7B",
		WaveTop:     "#BD93F9",
		WaveBot:     "#FF79C6",
		PillKeyFg:   "#BD93F9",
		PillKeyBg:   "#382A54",
		BgBase:      "#1a1b26",
	}

	ThemeNord = Theme{
		ID:          "nord",
		Name:        "Nord Arctic",
		Description: "Tonalidades del ártico con azul hielo y verde aurora",
		Accent:      "#88C0D0",
		Secondary:   "#81A1C1",
		Border:      "#4C566A",
		Text:        "#ECEFF4",
		Muted:       "#D8DEE9",
		Dim:         "#3B4252",
		Warning:     "#EBCB8B",
		Error:       "#BF616A",
		Playing:     "#A3BE8C",
		WaveTop:     "#88C0D0",
		WaveBot:     "#A3BE8C",
		PillKeyFg:   "#88C0D0",
		PillKeyBg:   "#2E3440",
		BgBase:      "#242933",
	}

	ThemeLightMinimal = Theme{
		ID:          "light-minimal",
		Name:        "Light Minimal",
		Description: "Estilo limpio de alto contraste con azul índigo y esmeralda",
		Accent:      "#3730A3",
		Secondary:   "#047857",
		Border:      "#64748B",
		Text:        "#0F172A",
		Muted:       "#334155",
		Dim:         "#64748B",
		Warning:     "#B45309",
		Error:       "#B91C1C",
		Playing:     "#1D4ED8",
		WaveTop:     "#3730A3",
		WaveBot:     "#047857",
		PillKeyFg:   "#FFFFFF",
		PillKeyBg:   "#3730A3",
		BgBase:      "#F8FAFC",
	}

	allThemes = []Theme{
		ThemeSpotifyDark,
		ThemeCyberpunk,
		ThemeTokyoNight,
		ThemeNord,
		ThemeLightMinimal,
	}
)

// IsLight reports whether the theme uses a light background canvas.
func (t Theme) IsLight() bool {
	if t.ID == "light-minimal" {
		return true
	}
	s := strings.TrimPrefix(t.BgBase, "#")
	if len(s) != 6 {
		return false
	}
	var r, g, b uint8
	if _, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b); err != nil {
		return false
	}
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	return lum > 140
}

// All returns all registered themes.
func All() []Theme {
	return allThemes
}

// Get finds a theme by ID or returns ThemeSpotifyDark if not found.
func Get(id string) Theme {
	id = strings.TrimSpace(strings.ToLower(id))
	for _, t := range allThemes {
		if t.ID == id || strings.ToLower(t.Name) == id {
			return t
		}
	}
	return ThemeSpotifyDark
}

// Style helpers
func (t Theme) StyleAccent() lipgloss.Style    { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Accent)).Bold(true) }
func (t Theme) StyleSecondary() lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Secondary)).Bold(true) }
func (t Theme) StyleBorder() lipgloss.Style    { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Border)) }
func (t Theme) StyleText() lipgloss.Style      { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Text)).Bold(true) }
func (t Theme) StyleMuted() lipgloss.Style     { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Muted)) }
func (t Theme) StyleDim() lipgloss.Style       { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Dim)) }
func (t Theme) StyleWarning() lipgloss.Style   { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Warning)) }
func (t Theme) StyleError() lipgloss.Style     { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Error)) }
func (t Theme) StylePlaying() lipgloss.Style   { return lipgloss.NewStyle().Foreground(lipgloss.Color(t.Playing)).Bold(true) }

// Config represents user persistent preferences.
type Config struct {
	Theme      string `json:"theme"`
	Bitrate    string `json:"bitrate"`
	Background string `json:"background"`
}

func configPath() string {
	var baseDir string
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		baseDir = filepath.Join(localAppData, "SpotifyGo")
	} else if home, err := os.UserHomeDir(); err == nil {
		baseDir = filepath.Join(home, ".config", "SpotifyGo")
	} else {
		baseDir = "."
	}
	_ = os.MkdirAll(baseDir, 0o755)
	return filepath.Join(baseDir, "config.json")
}

// LoadConfig loads user configuration or defaults.
func LoadConfig() Config {
	p := configPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return Config{Theme: "spotify-dark", Bitrate: "320", Background: "gradient"}
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{Theme: "spotify-dark", Bitrate: "320", Background: "gradient"}
	}
	if cfg.Theme == "" {
		cfg.Theme = "spotify-dark"
	}
	if cfg.Bitrate == "" {
		cfg.Bitrate = "320"
	}
	if cfg.Background == "" {
		cfg.Background = "gradient"
	}
	return cfg
}

// SaveConfig saves the configuration to disk.
func SaveConfig(cfg Config) error {
	p := configPath()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		logger.Error("Error al codificar configuración: %v", err)
		return err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		logger.Error("Error al guardar archivo de configuración en %s: %v", p, err)
		return err
	}
	return nil
}

// SaveBitrate updates and persists the audio bitrate setting.
func SaveBitrate(bitrate string) error {
	cfg := LoadConfig()
	cfg.Bitrate = bitrate
	return SaveConfig(cfg)
}

// SaveTheme updates and persists the theme setting.
func SaveTheme(themeID string) error {
	cfg := LoadConfig()
	cfg.Theme = themeID
	return SaveConfig(cfg)
}

// SaveBackground updates and persists the background mode setting.
func SaveBackground(bg string) error {
	cfg := LoadConfig()
	cfg.Background = bg
	return SaveConfig(cfg)
}
