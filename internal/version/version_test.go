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

func TestSemver(t *testing.T) {
	tests := []struct {
		remote  string
		local   string
		isNewer bool
	}{
		{"v1.0.5", "v1.0.4", true},
		{"v1.0.4", "v1.0.3", true},
		{"v1.0.3", "v1.0.2", true},
		{"v1.1.0", "v1.0.2", true},
		{"v2.0.0", "v1.9.9", true},
		{"v1.0.2", "v1.0.2", false},
		{"v1.0.1", "v1.0.2", false},
		{"v1.0.0", "v1.0.2", false},
		{"1.0.3", "1.0.2", true},
	}

	for _, tt := range tests {
		got := IsNewer(tt.remote, tt.local)
		if got != tt.isNewer {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.remote, tt.local, got, tt.isNewer)
		}
	}
}

func TestRecentHighlights(t *testing.T) {
	hl := RecentHighlights()
	if len(hl) == 0 {
		t.Fatal("expected at least one highlight entry")
	}
	if hl[0].Version == "" || len(hl[0].Points) == 0 {
		t.Fatalf("invalid highlight entry: %+v", hl[0])
	}
}
