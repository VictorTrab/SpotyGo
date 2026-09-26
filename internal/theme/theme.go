package theme

import (
	"encoding/json"
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
		Description: "Clásico verde Spotify con fondos carbón y slate de alto contraste",
		Accent:      "#1ED760",
		Secondary:   "#38BDF8",
		Border:      "#52525B",
		Text:        "#FFFFFF",
		Muted:       "#CBD5E1",
		Dim:         "#64748B",
		Warning:     "#FBBF24",
		Error:       "#F87171",
		Playing:     "#38BDF8",
		WaveTop:     "#1ED760",
		WaveBot:     "#38BDF8",
		PillKeyFg:   "#1ED760",
		PillKeyBg:   "#143820",
		BgBase:      "#0c0d0e",
	}

	ThemeCyberpunk = Theme{
		ID:          "cyberpunk",
		Name:        "Cyberpunk Neon",
		Description: "Retrofuturista con fucsia neón, cian eléctrico y alto contraste",
		Accent:      "#FF007F",
		Secondary:   "#00F0FF",
		Border:      "#FF1493",
		Text:        "#FFFFFF",
		Muted:       "#E2E8F0",
		Dim:         "#7C3AED",
		Warning:     "#FACC15",
		Error:       "#FF3366",
		Playing:     "#00F0FF",
		WaveTop:     "#FF007F",
		WaveBot:     "#00F0FF",
		PillKeyFg:   "#00F0FF",
		PillKeyBg:   "#3B0764",
		BgBase:      "#0a0118",
	}

	ThemeTokyoNight = Theme{
		ID:          "tokyo-night",
		Name:        "Tokyo Night",
		Description: "Paleta nocturna nítida con violeta, rosa y azul eléctrico",
		Accent:      "#C084FC",
		Secondary:   "#F472B6",
		Border:      "#7AA2F7",
		Text:        "#FFFFFF",
		Muted:       "#E2E8F0",
		Dim:         "#6B7280",
		Warning:     "#FBBF24",
		Error:       "#F87171",
		Playing:     "#4ADE80",
		WaveTop:     "#C084FC",
		WaveBot:     "#F472B6",
		PillKeyFg:   "#C084FC",
		PillKeyBg:   "#382A54",
		BgBase:      "#13141f",
	}

	ThemeNord = Theme{
		ID:          "nord",
		Name:        "Nord Arctic",
		Description: "Tonalidades del ártico nítidas con azul hielo y verde aurora",
		Accent:      "#88C0D0",
		Secondary:   "#81A1C1",
		Border:      "#616E88",
		Text:        "#FFFFFF",
		Muted:       "#E5E9F0",
		Dim:         "#4C566A",
		Warning:     "#EBCB8B",
		Error:       "#BF616A",
		Playing:     "#A3BE8C",
		WaveTop:     "#88C0D0",
		WaveBot:     "#A3BE8C",
		PillKeyFg:   "#88C0D0",
		PillKeyBg:   "#2E3440",
		BgBase:      "#1e222a",
	}

	allThemes = []Theme{
		ThemeSpotifyDark,
		ThemeCyberpunk,
		ThemeTokyoNight,
		ThemeNord,
	}
)

// IsLight reports whether the theme uses a light background canvas (always false as light theme was removed).
func (t Theme) IsLight() bool {
	return false
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
		return Config{Theme: "spotify-dark", Bitrate: "320", Background: "flow"}
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{Theme: "spotify-dark", Bitrate: "320", Background: "flow"}
	}
	if cfg.Theme == "" {
		cfg.Theme = "spotify-dark"
	}
	if cfg.Bitrate == "" {
		cfg.Bitrate = "320"
	}
	if cfg.Background == "" {
		cfg.Background = "flow"
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
