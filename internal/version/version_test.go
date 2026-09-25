package version

import (
	"strings"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := Info()
	if !strings.HasPrefix(info, "SpotifyGo v") {
		t.Errorf("expected version info to start with 'SpotifyGo v', got %q", info)
	}
	if Current == "" {
		t.Error("Current version string should not be empty")
	}
}
