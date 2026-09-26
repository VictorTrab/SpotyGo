package theme

import (
	"os"
	"testing"
)

func TestThemeGet(t *testing.T) {
	th := Get("cyberpunk")
	if th.ID != "cyberpunk" {
		t.Fatalf("expected cyberpunk, got %s", th.ID)
	}

	fallback := Get("non-existent-theme")
	if fallback.ID != "spotify-dark" {
		t.Fatalf("expected spotify-dark fallback, got %s", fallback.ID)
	}
}

func TestAllThemes(t *testing.T) {
	themes := All()
	if len(themes) != 4 {
		t.Fatalf("expected 4 themes, got %d", len(themes))
	}
}

func TestThemeIsLight(t *testing.T) {
	for _, th := range All() {
		if th.IsLight() {
			t.Fatalf("expected all themes to be dark, but %s is light", th.ID)
		}
	}
}

func TestConfigSaveLoad(t *testing.T) {
	tempDir := t.TempDir()
	origLocal := os.Getenv("LOCALAPPDATA")
	defer os.Setenv("LOCALAPPDATA", origLocal)
	os.Setenv("LOCALAPPDATA", tempDir)

	cfg := Config{Theme: "nord"}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded := LoadConfig()
	if loaded.Theme != "nord" {
		t.Fatalf("expected nord, got %s", loaded.Theme)
	}
	if loaded.Bitrate != "320" {
		t.Fatalf("expected default bitrate 320, got %s", loaded.Bitrate)
	}

	if err := SaveBitrate("160"); err != nil {
		t.Fatalf("SaveBitrate failed: %v", err)
	}
	loaded = LoadConfig()
	if loaded.Bitrate != "160" {
		t.Fatalf("expected bitrate 160, got %s", loaded.Bitrate)
	}
}

// useTempConfigDir points the config path at a throwaway directory.
func useTempConfigDir(t *testing.T) {
	t.Helper()
	tempDir := t.TempDir()
	orig := os.Getenv("LOCALAPPDATA")
	t.Cleanup(func() { _ = os.Setenv("LOCALAPPDATA", orig) })
	_ = os.Setenv("LOCALAPPDATA", tempDir)
}

func TestLegacyBackgroundMigratesToFlowOnce(t *testing.T) {
	useTempConfigDir(t)

	// What the previous version stored when the ambiguous first option was picked.
	if err := SaveConfig(Config{Theme: "nord", Bitrate: "320", Background: "gradient"}); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded := LoadConfig()
	if loaded.Background != "flow" {
		t.Fatalf("legacy 'gradient' must migrate to flow, got %q", loaded.Background)
	}
	if !loaded.BackgroundMigrated {
		t.Fatal("the migration must be recorded so it only runs once")
	}

	// A deliberate gradient choice made afterwards must be respected.
	if err := SaveBackground("gradient"); err != nil {
		t.Fatalf("SaveBackground failed: %v", err)
	}
	if got := LoadConfig().Background; got != "gradient" {
		t.Fatalf("an explicit gradient choice must stick after the migration, got %q", got)
	}
}

func TestExplicitDarkBackgroundSurvivesMigration(t *testing.T) {
	useTempConfigDir(t)

	if err := SaveConfig(Config{Theme: "nord", Bitrate: "320", Background: "dark"}); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	if got := LoadConfig().Background; got != "dark" {
		t.Fatalf("an explicit dark choice must not be migrated, got %q", got)
	}
}

func TestFreshInstallDefaultsToFlow(t *testing.T) {
	useTempConfigDir(t)

	cfg := LoadConfig()
	if cfg.Background != "flow" || cfg.Theme != "spotify-dark" || cfg.Bitrate != "320" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}
