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
	if len(themes) != 5 {
		t.Fatalf("expected 5 themes, got %d", len(themes))
	}
}

func TestThemeIsLight(t *testing.T) {
	light := Get("light-minimal")
	if !light.IsLight() {
		t.Fatal("expected light-minimal to be light")
	}

	dark := Get("spotify-dark")
	if dark.IsLight() {
		t.Fatal("expected spotify-dark not to be light")
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
