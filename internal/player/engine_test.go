package player

import (
	"testing"
)

func TestParseVolume(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "100% volume (65535)",
			input:    "[2026-09-24T00:33:41Z INFO  librespot_connect::spirc] delayed volume update for all devices: volume is now 65535",
			expected: 100,
		},
		{
			name:     "95% volume (62463)",
			input:    "[2026-09-24T00:33:52Z INFO  librespot_connect::spirc] delayed volume update for all devices: volume is now 62463",
			expected: 95,
		},
		{
			name:     "52% volume (33791)",
			input:    "[2026-09-24T00:34:30Z INFO  librespot_connect::spirc] delayed volume update for all devices: volume is now 33791",
			expected: 52,
		},
		{
			name:     "30% volume (19456)",
			input:    "[2026-09-24T00:33:55Z INFO  librespot_connect::spirc] delayed volume update for all devices: volume is now 19456",
			expected: 30,
		},
		{
			name:     "5% volume (3072)",
			input:    "[2026-09-24T00:34:02Z INFO  librespot_connect::spirc] delayed volume update for all devices: volume is now 3072",
			expected: 5,
		},
		{
			name:     "0% volume (0)",
			input:    "[2026-09-24T00:34:01Z INFO  librespot_connect::spirc] delayed volume update for all devices: volume is now 0",
			expected: 0,
		},
		{
			name:     "Direct 0..100 scale: 75",
			input:    "volume is now: 75",
			expected: 75,
		},
		{
			name:     "Non-volume line: track loading",
			input:    "[2026-09-24T00:31:42Z INFO  librespot_playback::player] Loading <Stay the Night> with Spotify URI <spotify:track:7zJnmSjZKjntHmOvEokGb3>",
			expected: -1,
		},
		{
			name:     "Non-volume line: track loaded",
			input:    "[2026-09-24T00:31:42Z INFO  librespot_playback::player] <Stay the Night> (264893 ms) loaded",
			expected: -1,
		},
		{
			name:     "Empty line",
			input:    "",
			expected: -1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVolume(tc.input)
			if got != tc.expected {
				t.Errorf("parseVolume(%q) = %d; want %d", tc.input, got, tc.expected)
			}
		})
	}
}

func TestParseTrackEvents(t *testing.T) {
	loadLine := "[2026-09-24T00:31:42Z INFO  librespot_playback::player] Loading <Stay the Night> with Spotify URI <spotify:track:7zJnmSjZKjntHmOvEokGb3>"
	name, uri, ok := parseTrackLoading(loadLine)
	if !ok {
		t.Fatal("expected ok=true for parseTrackLoading")
	}
	if name != "Stay the Night" {
		t.Fatalf("expected name 'Stay the Night', got %q", name)
	}
	if uri != "spotify:track:7zJnmSjZKjntHmOvEokGb3" {
		t.Fatalf("expected URI 'spotify:track:7zJnmSjZKjntHmOvEokGb3', got %q", uri)
	}

	loadedLine := "[2026-09-24T00:31:42Z INFO  librespot_playback::player] <Stay the Night> (264893 ms) loaded"
	nameLoaded, dur, okLoaded := parseTrackLoaded(loadedLine)
	if !okLoaded {
		t.Fatal("expected ok=true for parseTrackLoaded")
	}
	if nameLoaded != "Stay the Night" {
		t.Fatalf("expected name 'Stay the Night', got %q", nameLoaded)
	}
	if dur != 264893 {
		t.Fatalf("expected duration 264893, got %d", dur)
	}

	// Negative cases
	if _, _, ok := parseTrackLoading("some random line"); ok {
		t.Fatal("expected ok=false for random line in parseTrackLoading")
	}
	if _, _, ok := parseTrackLoaded("some random line"); ok {
		t.Fatal("expected ok=false for random line in parseTrackLoaded")
	}
}

