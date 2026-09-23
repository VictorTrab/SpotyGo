package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
)

func pressSpace(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	return next.(Model)
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
