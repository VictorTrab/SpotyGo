package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/charmbracelet/x/ansi"
)

func pressSpace(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	return next.(Model)
}

func TestPlaylistNavigationAndRestrictedEntries(t *testing.T) {
	m := New(nil, "PC", nil)
	m.showPlaylists = true
	m.playlistSeq = 1
	page := spotify.PlaylistPage{Items: []spotify.Playlist{{ID: "one", Name: "Primera"}, {ID: "two", Name: "Segunda"}}}
	next, _ := m.Update(playlistsMsg{page: page, seq: 1})
	m = next.(Model)
	if len(m.playlists) != 2 || m.playlistBusy {
		t.Fatal("playlist list was not loaded")
	}
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = next.(Model)
	if m.playlistPick != 1 {
		t.Fatal("j did not select the second playlist")
	}
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(Model)
	if !m.showTracks || m.activePlaylist.ID != "two" {
		t.Fatal("Enter did not open the selected playlist")
	}
	next, _ = m.Update(playlistEntriesMsg{err: errors.New("Spotify HTTP 403: Forbidden"), seq: m.entriesSeq, playlistID: "two"})
	m = next.(Model)
	if !m.statusError || !strings.Contains(m.status, "playlists seguidas") {
		t.Fatal("restricted playlists need a useful message")
	}
}

func TestViewFillsTerminalAndHidesConfirmedStatus(t *testing.T) {
	m := New(nil, "PC", nil)
	m.width, m.height = 100, 30
	m.status = "Listo: acción"
	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) != 30 {
		t.Fatalf("expected 30 terminal rows, got %d", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width != 100 {
			t.Fatalf("row %d has width %d, expected 100", i, width)
		}
	}
	if strings.Contains(view.Content, "Listo: acción") {
		t.Fatal("confirmed status should not remain visible")
	}
}

func TestSpaceIgnoresDuplicateAndOldState(t *testing.T) {
	m := New(nil, "PC", nil)
	m.state = spotify.PlaybackState{Available: true, IsPlaying: true}
	m = pressSpace(m)
	if !m.transportBusy || m.state.IsPlaying || m.desiredPlaying == nil || *m.desiredPlaying {
		t.Fatal("first space must request a pause")
	}
	old, _ := m.Update(stateMsg{state: spotify.PlaybackState{Available: true, IsPlaying: true}, epoch: 0})
	m = old.(Model)
	if m.state.IsPlaying {
		t.Fatal("old playback response replaced the pending pause")
	}
	next, _ := m.Update(actionMsg{name: "pausar"})
	m = pressSpace(next.(Model))
	if m.state.IsPlaying || m.transportBusy {
		t.Fatal("duplicate space must not resume playback")
	}
}

func TestVolumeKeepsConfirmedValueDuringSpotifyDelay(t *testing.T) {
	m := New(nil, "PC", nil)
	oldVolume := 50
	m.state = spotify.PlaybackState{Available: true, Device: &spotify.Device{ID: "pc", VolumePercent: &oldVolume}}
	next, _ := m.Update(actionMsg{name: "volumen", seq: 0, value: 55})
	m = next.(Model)
	if m.state.Device.VolumePercent == nil || *m.state.Device.VolumePercent != 55 {
		t.Fatal("confirmed volume must be visible immediately")
	}
	next, _ = m.Update(stateMsg{epoch: m.stateEpoch, state: spotify.PlaybackState{Available: true, Device: &spotify.Device{ID: "pc", VolumePercent: &oldVolume}}})
	m = next.(Model)
	if m.state.Device.VolumePercent == nil || *m.state.Device.VolumePercent != 55 {
		t.Fatal("late Spotify response must not revert the displayed volume")
	}
}
