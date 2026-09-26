package ui

import (
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/VictorTrab/SpotyGo/internal/player"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/theme"
	"github.com/VictorTrab/SpotyGo/internal/version"
	"github.com/charmbracelet/x/ansi"
)

func pressSpace(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace, Text: " "}))
	return next.(Model)
}

func pressKey(m Model, k string) Model {
	switch k {
	case "enter":
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		return next.(Model)
	case "esc":
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
		return next.(Model)
	default:
		var r rune
		if len(k) > 0 {
			r = rune(k[0])
		}
		next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: k}))
		return next.(Model)
	}
}

func TestPlaylistNavigationAndRestrictedEntries(t *testing.T) {
	m := New(nil, "PC", nil, nil)
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
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	m.status = "Listo: acción"
	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) != 29 {
		t.Fatalf("expected 29 terminal rows, got %d", len(lines))
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
	m := New(nil, "PC", nil, nil)
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
	m := New(nil, "PC", nil, nil)
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

func TestCommandModeAndPrefixResolutions(t *testing.T) {
	m := New(nil, "PC", nil, nil)

	// Press '/' to open command mode
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m = next.(Model)
	if !m.commandActive {
		t.Fatal("pressing / must activate command mode")
	}

	// Type 'q' and press Enter -> should quit
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd == nil {
		t.Fatal("q command must return tea.Quit command")
	}
	m = next.(Model)

	// Press '/' again, type 'th' and press Enter -> should switch to viewThemePicker
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(Model)
	if m.currentView != viewThemePicker {
		t.Fatalf("expected viewThemePicker, got %v", m.currentView)
	}

	// Press Esc to cancel theme picker
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = next.(Model)
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after Esc, got %v", m.currentView)
	}
}

func TestDirectSearchShortcut(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	// Press 'S' to search
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'S', Text: "S"}))
	m = next.(Model)
	if m.currentView != viewSearch {
		t.Fatalf("expected viewSearch on pressing S, got %v", m.currentView)
	}

	// Press Esc to exit search
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = next.(Model)
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after Esc, got %v", m.currentView)
	}
}

func TestDevicesViewDoesNotExitImmediately(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.devices = []spotify.Device{
		{ID: "d1", Name: "Mi Telefono", Type: "Smartphone"},
		{ID: "d2", Name: "PC", Type: "Computer", IsActive: true},
	}

	// Press 'd' to open devices view
	next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	m = next.(Model)
	if m.currentView != viewDevices {
		t.Fatalf("expected viewDevices on pressing d, got %v", m.currentView)
	}

	// Render view to ensure devices table is present and does not exit
	view := m.View()
	if !strings.Contains(view.Content, "DISPOSITIVOS DISPONIBLES") {
		t.Fatal("viewDevices should display DISPOSITIVOS DISPONIBLES header")
	}
	if !strings.Contains(view.Content, "Mi Telefono") {
		t.Fatal("viewDevices should list devices")
	}

	// Press Esc to return
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	m = next.(Model)
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after Esc, got %v", m.currentView)
	}
}

func TestNoRedundantEscVolverInPlaylistHeader(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.activePlaylist = spotify.Playlist{ID: "p1", Name: "Mi Playlist Favorita"}
	m.currentView = viewTracks
	view := m.View()
	if strings.Contains(view.Content, "[Esc Volver]") {
		t.Fatal("playlist header must not contain redundant [Esc Volver]")
	}
	if !strings.Contains(view.Content, "Mi Playlist Favorita") {
		t.Fatal("playlist name must be displayed")
	}
}

func TestNextAndPreviousTrackHotkeys(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	// Press 'n' -> next track
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "n"}))
	m = next.(Model)
	if cmd == nil || !m.transportBusy || !strings.Contains(m.status, "Siguiente") {
		t.Fatal("n must trigger next track")
	}

	m.transportBusy = false
	// Press 'b' -> previous track
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'b', Text: "b"}))
	m = next.(Model)
	if cmd == nil || !m.transportBusy || !strings.Contains(m.status, "anterior") {
		t.Fatal("b must trigger previous track")
	}
}

func TestNoCommandFooterPillsInView(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.currentView = viewPlaylists
	view := m.View()
	if strings.Contains(view.Content, "[p]") || strings.Contains(view.Content, "[Enter]") || strings.Contains(view.Content, "[↑/↓]") {
		t.Fatal("viewPlaylists must not show command shortcut pills in footer")
	}
}

func TestPendingDeviceTimeoutClearsSpinner(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.pendingDeviceID = "dev-123"
	m.pendingDeviceName = "Mi Altavoz"
	m.pendingDeviceSince = time.Now().Add(-3 * time.Second) // 3s ago (exceeded 2.5s)

	// In stateMsg, it should clear pendingDeviceID
	next, _ := m.Update(stateMsg{epoch: m.stateEpoch, state: spotify.PlaybackState{Available: true}})
	m = next.(Model)
	if m.pendingDeviceID != "" {
		t.Fatal("pendingDeviceID must be cleared after timeout")
	}

	view := m.View()
	if strings.Contains(view.Content, "Conectando") {
		t.Fatal("View must not display Conectando after timeout")
	}
}

func TestLocalVolumeEventInstantUpdate(t *testing.T) {
	volCh := make(chan int, 1)
	m := New(nil, "PC", nil, nil, volCh)
	oldVolume := 20
	m.state = spotify.PlaybackState{Available: true, Device: &spotify.Device{ID: "pc", Name: "PC", VolumePercent: &oldVolume}}
	m.devices = []spotify.Device{{ID: "pc", Name: "PC", VolumePercent: &oldVolume}}

	// Send local volume update (e.g. from mobile phone via Spotify Connect / librespot)
	volCh <- 68
	cmd := m.listenVolume()
	if cmd == nil {
		t.Fatal("listenVolume must return a valid command")
	}
	msg := cmd()
	volMsg, ok := msg.(volumeEventMsg)
	if !ok || volMsg.percent != 68 {
		t.Fatalf("expected volumeEventMsg with 68, got %#v", msg)
	}

	// Update model with volumeEventMsg
	next, followCmd := m.Update(volMsg)
	m = next.(Model)
	if followCmd == nil {
		t.Fatal("Update must re-arm listenVolume")
	}
	if m.state.Device == nil || m.state.Device.VolumePercent == nil || *m.state.Device.VolumePercent != 68 {
		t.Fatalf("expected device volume 68, got %v", m.state.Device.VolumePercent)
	}
	if len(m.devices) > 0 && *m.devices[0].VolumePercent != 68 {
		t.Fatalf("expected m.devices volume 68, got %v", m.devices[0].VolumePercent)
	}

	// Stale web poll returning old volume 20 must NOT overwrite the fresh confirmed volume
	next, _ = m.Update(stateMsg{epoch: m.stateEpoch, state: spotify.PlaybackState{Available: true, Device: &spotify.Device{ID: "pc", Name: "PC", VolumePercent: &oldVolume}}})
	m = next.(Model)
	if m.state.Device == nil || m.state.Device.VolumePercent == nil || *m.state.Device.VolumePercent != 68 {
		t.Fatalf("stale poll must not overwrite fresh local volume 68, got %v", m.state.Device.VolumePercent)
	}
}

func TestArrowKeysNextAndPrevious(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{Available: true, IsPlaying: true}

	// Press 'right' -> next track
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, Text: "right"}))
	m = next.(Model)
	if cmd == nil || !m.transportBusy || !strings.Contains(m.status, "Siguiente") {
		t.Fatal("right arrow must trigger next track")
	}

	m.transportBusy = false
	// Press 'left' -> previous track
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft, Text: "left"}))
	m = next.(Model)
	if cmd == nil || !m.transportBusy || !strings.Contains(m.status, "anterior") {
		t.Fatal("left arrow must trigger previous track")
	}
}

func TestQualityCommandAndHotRestart(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	restartedWith := ""
	m.SetEngineRestarter(func(b string) error {
		restartedWith = b
		return nil
	})

	// Enter command mode and execute "/ quality 160"
	m.commandActive = true
	m.commandInput = "quality 160"
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(Model)
	if m.bitrate != "160" {
		t.Fatalf("expected bitrate 160, got %s", m.bitrate)
	}
	if !strings.Contains(m.status, "160 kbps") {
		t.Fatalf("expected status mentioning 160 kbps, got %s", m.status)
	}
	if cmd == nil {
		t.Fatal("expected async restart command")
	}
	msg := cmd()
	if restartMsg, ok := msg.(engineRestartMsg); !ok || restartMsg.bitrate != "160" {
		t.Fatalf("expected engineRestartMsg with 160, got %#v", msg)
	}
	if restartedWith != "160" {
		t.Fatalf("expected restarter called with 160, got %s", restartedWith)
	}

	// Now switch back to 320 with "/ quality high"
	m.commandActive = true
	m.commandInput = "quality high"
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(Model)
	if m.bitrate != "320" {
		t.Fatalf("expected bitrate 320, got %s", m.bitrate)
	}
	_ = cmd()
	if restartedWith != "320" {
		t.Fatalf("expected restarter called with 320, got %s", restartedWith)
	}
}

func TestSimplifiedHeaderAndQualityBadge(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	vol := 60
	m.state = spotify.PlaybackState{
		Available: true,
		Device:    &spotify.Device{ID: "pc", Name: "PC", Type: "Computer", VolumePercent: &vol},
	}
	m.bitrate = "320"

	view := m.View()
	if !strings.Contains(view.Content, "💻 PC") {
		t.Fatalf("header must contain simplified badge '💻 PC', got:\n%s", view.Content)
	}
	if !strings.Contains(view.Content, "HQ 320k") {
		t.Fatalf("header must contain quality badge 'HQ 320k', got:\n%s", view.Content)
	}

	// Change to mobile device and 160k
	m.state.Device = &spotify.Device{ID: "phone", Name: "iPhone", Type: "Smartphone", VolumePercent: &vol}
	m.bitrate = "160"
	view = m.View()
	if !strings.Contains(view.Content, "📱 Móvil") {
		t.Fatalf("header must contain '📱 Móvil' for phone, got:\n%s", view.Content)
	}
	if !strings.Contains(view.Content, "MQ 160k") {
		t.Fatalf("header must contain 'MQ 160k' for 160 bitrate, got:\n%s", view.Content)
	}
}

func TestLocalActiveSuppressesPoll(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.localReady = true
	m.state = spotify.PlaybackState{
		Available: true,
		Device:    &spotify.Device{ID: "pc", Name: "PC", Type: "Computer"},
	}

	if !m.isLocalActive() {
		t.Fatal("isLocalActive must return true when active device matches localName")
	}

	// On pollMsg, it should only return poll() (the tick), NOT fetchState()
	next, cmd := m.Update(pollMsg{})
	_ = next.(Model)
	if cmd == nil {
		t.Fatal("expected poll tick cmd")
	}
}

func TestPlayPauseImmediateOptimisticAndStaleProtection(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 10000,
		Item:       &spotify.Track{DurationMS: 200000},
	}
	m.lastSync = time.Now().Add(-1 * time.Second)

	// 1. User presses Space to Pause
	m = pressSpace(m)
	if m.state.IsPlaying {
		t.Fatal("optimistic state must immediately show paused")
	}
	if m.desiredPlaying == nil || *m.desiredPlaying != false {
		t.Fatal("desiredPlaying must be set to false")
	}
	if m.status != "" {
		t.Fatalf("status should be clean on pause, got: %q", m.status)
	}

	// 2. Action completes successfully
	next, _ := m.Update(actionMsg{name: "pausar"})
	m = next.(Model)
	if m.transportBusy {
		t.Fatal("transportBusy must be cleared once action completes")
	}
	if m.status != "" {
		t.Fatalf("status should remain clean, got: %q", m.status)
	}

	// 3. Stale Spotify Web API response returns IsPlaying: true
	savedPos := m.state.ProgressMS
	next, _ = m.Update(stateMsg{
		epoch: m.stateEpoch,
		state: spotify.PlaybackState{
			Available:  true,
			IsPlaying:  true, // Stale!
			ProgressMS: 12000,
			Item:       &spotify.Track{DurationMS: 200000},
		},
	})
	m = next.(Model)
	if m.state.IsPlaying {
		t.Fatal("stale Spotify API response must NOT overwrite optimistic pause")
	}
	if m.state.ProgressMS != savedPos {
		t.Fatalf("paused progress must remain pinned at %d, got %d", savedPos, m.state.ProgressMS)
	}

	// 4. User resumes after 250ms
	m.lastTransport = time.Now().Add(-250 * time.Millisecond)
	m = pressSpace(m)
	if !m.state.IsPlaying {
		t.Fatal("optimistic state must immediately show playing")
	}
	if m.desiredPlaying == nil || *m.desiredPlaying != true {
		t.Fatal("desiredPlaying must be set to true")
	}

	// 5. Resume action completes
	next, _ = m.Update(actionMsg{name: "reproducir"})
	m = next.(Model)
	if m.transportBusy {
		t.Fatal("transportBusy must be cleared after resume action")
	}

	// 6. Stale Spotify Web API response returns IsPlaying: false
	next, _ = m.Update(stateMsg{
		epoch: m.stateEpoch,
		state: spotify.PlaybackState{
			Available:  true,
			IsPlaying:  false, // Stale!
			ProgressMS: 11000,
			Item:       &spotify.Track{DurationMS: 200000},
		},
	})
	m = next.(Model)
	if !m.state.IsPlaying {
		t.Fatal("stale Spotify API response must NOT revert optimistic play")
	}

	// 7. Spotify catches up
	next, _ = m.Update(stateMsg{
		epoch: m.stateEpoch,
		state: spotify.PlaybackState{
			Available:  true,
			IsPlaying:  true, // Converged!
			ProgressMS: 11500,
			Item:       &spotify.Track{DurationMS: 200000},
		},
	})
	m = next.(Model)
	if m.desiredPlaying != nil {
		t.Fatal("desiredPlaying must be cleared once Spotify converges")
	}
	if !m.state.IsPlaying {
		t.Fatal("state must be playing")
	}
}

func TestPlayPauseErrorReversion(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{Available: true, IsPlaying: true}

	m = pressSpace(m)
	if m.state.IsPlaying {
		t.Fatal("optimistic state should be paused")
	}

	// Network error on pause
	next, _ := m.Update(actionMsg{name: "pausar", err: errors.New("timeout connecting to Spotify")})
	m = next.(Model)
	if !m.state.IsPlaying {
		t.Fatal("state must revert to playing on error")
	}
	if m.desiredPlaying != nil {
		t.Fatal("desiredPlaying must be nil after error")
	}
	if !m.statusError || !strings.Contains(m.status, "timeout connecting to Spotify") {
		t.Fatalf("error status must be set, got: %q", m.status)
	}
}

func TestPlayPlaylistInstantOptimisticColoring(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.devices = []spotify.Device{{ID: "pc", Name: "PC", IsRestricted: false}}
	m.currentView = viewTracks
	m.activePlaylist = spotify.Playlist{ID: "pl1", Name: "My Playlist", URI: "spotify:playlist:pl1"}
	m.entries = []spotify.PlaylistEntry{
		{Position: 0, Track: spotify.Track{Name: "First Song", URI: "spotify:track:t1", DurationMS: 180000}},
		{Position: 1, Track: spotify.Track{Name: "Second Song", URI: "spotify:track:t2", DurationMS: 200000}},
	}

	// User presses 'p' to start playlist at position 0
	next, _ := m.playPlaylist(0)
	m = next.(Model)

	if m.state.Item == nil {
		t.Fatal("expected state.Item to be optimistically set to first track immediately")
	}
	if m.state.Item.URI != "spotify:track:t1" {
		t.Fatalf("expected URI 'spotify:track:t1', got %q", m.state.Item.URI)
	}
	if !m.state.IsPlaying {
		t.Fatal("expected state.IsPlaying to be true immediately")
	}

	// Verify that the view renders the first track as currently playing
	view := m.View()
	if !strings.Contains(view.Content, "♫") {
		t.Fatalf("expected active playing indicator ♫ in view, got:\n%s", view.Content)
	}

	// Simulate stale Spotify response returning empty item or old state
	next, _ = m.Update(stateMsg{
		epoch: m.stateEpoch,
		state: spotify.PlaybackState{
			Available: true,
			IsPlaying: false, // Stale!
			Item:      nil,   // Stale!
		},
	})
	m = next.(Model)
	if m.state.Item == nil || m.state.Item.URI != "spotify:track:t1" {
		t.Fatal("stale Spotify API response must NOT clear optimistic track")
	}
	if !m.state.IsPlaying {
		t.Fatal("stale Spotify API response must NOT pause optimistic playback")
	}
}

func TestTrackEventInstantSwitching(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.entries = []spotify.PlaylistEntry{
		{Position: 0, Track: spotify.Track{Name: "First Song", URI: "spotify:track:t1", DurationMS: 180000}},
		{Position: 1, Track: spotify.Track{Name: "Stay the Night", URI: "spotify:track:7zJnmSjZKjntHmOvEokGb3", DurationMS: 264893}},
	}

	// Simulate real-time event from librespot
	next, _ := m.Update(trackEventMsg{
		event: player.TrackEvent{
			URI:        "spotify:track:7zJnmSjZKjntHmOvEokGb3",
			Name:       "Stay the Night",
			DurationMS: 264893,
		},
	})
	m = next.(Model)

	if m.state.Item == nil {
		t.Fatal("expected track to be matched and set")
	}
	if m.state.Item.URI != "spotify:track:7zJnmSjZKjntHmOvEokGb3" {
		t.Fatalf("expected URI 'spotify:track:7zJnmSjZKjntHmOvEokGb3', got %q", m.state.Item.URI)
	}
	if !m.state.IsPlaying {
		t.Fatal("expected state.IsPlaying to be true")
	}
	if m.state.Item.DurationMS != 264893 {
		t.Fatalf("expected duration 264893, got %d", m.state.Item.DurationMS)
	}
}

func TestTopHeaderDoesNotContainSpotifyGo(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) == 0 {
		t.Fatal("expected view to have content")
	}
	headerLine := lines[0]
	if strings.Contains(headerLine, "SPOTIFYGO") {
		t.Fatalf("top header must not contain 'SPOTIFYGO', got:\n%s", headerLine)
	}
}

func TestNowPlayingCardShowsAlbumAndYearWithoutExplicitBadge(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			Name: "Starboy",
			Artists: []spotify.Artist{
				{Name: "The Weeknd"},
				{Name: "Daft Punk"},
			},
			Album: spotify.Album{
				Name:        "Starboy",
				ReleaseDate: "2016-11-25",
			},
			DurationMS: 230000,
		},
	}

	view := m.View()
	if !strings.Contains(view.Content, "Starboy") {
		t.Fatal("expected song title 'Starboy' in view")
	}
	if !strings.Contains(view.Content, "The Weeknd") {
		t.Fatal("expected artist 'The Weeknd' in view")
	}
	if !strings.Contains(view.Content, "💿 Starboy · 2016") {
		t.Fatalf("expected '💿 Starboy · 2016' in view, got content:\n%s", view.Content)
	}
	if strings.Contains(view.Content, "[E]") {
		t.Fatal("view MUST NOT contain explicit badge '[E]' per user instructions")
	}
}

func TestNowPlayingCardWithCoverData(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	m.coverData = &CoverData{
		Lines: []string{
			"1234567890",
			"abcdefghij",
			"klmnopqrst",
			"uvwxyz1234",
			"567890abcd",
		},
		DominantHex: "#ff007f",
	}
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			Name:       "Test Track",
			DurationMS: 180000,
		},
	}

	view := m.View()
	for _, l := range m.coverData.Lines {
		if !strings.Contains(view.Content, l) {
			t.Fatalf("expected view to contain cover line %q", l)
		}
	}
}

func TestBackgroundCommandAndModes(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.bgMode = "default"

	// 1. Typing /background without args opens viewBgPicker modal
	m = pressKey(m, "/")
	for _, c := range "background" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.currentView != viewBgPicker {
		t.Fatalf("expected viewBgPicker, got %d", m.currentView)
	}

	// 2. Navigating modal down: option 0 (default) -> option 1 (flow)
	m = pressKey(m, "down")
	if m.bgPickerSelected != 1 || m.bgMode != "flow" {
		t.Fatalf("expected option 1 (flow), got pick=%d, mode=%q", m.bgPickerSelected, m.bgMode)
	}

	// 3. Confirming with enter saves flow and returns to previous view
	m = pressKey(m, "enter")
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after confirm, got %d", m.currentView)
	}
	if m.bgMode != "flow" {
		t.Fatalf("expected bgMode 'flow', got %q", m.bgMode)
	}

	// 4. Direct command /background dark
	m = pressKey(m, "/")
	for _, c := range "background dark" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.bgMode != "dark" {
		t.Fatalf("expected bgMode 'dark', got %q", m.bgMode)
	}

	// 5. Direct command /bg default
	m = pressKey(m, "/")
	for _, c := range "bg default" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.bgMode != "default" {
		t.Fatalf("expected bgMode 'default', got %q", m.bgMode)
	}
}

func TestEcoPowerZeroIdle(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	if !m.isFocused {
		t.Fatal("expected isFocused to be true by default")
	}

	// 1. Terminal loses focus (User switches to VS Code / browser / agent execution)
	res, cmd := m.Update(tea.BlurMsg{})
	m = res.(Model)
	if m.isFocused {
		t.Fatal("expected isFocused to be false after BlurMsg")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd on BlurMsg to stop scheduling")
	}

	// 2. Spinner tick while unfocused must return nil (0% CPU, tick loop frozen)
	res, tickCmd := m.Update(spinner.TickMsg{})
	m = res.(Model)
	if tickCmd != nil {
		t.Fatal("expected tickCmd to be nil while unfocused to guarantee 0% CPU")
	}

	// 3. Intro tick while unfocused must also return nil
	m.EnableIntro()
	res, introCmd := m.Update(introTickMsg{})
	if introCmd != nil {
		t.Fatal("expected introCmd to be nil while unfocused")
	}

	// 4. Terminal regains focus (User switches back to SpotifyGo)
	res, focusCmd := m.Update(tea.FocusMsg{})
	m = res.(Model)
	if !m.isFocused {
		t.Fatal("expected isFocused to be true after FocusMsg")
	}
	if focusCmd == nil {
		t.Fatal("expected non-nil focusCmd to resume animations immediately")
	}

	// 5. Toast trigger and expiration
	m.setView(viewPlaylists)
	m.triggerToast("🔊 Volumen 75%", 3)
	if m.toastMessage != "🔊 Volumen 75%" || m.toastTimer != 3 {
		t.Fatalf("expected active toast message, got %q, timer=%d", m.toastMessage, m.toastTimer)
	}
	view := m.View()
	if !strings.Contains(view.Content, "🔊 Volumen 75%") {
		t.Fatal("expected toast message visible in top header view")
	}

	// Decrement toast
	for i := 0; i < 3; i++ {
		res, _ := m.Update(spinner.TickMsg{})
		m = res.(Model)
	}
	if m.toastMessage != "" || m.toastTimer != 0 {
		t.Fatalf("expected toast message expired, got %q, timer=%d", m.toastMessage, m.toastTimer)
	}
}

func TestCompactCardWhenTerminalHeightIsSmall(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 80, 18 // Compact height < 22
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			Name: "Compact Song",
			Album: spotify.Album{
				Name:        "Compact Album",
				ReleaseDate: "2023",
			},
			DurationMS: 120000,
		},
	}

	view := m.View()
	if !strings.Contains(view.Content, "Compact Song") {
		t.Fatal("expected 'Compact Song' in compact view")
	}
	if !strings.Contains(view.Content, "💿 Compact Album · 2023") {
		t.Fatal("expected album info in compact view")
	}
}

func TestIntroAnimationAndSkip(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.EnableIntro()

	if !m.introActive || m.currentView != viewIntro {
		t.Fatalf("expected introActive true and viewIntro, got active=%v, view=%d", m.introActive, m.currentView)
	}

	// 1. Initial Intro View content check (pure Zen: logo only, no secondary text)
	view := m.View()
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "███████╗") {
		t.Fatal("expected block ASCII logo in intro view")
	}
	if strings.Contains(stripped, "S P O T I F Y   F O R   T E R M I N A L") {
		t.Fatal("expected no secondary tagline in pure Zen intro")
	}
	if strings.Contains(stripped, "Pulsa cualquier tecla") {
		t.Fatal("expected no hint text in pure Zen intro")
	}

	// 2. Advancing frames via introTickMsg
	for i := 0; i < 10; i++ {
		res, _ := m.Update(introTickMsg{})
		m = res.(Model)
	}
	if m.introFrame != 10 {
		t.Fatalf("expected introFrame 10, got %d", m.introFrame)
	}
	if m.currentView != viewIntro {
		t.Fatalf("expected still viewIntro at frame 10, got %d", m.currentView)
	}

	// 3. Advancing to completion (frame > 165)
	for i := 0; i < 160; i++ {
		res, _ := m.Update(introTickMsg{})
		m = res.(Model)
	}
	if m.introActive || m.currentView != viewPlaylists {
		t.Fatalf("expected introActive false and viewPlaylists after frame expiry, got active=%v view=%d", m.introActive, m.currentView)
	}

	// 4. Instant skip on keypress
	m2 := New(nil, "PC", nil, nil)
	m2.width, m2.height = 100, 24
	m2.EnableIntro()
	m2 = pressKey(m2, " ") // Any key press skips
	if m2.introActive || m2.currentView != viewPlaylists {
		t.Fatalf("expected instant skip on keypress to viewPlaylists, got active=%v view=%d", m2.introActive, m2.currentView)
	}

	// 5. Test narrow terminal fallback (< 80 columns)
	mNarrow := New(nil, "PC", nil, nil)
	mNarrow.width, mNarrow.height = 60, 20
	mNarrow.EnableIntro()
	narrowView := mNarrow.View()
	strippedNarrow := ansi.Strip(narrowView.Content)
	if !strings.Contains(strippedNarrow, "___ ___") {
		t.Fatal("expected compact ASCII logo in narrow terminal intro")
	}
}

func TestCommandPaletteNavigationAndSearch(t *testing.T) {
	// 1. Verify registered commands contains search and others
	cmds := FilterCommands("")
	if len(cmds) < 10 {
		t.Fatalf("expected at least 10 registered commands, got %d", len(cmds))
	}
	hasSearch := false
	for _, c := range cmds {
		if c.Name == "search" {
			hasSearch = true
			break
		}
	}
	if !hasSearch {
		t.Fatal("expected search command in FilterCommands")
	}

	// 2. Open command palette with '/'
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m = pressKey(m, "/")
	if !m.commandActive {
		t.Fatal("expected commandActive to be true after '/'")
	}

	// 3. Arrow down navigation can traverse all commands past index 3
	for i := 0; i < len(m.commandMatches)-1; i++ {
		m = pressKey(m, "down")
	}
	if m.commandSelect != len(m.commandMatches)-1 {
		t.Fatalf("expected commandSelect %d, got %d", len(m.commandMatches)-1, m.commandSelect)
	}

	// 4. Verify View() contains properly closed box and scroll indicator
	view := m.View()
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "COMMANDS") {
		t.Fatal("expected COMMANDS header in command palette view")
	}
	if !strings.Contains(stripped, "│ /") {
		t.Fatal("expected search prompt in command palette view")
	}

	// 5. Test executing /search without args opens search view
	m = pressKey(m, "esc")
	m = pressKey(m, "/")
	for _, c := range "search" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")
	if m.currentView != viewSearch {
		t.Fatalf("expected viewSearch after /search, got %d", m.currentView)
	}

	// 6. Test executing /playlists opens playlists view
	m = pressKey(m, "esc")
	m = pressKey(m, "/")
	for _, c := range "playlists" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after /playlists, got %d", m.currentView)
	}
}

func TestPlayerCardInlineIconAndNoReproduciendo(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 26
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			Name:       "Test Track",
			DurationMS: 180000,
			Album:      spotify.Album{Name: "Test Album"},
		},
	}

	// 1. When playing, the card should contain the play icon inline
	viewPlaying := m.View()
	strippedPlaying := ansi.Strip(viewPlaying.Content)
	if !strings.Contains(strippedPlaying, "▶") {
		t.Fatal("expected play icon ▶ in player card")
	}
	// The word "Reproduciendo" should NOT appear in the player card
	if strings.Contains(strippedPlaying, "Reproduciendo") {
		t.Fatal("did NOT expect 'Reproduciendo' text in player card")
	}

	// 2. When paused, the card should contain the pause icon inline
	m.state.IsPlaying = false
	viewPaused := m.View()
	strippedPaused := ansi.Strip(viewPaused.Content)
	if !strings.Contains(strippedPaused, "⏸") {
		t.Fatal("expected pause icon ⏸ in player card")
	}
	if strings.Contains(strippedPaused, "En pausa") {
		t.Fatal("did NOT expect 'En pausa' text inside the player card")
	}
}

func TestTransportToastsSpacing(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			Name:       "Test Track",
			DurationMS: 180000,
		},
	}

	// 1. Space to pause produces generous spacing in toast
	m = pressKey(m, " ")
	if !strings.Contains(m.toastMessage, "⏸   En pausa") {
		t.Fatalf("expected '⏸   En pausa' in toast, got %q", m.toastMessage)
	}

	// 2. Space to resume produces generous spacing in toast
	m.transportBusy = false
	m.lastTransport = time.Time{}
	m = pressKey(m, " ")
	if !strings.Contains(m.toastMessage, "▶   Reproduciendo") {
		t.Fatalf("expected '▶   Reproduciendo' in toast, got %q", m.toastMessage)
	}

	// 3. Next song toast
	m.transportBusy = false
	m.lastTransport = time.Time{}
	m = pressKey(m, "right")
	if !strings.Contains(m.toastMessage, "⏭   Siguiente canción") {
		t.Fatalf("expected '⏭   Siguiente canción' in toast, got %q", m.toastMessage)
	}

	// 4. Prev song toast
	m.transportBusy = false
	m.lastTransport = time.Time{}
	m = pressKey(m, "left")
	if !strings.Contains(m.toastMessage, "⏮   Canción anterior") {
		t.Fatalf("expected '⏮   Canción anterior' in toast, got %q", m.toastMessage)
	}
}

func TestBigCoverArtViewAndToggle(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 32
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 60000,
		Item: &spotify.Track{
			Name:       "Fire & Desire",
			DurationMS: 238000,
			Album:      spotify.Album{Name: "Views", ReleaseDate: "2016"},
			Artists:    []spotify.Artist{{Name: "Drake"}},
		},
	}
	bigLines := make([]string, BigCoverHeightChars)
	for i := 0; i < BigCoverHeightChars; i++ {
		bigLines[i] = strings.Repeat("▀", BigCoverWidthChars)
	}
	m.coverData = &CoverData{
		Lines:       make([]string, 5),
		BigLines:    bigLines,
		DominantHex: "#1a472a",
	}

	// 1. Toggle with 'z' key activates viewCoverArt and triggers transition
	m = pressKey(m, "z")
	if m.currentView != viewCoverArt {
		t.Fatalf("expected viewCoverArt after 'z', got %d", m.currentView)
	}
	if !m.zenTransActive {
		t.Fatalf("expected zenTransActive to be true right after entering viewCoverArt")
	}

	// Advance transition to completion (56 frames)
	for i := 0; i < 60; i++ {
		updated, _ := m.Update(spinner.TickMsg{})
		m = updated.(Model)
	}
	if m.zenTransActive {
		t.Fatalf("expected zenTransActive to be false after 60 ticks")
	}

	// 2. View() in viewCoverArt contains the big cover art, track, and artist subtitle
	view := m.View()
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "Fire & Desire") {
		t.Fatal("expected track name 'Fire & Desire' in Big Cover Art view")
	}
	if !strings.Contains(stripped, "Views") {
		t.Fatal("expected album name 'Views' in Big Cover Art view")
	}
	if !strings.Contains(stripped, "Drake") {
		t.Fatal("expected artist 'Drake' in Big Cover Art view")
	}
	if !strings.Contains(stripped, "━") {
		t.Fatal("expected burning fuse progress bar in Big Cover Art view")
	}

	// Playlists and header indicators should NOT be present (clean zen sanctuary!)
	if strings.Contains(stripped, "TUS PLAYLISTS") {
		t.Fatal("did NOT expect 'TUS PLAYLISTS' when Big Cover Art mode is active")
	}
	if strings.Contains(stripped, "HQ 320k") {
		t.Fatal("did NOT expect top indicators 'HQ 320k' in clean Zen mode")
	}
	if strings.Contains(stripped, "Reproduciendo") || strings.Contains(stripped, "En pausa") {
		t.Fatal("did NOT expect text status in clean Zen mode")
	}

	// 3. Pressing 'z' again returns to viewPlaylists
	m = pressKey(m, "z")
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after second 'z', got %d", m.currentView)
	}

	// 4. Command /art toggles into viewCoverArt
	m = pressKey(m, "/")
	for _, c := range "art" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")
	if m.currentView != viewCoverArt {
		t.Fatalf("expected viewCoverArt after /art command, got %d", m.currentView)
	}

	// 5. Esc returns to viewPlaylists
	m = pressKey(m, "esc")
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after Esc, got %d", m.currentView)
	}
}

func TestTrackChangeClearsOldCoverAndFetchesMetadata(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.currentCoverURL = "https://i.scdn.co/image/old_cover"
	m.coverData = &CoverData{DominantHex: "#112233"}
	m.bigCoverCache = map[string][]string{"test": {"line1"}}
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			Name: "Old Song",
			URI:  "spotify:track:old_uri",
		},
	}

	// Librespot sends track event for new song
	next, _ := m.Update(trackEventMsg{
		event: player.TrackEvent{
			Name:       "New Song",
			URI:        "spotify:track:new_uri",
			DurationMS: 210000,
		},
	})
	m = next.(Model)

	// Old cover art and cache MUST be invalidated immediately
	if m.coverData != nil {
		t.Fatal("expected coverData to be reset to nil on track change")
	}
	if m.currentCoverURL != "" {
		t.Fatalf("expected currentCoverURL to be reset, got %q", m.currentCoverURL)
	}
	if len(m.bigCoverCache) != 0 {
		t.Fatal("expected bigCoverCache to be cleared on track change")
	}
	if m.state.ProgressMS != 0 {
		t.Fatalf("expected progress to reset to 0, got %d", m.state.ProgressMS)
	}
	if !m.state.IsPlaying {
		t.Fatal("expected state.IsPlaying to remain true")
	}

	// Now API returns rich track metadata (artists, album, cover images)
	newTrack := spotify.Track{
		Name:       "New Song",
		URI:        "spotify:track:new_uri",
		DurationMS: 210000,
		Artists:    []spotify.Artist{{Name: "LANY"}},
		Album: spotify.Album{
			Name: "Malibu Nights",
			Images: []spotify.Image{
				{URL: "https://i.scdn.co/image/new_cover_300", Width: 300, Height: 300},
			},
		},
	}

	next2, cmd := m.Update(trackFetchedMsg{
		track: newTrack,
		uri:   "spotify:track:new_uri",
	})
	m = next2.(Model)

	if m.state.Item.Album.Name != "Malibu Nights" {
		t.Fatalf("expected album name 'Malibu Nights', got %q", m.state.Item.Album.Name)
	}
	if m.state.Item.CoverURL() != "https://i.scdn.co/image/new_cover_300" {
		t.Fatalf("expected new cover URL, got %q", m.state.Item.CoverURL())
	}
	if cmd == nil {
		t.Fatal("expected cmd to fetch the new cover")
	}
}

func TestProgressAdvancesAutomaticallyDuringTicksWithoutPause(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	m.isFocused = true
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 0,
		Item: &spotify.Track{
			Name:       "Continuous Track",
			URI:        "spotify:track:cont",
			DurationMS: 200000,
		},
	}
	// Simulated 5 seconds elapsed on system clock
	m.lastSync = time.Now().Add(-5 * time.Second)

	// View rendering calculates real-time progress using lastSync
	view := m.View()
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "00:05") {
		t.Fatalf("expected real-time progress to show 00:05, got:\n%s", stripped)
	}

	// A spinner tick while playing MUST reschedule the next tick (cmd != nil)
	next, cmd := m.Update(m.spinner.Tick())
	_ = next.(Model)
	if cmd == nil {
		t.Fatal("expected spinner.TickMsg to return next tick command while playing")
	}
}

func TestTrackEventInstantMetadataFromCache(t *testing.T) {
	client := spotify.NewClient(nil)
	client.CacheTrack(spotify.Track{
		URI:     "spotify:track:instant1",
		Name:    "Instant Song",
		Artists: []spotify.Artist{{Name: "Instant Artist"}},
		Album: spotify.Album{
			Name:   "Instant Album",
			Images: []spotify.Image{{URL: "https://example.com/cover.jpg", Width: 300, Height: 300}},
		},
		DurationMS: 180000,
	})

	m := New(client, "PC", nil, nil)
	m.width, m.height = 100, 30

	// Send trackEventMsg from librespot
	next, _ := m.Update(trackEventMsg{
		event: player.TrackEvent{
			URI:  "spotify:track:instant1",
			Name: "Instant Song",
		},
	})
	m2 := next.(Model)

	// Item must be immediately populated with complete metadata from cache (0 ms)
	if m2.state.Item == nil {
		t.Fatal("expected state.Item to be populated from cache")
	}
	if len(m2.state.Item.Artists) == 0 || m2.state.Item.Artists[0].Name != "Instant Artist" {
		t.Fatalf("expected artist 'Instant Artist', got: %+v", m2.state.Item.Artists)
	}
	if m2.state.Item.Album.Name != "Instant Album" {
		t.Fatalf("expected album 'Instant Album', got: %s", m2.state.Item.Album.Name)
	}
	if m2.currentCoverURL != "https://example.com/cover.jpg" {
		t.Fatalf("expected currentCoverURL to be set immediately, got: %s", m2.currentCoverURL)
	}
}

func TestDebouncedFetchState(t *testing.T) {
	m := New(nil, "PC", nil, nil)

	// First fetchState should return a command
	cmd1 := m.fetchState()
	if cmd1 == nil {
		t.Fatal("expected first fetchState to return a command")
	}

	// Immediate second fetchState must return nil (debounced)
	cmd2 := m.fetchState()
	if cmd2 != nil {
		t.Fatal("expected second rapid fetchState to return nil due to debouncing")
	}
}

func TestGaplessPrefetchDoesNotSwitchTrackPrematurely(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 160000,
		Item: &spotify.Track{
			URI:        "spotify:track:current_playing",
			Name:       "Current Song",
			DurationMS: 200000, // 40 seconds remaining
		},
	}
	m.lastTransport = time.Time{} // No transport action

	// Simulate librespot pre-fetching the next track 40 seconds before the end
	nextMsg := trackEventMsg{
		event: player.TrackEvent{
			URI:  "spotify:track:prefetched_next",
			Name: "Next Song",
		},
	}

	nextModel, _ := m.Update(nextMsg)
	m2 := nextModel.(Model)

	// Track MUST NOT have switched yet!
	if m2.state.Item.URI != "spotify:track:current_playing" {
		t.Fatalf("expected track to remain 'current_playing', but got: %s", m2.state.Item.URI)
	}
	if m2.prefetchedTrack == nil || m2.prefetchedTrack.URI != "spotify:track:prefetched_next" {
		t.Fatalf("expected prefetchedTrack to be stored, got: %+v", m2.prefetchedTrack)
	}

	// Now simulate song finishing (progress reached 200000ms) and tick fires
	m2.state.ProgressMS = 200000
	m2.lastSync = time.Now()
	tickModel, _ := m2.Update(spinner.TickMsg{Time: time.Now()})
	m3 := tickModel.(Model)

	// Now it MUST promote the prefetched track!
	if m3.state.Item == nil || m3.state.Item.URI != "spotify:track:prefetched_next" {
		t.Fatalf("expected promoted track 'prefetched_next', got: %+v", m3.state.Item)
	}
	if m3.prefetchedTrack != nil {
		t.Fatalf("expected prefetchedTrack to be cleared after promotion")
	}
}

func TestBoundsClamping(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.playlists = []spotify.Playlist{{ID: "1", Name: "P1"}}
	m.playlistPick = 99 // Out of range

	// Trigger enter key in playlists view
	m2 := pressKey(m, "enter")
	if m2.playlistPick >= len(m2.playlists) {
		t.Fatalf("expected playlistPick to be clamped, got %d", m2.playlistPick)
	}

	// Test entries clamp
	m2.currentView = viewTracks
	m2.entries = []spotify.PlaylistEntry{{Track: spotify.Track{Name: "T1"}}}
	m2.entryPick = 50
	m3 := pressKey(m2, "enter")
	if m3.entryPick >= len(m3.entries) {
		t.Fatalf("expected entryPick to be clamped, got %d", m3.entryPick)
	}
}

func TestHotkeysAndMute(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	vol := 70
	m.state = spotify.PlaybackState{
		Available: true,
		Device: &spotify.Device{
			ID:            "dev-1",
			Name:          "Speaker",
			VolumePercent: &vol,
		},
	}

	// 1. Press 't' to open theme picker
	m2 := pressKey(m, "t")
	if m2.currentView != viewThemePicker {
		t.Fatalf("expected viewThemePicker, got %d", m2.currentView)
	}
	m2 = pressKey(m2, "esc")

	// 2. Press '?' to open command palette
	m3 := pressKey(m2, "?")
	if !m3.commandActive {
		t.Fatal("expected commandActive true on '?'")
	}
	m3 = pressKey(m3, "esc")

	// 3. Press 'm' to mute
	mMute := pressKey(m3, "m")
	if mMute.volumeOverride == nil || *mMute.volumeOverride != 0 {
		t.Fatalf("expected volume override 0 on mute, got %v", mMute.volumeOverride)
	}

	// 4. Press 'm' again to unmute (restore previous volume 70)
	mUnmute := pressKey(mMute, "m")
	if mUnmute.volumeOverride == nil || *mUnmute.volumeOverride != 70 {
		t.Fatalf("expected volume restored to 70, got %v", mUnmute.volumeOverride)
	}
}

func TestPinnedFooterAndLightTheme(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.theme = theme.Get("light-minimal")

	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) != 23 {
		t.Fatalf("expected 23 terminal rows (height-1), got %d", len(lines))
	}

	// Last line should have the footer shortcuts
	lastLine := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(lastLine, "Play") || !strings.Contains(lastLine, "Tema") {
		t.Fatalf("expected footer with shortcuts on last line, got: %q", lastLine)
	}

	// Verify background applied is light (F8FAFC)
	if !strings.Contains(lines[0], "\x1b[48;2;") {
		t.Fatal("expected background truecolor escape sequences in line")
	}
}

func TestAutoUpdateCheckAndPrompt(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24

	// Verify default background is flow
	if m.bgMode != "flow" {
		t.Fatalf("expected default bgMode to be 'flow', got %q", m.bgMode)
	}

	// 1. Simulate updateCheckMsg with newer release
	rel := &version.GitHubRelease{
		TagName: "v9.9.9",
		Name:    "v9.9.9",
	}
	mUpdated, _ := m.Update(updateCheckMsg{release: rel})
	m2 := mUpdated.(Model)

	if m2.currentView != viewUpdatePrompt {
		t.Fatalf("expected viewUpdatePrompt on newer release, got %d", m2.currentView)
	}

	// Verify View contains update prompt text
	view := m2.View()
	if !strings.Contains(view.Content, "ACTUALIZACIÓN DISPONIBLE") || !strings.Contains(view.Content, "v9.9.9") {
		t.Fatalf("expected update prompt content, got: %s", view.Content)
	}

	// 2. Pressing 'c' switches to viewChangelog
	mChangelog := pressKey(m2, "c")
	if mChangelog.currentView != viewChangelog {
		t.Fatalf("expected viewChangelog after pressing 'c', got %d", mChangelog.currentView)
	}

	// 3. Pressing 'esc' in changelog returns to previous view
	mBack := pressKey(mChangelog, "esc")
	if mBack.currentView != viewUpdatePrompt {
		t.Fatalf("expected return to viewUpdatePrompt, got %d", mBack.currentView)
	}

	// 4. Pressing 'esc' in update prompt dismisses to playlists
	mDismiss := pressKey(mBack, "esc")
	if mDismiss.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after dismissing update prompt, got %d", mDismiss.currentView)
	}
}

func TestChangelogModal(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24

	// Open changelog via command /changelog
	m = pressKey(m, "/")
	for _, c := range "changelog" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.currentView != viewChangelog {
		t.Fatalf("expected viewChangelog after /changelog, got %d", m.currentView)
	}

	view := m.View()
	if !strings.Contains(view.Content, "NOVEDADES Y CAMBIOS RECIENTES") {
		t.Fatalf("expected changelog header in view, got: %s", view.Content)
	}

	// Pressing esc closes changelog
	mClosed := pressKey(m, "esc")
	if mClosed.currentView != viewPlaylists {
		t.Fatalf("expected return to viewPlaylists, got %d", mClosed.currentView)
	}
}

func TestRightAlignedVisualizerAndFineProgressBar(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.state = spotify.PlaybackState{
		Available: true,
		IsPlaying: true,
		Item: &spotify.Track{
			URI:        "spotify:track:123",
			Name:       "Test Track",
			DurationMS: 200000,
			Artists:    []spotify.Artist{{Name: "Test Artist"}},
		},
	}

	view := m.View()
	// Check fine progress bar elements
	if !strings.Contains(view.Content, "●") {
		t.Fatal("expected fine progress bar pulsing knob '●' in view")
	}

	// Verify 320k hq · stereo was removed from the card
	if strings.Contains(view.Content, "320k hq · stereo") {
		t.Fatal("did not expect '320k hq · stereo' in playback card")
	}
}

func TestLikedTracksSingleHeart(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.menuTransActive = false
	m.playlists = []spotify.Playlist{
		{ID: spotify.LikedTracksID, Name: "Canciones que te gustan"},
		{ID: "p2", Name: "♥ Canciones que te gustan"},
	}

	view := m.View()
	stripped := ansi.Strip(view.Content)
	if strings.Contains(stripped, "♥ ♥") {
		t.Fatalf("expected single heart for liked tracks, but found duplicated '♥ ♥' in view:\n%s", stripped)
	}
	if !strings.Contains(stripped, "♥ Canciones que te gustan") {
		t.Fatalf("expected '♥ Canciones que te gustan' in view, got:\n%s", stripped)
	}
}

func TestZenTransitionNoPrevCoverGlitch(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24

	// Pretend there was an old previous cover
	m.zenPrevImage = image.NewRGBA(image.Rect(0, 0, 10, 10))

	// Pressing 'z' to enter Zen mode
	mZen := pressKey(m, "z")
	if mZen.currentView != viewCoverArt {
		t.Fatalf("expected viewCoverArt, got %d", mZen.currentView)
	}
	// zenPrevImage MUST be nil so it doesn't flash the old song's cover
	if mZen.zenPrevImage != nil {
		t.Fatal("expected zenPrevImage to be nil upon entering Zen mode")
	}
	if !mZen.zenTransActive {
		t.Fatal("expected zenTransActive to be true")
	}
	if ZenTransitionMaxFrames != 50 {
		t.Fatalf("expected ZenTransitionMaxFrames to be 50 (6.0s at 120ms), got %d", ZenTransitionMaxFrames)
	}
}

func TestStaggeredPlaylistAndTrackSweep(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.setView(viewTracks)
	if !m.menuTransActive {
		t.Fatal("expected menuTransActive to be true on setView")
	}

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#1db954"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))

	rowContent := "› • Mi Playlist Favorita (50)"

	// Frame 0, Row 3: row 3 should not be revealed yet (spaces)
	sweptEarly := renderSweptRow(rowContent, 3, 0, true, accent, muted)
	if strings.Contains(sweptEarly, "Playlist") {
		t.Fatalf("row 3 should not be revealed at frame 0, got %q", sweptEarly)
	}

	// Frame 3, Row 3: row 3 is scanning with spark
	sweptScanning := renderSweptRow(rowContent, 3, 3, true, accent, muted)
	if !strings.Contains(sweptScanning, "\x1b[") {
		t.Fatalf("row 3 at active scan should contain ANSI spark, got %q", sweptScanning)
	}

	// Frame 10, Row 3: transition ended, returns full content
	sweptDone := renderSweptRow(rowContent, 3, 10, true, accent, muted)
	if sweptDone != rowContent {
		t.Fatalf("expected full content at frame 10, got %q", sweptDone)
	}
}
