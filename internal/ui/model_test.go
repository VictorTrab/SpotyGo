package ui

import (
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
	if !m.transportBusy || m.desiredPlaying == nil || *m.desiredPlaying {
		t.Fatal("first space must request a pause")
	}
	next, _ := m.Update(actionMsg{name: "pausar"})
	m = next.(Model)
	if m.state.IsPlaying {
		t.Fatal("confirmed pause action must set isPlaying to false")
	}
	old, _ := m.Update(stateMsg{state: spotify.PlaybackState{Available: true, IsPlaying: true}, epoch: 0})
	m = old.(Model)
	if m.state.IsPlaying {
		t.Fatal("old playback response replaced the pending pause")
	}
	m = pressSpace(m)
	if m.state.IsPlaying {
		t.Fatal("duplicate space must not resume playback while busy")
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

	// Footer-only 'q' must not remain as a hidden slash command.
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil {
		t.Fatal("unknown slash command should not execute a hidden action")
	}
	m = next.(Model)
	if !m.statusError || !strings.Contains(m.status, "no existe") {
		t.Fatalf("expected q to be reported as an unavailable slash command, got %q", m.status)
	}
	if matches := FilterCommands("th"); len(matches) != 0 {
		t.Fatalf("description substring must not expose unrelated commands: %+v", matches)
	}

	// Theme selection is available from the footer, not as a redundant slash command.
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	m = next.(Model)
	if m.currentView != viewPlaylists || !m.statusError {
		t.Fatalf("expected unavailable /theme to leave the current view unchanged, got view=%v status=%q", m.currentView, m.status)
	}

	// Esc leaves the normal view unchanged after the unknown command.
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
	if cmd == nil || !m.transportBusy || m.status != statusNext {
		t.Fatal("n must trigger next track")
	}

	m.transportBusy = false
	// Press 'b' -> previous track
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'b', Text: "b"}))
	m = next.(Model)
	if cmd == nil || !m.transportBusy || m.status != statusPrev {
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
	if cmd == nil || !m.transportBusy || m.status != statusNext {
		t.Fatal("right arrow must trigger next track")
	}

	m.transportBusy = false
	// Press 'left' -> previous track
	next, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft, Text: "left"}))
	m = next.(Model)
	if cmd == nil || !m.transportBusy || m.status != statusPrev {
		t.Fatal("left arrow must trigger previous track")
	}
}

func TestDefaultBitrateFixedTo320(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	if m.bitrate != "320" {
		t.Fatalf("expected default bitrate 320, got %s", m.bitrate)
	}

	// Verify quality is removed from command palette
	cmds := FilterCommands("quality")
	if len(cmds) > 0 {
		t.Fatalf("expected quality command to be removed, found %d matches", len(cmds))
	}
}

func TestSimplifiedHeaderWithoutQualityBadge(t *testing.T) {
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
	if strings.Contains(view.Content, "HQ 320k") || strings.Contains(view.Content, "MQ 160k") {
		t.Fatalf("header must NOT contain quality badge, got:\n%s", view.Content)
	}

	// Change to mobile device
	m.state.Device = &spotify.Device{ID: "phone", Name: "iPhone", Type: "Smartphone", VolumePercent: &vol}
	view = m.View()
	if !strings.Contains(view.Content, "📱 Móvil") {
		t.Fatalf("header must contain '📱 Móvil' for phone, got:\n%s", view.Content)
	}
}

func TestPollDelayAdaptsToPlaybackState(t *testing.T) {
	phone := &spotify.Device{ID: "phone", Name: "iPhone"}
	local := &spotify.Device{ID: "pc", Name: "PC"}

	playing := func(device *spotify.Device, durationMS, progressMS int) Model {
		m := New(nil, "PC", nil, nil)
		m.state = spotify.PlaybackState{
			Available:  true,
			IsPlaying:  true,
			ProgressMS: progressMS,
			Item:       &spotify.Track{URI: "spotify:track:x", DurationMS: durationMS},
			Device:     device,
		}
		m.lastSync = time.Now()
		return m
	}

	if d := playing(phone, 200000, 0).pollDelay(); d != remotePollCeiling {
		t.Fatalf("mid-track remote playback must fall back to the remote ceiling, got %s", d)
	}
	if d := playing(phone, 2000, 0).pollDelay(); d != 4*time.Second {
		t.Fatalf("near the end of a remote track the poll must land just past it, got %s", d)
	}
	if d := playing(phone, 1000, 1000).pollDelay(); d != minPollDelay {
		t.Fatalf("a finished remote track must be re-checked at the floor, got %s", d)
	}
	if d := playing(local, 200000, 0).pollDelay(); d != localPollCeiling {
		t.Fatalf("local playback keeps librespot events, so it may poll slower, got %s", d)
	}
	if d := playing(local, 1000, 1000).pollDelay(); d != 6*time.Second {
		t.Fatalf("local boundary polls must stay inside the local floor, got %s", d)
	}

	paused := playing(phone, 200000, 0)
	paused.state.IsPlaying = false
	if d := paused.pollDelay(); d != pausedDelay {
		t.Fatalf("paused playback should poll lazily, got %s", d)
	}

	unknown := New(nil, "PC", nil, nil)
	if d := unknown.pollDelay(); d != unknownDelay {
		t.Fatalf("unknown state should retry quickly, got %s", d)
	}

	blurred := playing(phone, 2000, 0)
	blurred.isFocused = false
	if d := blurred.pollDelay(); d != unfocusedDelay {
		t.Fatalf("an unfocused window must throttle polling, got %s", d)
	}

	// Regression guard: the old scheduler refreshed only every 4th 25s tick (100s)
	// whenever the local device was the active one.
	if d := playing(local, 200000, 0).pollDelay(); d > 25*time.Second {
		t.Fatalf("local playback must never wait anywhere near the old 100s gap, got %s", d)
	}
}

func TestPollAtTrackBoundaryProbesAtMostTwice(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 1000,
		Item:       &spotify.Track{URI: "spotify:track:x", DurationMS: 1000},
		Device:     &spotify.Device{ID: "phone", Name: "iPhone"},
	}
	m.lastSync = time.Now()

	if !m.atTrackBoundary() {
		t.Fatal("a track at its end must be reported as a boundary")
	}
	for want := 1; want <= maxBoundaryProbes; want++ {
		next, cmd := m.Update(pollMsg{})
		m = next.(Model)
		if cmd == nil {
			t.Fatal("boundary probe must schedule a refresh")
		}
		if m.boundaryProbes != want {
			t.Fatalf("expected %d boundary probes, got %d", want, m.boundaryProbes)
		}
	}
	next, _ := m.Update(pollMsg{})
	m = next.(Model)
	if m.boundaryProbes != maxBoundaryProbes {
		t.Fatalf("boundary probes must stay capped at %d, got %d", maxBoundaryProbes, m.boundaryProbes)
	}
}

func TestRemoteTrackChangeResetsBoundaryProbes(t *testing.T) {
	phone := &spotify.Device{ID: "phone", Name: "iPhone"}
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 1000,
		Item:       &spotify.Track{URI: "spotify:track:old", DurationMS: 1000},
		Device:     phone,
	}
	m.boundaryProbes = maxBoundaryProbes

	next, _ := m.Update(stateMsg{
		epoch: m.stateEpoch,
		state: spotify.PlaybackState{
			Available: true,
			IsPlaying: true,
			Item:      &spotify.Track{URI: "spotify:track:new", DurationMS: 180000},
			Device:    phone,
		},
	})
	m = next.(Model)
	if m.state.Item == nil || m.state.Item.URI != "spotify:track:new" {
		t.Fatal("a remote track change must be adopted")
	}
	if m.boundaryProbes != 0 {
		t.Fatalf("a new track must reset boundary probing, got %d", m.boundaryProbes)
	}
}

func TestKeyPressRefreshesStalePlaybackState(t *testing.T) {
	client := spotify.NewClient(nil)

	stale := New(client, "PC", nil, nil)
	stale.lastStateFetch = time.Now().Add(-4 * time.Second)
	next, cmd := stale.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	stale = next.(Model)
	if cmd == nil {
		t.Fatal("a keystroke on a stale state must request a refresh")
	}
	if !stale.stateFetching {
		t.Fatal("the refresh must be marked in flight so it is not duplicated")
	}

	fresh := New(client, "PC", nil, nil)
	fresh.lastStateFetch = time.Now()
	next, _ = fresh.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	fresh = next.(Model)
	if fresh.stateFetching {
		t.Fatal("a fresh state must not trigger an extra request on every keystroke")
	}

	quit := New(client, "PC", nil, nil)
	quit.lastStateFetch = time.Now().Add(-4 * time.Second)
	next, _ = quit.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	quit = next.(Model)
	if quit.stateFetching {
		t.Fatal("quitting must not fire a playback refresh")
	}
}

func TestLocalActivePollsAsSafetyNet(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.localReady = true
	m.state = spotify.PlaybackState{
		Available: true,
		Device:    &spotify.Device{ID: "pc", Name: "PC", Type: "Computer"},
	}

	if !m.isLocalActive() {
		t.Fatal("isLocalActive must return true when active device matches localName")
	}

	// The scheduler must still produce a tick, and one that stays close enough to
	// recover from a missed librespot event.
	next, cmd := m.Update(pollMsg{})
	_ = next.(Model)
	if cmd == nil {
		t.Fatal("expected poll tick cmd")
	}
	if d := m.pollDelay(); d > localPollCeiling {
		t.Fatalf("local polling must remain a safety net, got %s", d)
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

	// 1. User presses Space to Pause: UI keeps playing while waiting for Spotify API
	m = pressSpace(m)
	if !m.state.IsPlaying {
		t.Fatal("player must remain playing until Spotify API confirms pause")
	}
	if !m.transportBusy {
		t.Fatal("transportBusy must be set while waiting for pause response")
	}
	if m.desiredPlaying == nil || *m.desiredPlaying != false {
		t.Fatal("desiredPlaying must be set to false")
	}
	if m.status != "" {
		t.Fatalf("status should be clean on pause, got: %q", m.status)
	}
	// 2. Action completes successfully: Spotify API confirmed pause
	next, _ := m.Update(actionMsg{name: "pausar"})
	m = next.(Model)
	if m.transportBusy {
		t.Fatal("transportBusy must be cleared once action completes")
	}
	if m.state.IsPlaying {
		t.Fatal("state must transition to paused once Spotify API responds")
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

	// 4. User resumes after 250ms: UI waits for Spotify API to avoid desynchronization/rewind
	m.lastTransport = time.Now().Add(-250 * time.Millisecond)
	m = pressSpace(m)
	if m.state.IsPlaying {
		t.Fatal("player must remain waiting until Spotify API responds")
	}
	if !m.transportBusy {
		t.Fatal("transportBusy must be true while waiting for resume response")
	}
	if m.desiredPlaying == nil || *m.desiredPlaying != true {
		t.Fatal("desiredPlaying must be set to true")
	}

	// 5. Resume action completes (API responded!) -> starts playing and syncs clock
	next, _ = m.Update(actionMsg{name: "reproducir"})
	m = next.(Model)
	if m.transportBusy {
		t.Fatal("transportBusy must be cleared after resume action")
	}
	if !m.state.IsPlaying {
		t.Fatal("state must transition to playing once Spotify API responds")
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

func TestLocalDevicePauseRemainsPausedAndDoesNotCountSeconds(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.state = spotify.PlaybackState{
		Available:  true,
		IsPlaying:  true,
		ProgressMS: 46000,
		Item: &spotify.Track{
			Name:       "Lovely",
			URI:        "spotify:track:lovely123",
			DurationMS: 291000,
		},
		Device: &spotify.Device{
			Name: "PC",
			Type: "Computer",
			ID:   "pc-dev",
		},
	}
	m.lastSync = time.Now()

	if !m.isLocalActive() {
		t.Fatal("expected local device to be active")
	}

	// 1. User pauses via Space: waits for API to avoid desync
	m = pressSpace(m)
	if !m.state.IsPlaying {
		t.Fatal("player must remain playing until Spotify API responds")
	}
	if !m.transportBusy {
		t.Fatal("transportBusy must be set while waiting for pause")
	}

	// 2. Pause action completes
	next, _ := m.Update(actionMsg{name: "pausar"})
	m = next.(Model)
	if m.state.IsPlaying {
		t.Fatal("player must pause once action completes")
	}

	// 3. Spotify state arrives confirming playback is paused on local device
	next, _ = m.Update(stateMsg{
		epoch: m.stateEpoch,
		state: spotify.PlaybackState{
			Available:  true,
			IsPlaying:  false, // Spotify confirmed paused!
			ProgressMS: 46000,
			Item: &spotify.Track{
				Name:       "Lovely",
				URI:        "spotify:track:lovely123",
				DurationMS: 291000,
			},
			Device: &spotify.Device{
				Name: "PC",
				Type: "Computer",
				ID:   "pc-dev",
			},
		},
	})
	m = next.(Model)

	if m.state.IsPlaying {
		t.Fatal("state must remain paused when Spotify confirmed paused on local device")
	}

	// 4. Progress must NOT advance even if time passes
	m.lastSync = time.Now().Add(-10 * time.Second)
	if got := m.currentPositionMS(); got != 46000 {
		t.Fatalf("expected progress to stay at 46000ms while paused, got %d", got)
	}

	// 5. View must display pause icon ⏸ and NOT play icon ▶
	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "⏸") {
		t.Fatal("view must render pause icon ⏸ when paused")
	}
	if strings.Contains(content, "▶ 00:46") {
		t.Fatal("view must NOT show play icon ▶ 00:46 while paused")
	}
}

func TestPlayPauseErrorReversion(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{Available: true, IsPlaying: true}

	m = pressSpace(m)
	if !m.transportBusy || !m.state.IsPlaying {
		t.Fatal("player should remain playing and busy until response")
	}

	// Network error on pause
	next, _ := m.Update(actionMsg{name: "pausar", err: errors.New("timeout connecting to Spotify")})
	m = next.(Model)
	if !m.state.IsPlaying {
		t.Fatal("state must remain playing on error")
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
	m.bgMode = "flow"

	// 1. Typing /background without args opens viewBgPicker modal
	m = pressKey(m, "/")
	for _, c := range "background" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.currentView != viewBgPicker {
		t.Fatalf("expected viewBgPicker, got %d", m.currentView)
	}

	// 2. Flow is the first option and the one selected on open.
	if m.bgPickerSelected != 0 || m.bgMode != "flow" {
		t.Fatalf("expected flow preselected as option 0, got pick=%d, mode=%q", m.bgPickerSelected, m.bgMode)
	}

	// 3. Navigating down lands on gradient and previews it live.
	m = pressKey(m, "down")
	if m.bgPickerSelected != 1 || m.bgMode != "gradient" {
		t.Fatalf("expected option 1 (gradient), got pick=%d, mode=%q", m.bgPickerSelected, m.bgMode)
	}

	// 4. Confirming with enter saves gradient and returns to the previous view.
	m = pressKey(m, "enter")
	if m.currentView != viewPlaylists {
		t.Fatalf("expected viewPlaylists after confirm, got %d", m.currentView)
	}
	if m.bgMode != "gradient" {
		t.Fatalf("expected bgMode 'gradient', got %q", m.bgMode)
	}
	if !strings.Contains(m.toastMessage, "gradient") {
		t.Fatalf("expected a friendly gradient toast, got %q", m.toastMessage)
	}

	// 5. Direct command /background dark
	m = pressKey(m, "/")
	for _, c := range "background dark" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.bgMode != "dark" {
		t.Fatalf("expected bgMode 'dark', got %q", m.bgMode)
	}

	// 6. Direct command /bg default (legacy alias for gradient)
	m = pressKey(m, "/")
	for _, c := range "bg default" {
		m = pressKey(m, string(c))
	}
	m = pressKey(m, "enter")

	if m.bgMode != "gradient" {
		t.Fatalf("expected the legacy 'default' alias to map to gradient, got %q", m.bgMode)
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

	// 2. A stale playback tick while unfocused must not restart a timer.
	res, tickCmd := m.Update(playbackTickMsg{epoch: m.playbackClockEpoch})
	m = res.(Model)
	if tickCmd != nil || m.playbackTickScheduled {
		t.Fatal("playback clock must remain stopped while unfocused")
	}

	// 3. Intro tick while unfocused must also return nil
	m.EnableIntro()
	res, introCmd := m.Update(introTickMsg{})
	if introCmd != nil {
		t.Fatal("expected introCmd to be nil while unfocused")
	}
	m.introActive = false
	m.setView(viewPlaylists)

	// 4. Terminal regains focus (User switches back to SpotifyGo)
	res, focusCmd := m.Update(tea.FocusMsg{})
	m = res.(Model)
	if !m.isFocused {
		t.Fatal("expected isFocused to be true after FocusMsg")
	}
	if !m.isFocused || m.playbackTickScheduled || m.zenTickScheduled {
		t.Fatal("focus must not start a decorative animation clock while idle")
	}
	_ = focusCmd // fetchState may request a remote refresh; it is not an animation timer.

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
	res, _ = m.Update(toastExpireMsg{seq: m.toastSeq})
	m = res.(Model)
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

func TestCommandPaletteOnlyContainsNonFooterCommands(t *testing.T) {
	// 1. Verify registered commands contains non-redundant commands
	cmds := FilterCommands("")
	if len(cmds) != 6 {
		t.Fatalf("expected 6 non-redundant registered commands, got %d", len(cmds))
	}
	hasBg := false
	for _, c := range cmds {
		if c.Name == "background" {
			hasBg = true
			break
		}
	}
	if !hasBg {
		t.Fatal("expected background command in FilterCommands")
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

	// Footer actions must have no hidden slash-command resolver entries.
	for _, name := range []string{"search", "play", "pause", "next", "prev", "playlists", "devices", "volume", "theme", "art", "quit"} {
		if _, ok := ResolveCommand(name); ok {
			t.Errorf("footer action /%s must not be registered in the command palette", name)
		}
	}
	for _, name := range []string{"background", "login", "update", "changelog", "version", "help"} {
		if _, ok := ResolveCommand(name); !ok {
			t.Errorf("expected /%s to remain available in the command palette", name)
		}
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

	// 1. When playing, the card should contain the play icon inline with trailing space
	viewPlaying := m.View()
	strippedPlaying := ansi.Strip(viewPlaying.Content)
	if !strings.Contains(strippedPlaying, "▶ ") {
		t.Fatal("expected play icon '▶ ' with space in player card")
	}
	// The word "Reproduciendo" should NOT appear in the player card
	if strings.Contains(strippedPlaying, "Reproduciendo") {
		t.Fatal("did NOT expect 'Reproduciendo' text in player card")
	}

	// 2. When paused, the card should contain the pause icon with double space for wide emoji compatibility
	m.state.IsPlaying = false
	viewPaused := m.View()
	strippedPaused := ansi.Strip(viewPaused.Content)
	if !strings.Contains(strippedPaused, "⏸  ") {
		t.Fatal("expected pause icon '⏸  ' with spacing for emoji in player card")
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

	// 1. Space to pause produces the friendly pause toast
	m = pressKey(m, " ")
	if m.toastMessage != toastPaused {
		t.Fatalf("expected %q in toast, got %q", toastPaused, m.toastMessage)
	}
	next, _ := m.Update(actionMsg{name: "pausar"})
	m = next.(Model)

	// 2. Space to resume produces the friendly playing toast
	m.transportBusy = false
	m.lastTransport = time.Time{}
	m = pressKey(m, " ")
	if m.toastMessage != toastPlaying {
		t.Fatalf("expected %q in toast, got %q", toastPlaying, m.toastMessage)
	}
	next, _ = m.Update(actionMsg{name: "reproducir"})
	m = next.(Model)

	// 3. Next song toast
	m.transportBusy = false
	m.lastTransport = time.Time{}
	m = pressKey(m, "right")
	if m.toastMessage != toastNext {
		t.Fatalf("expected %q in toast, got %q", toastNext, m.toastMessage)
	}

	// 4. Prev song toast
	m.transportBusy = false
	m.lastTransport = time.Time{}
	m = pressKey(m, "left")
	if m.toastMessage != toastPrev {
		t.Fatalf("expected %q in toast, got %q", toastPrev, m.toastMessage)
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

	// Advance the independent 30 FPS transition clock to completion (~1 second).
	for i := 0; i < ZenTransitionMaxFrames; i++ {
		updated, _ := m.Update(zenTransitionTickMsg{epoch: m.zenTransitionClockEpoch})
		m = updated.(Model)
	}
	if m.zenTransActive {
		t.Fatalf("expected zenTransActive to be false after %d ticks", ZenTransitionMaxFrames)
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

	// 4. The footer shortcut toggles back into Zen mode.
	m = pressKey(m, "z")
	if m.currentView != viewCoverArt {
		t.Fatalf("expected viewCoverArt after pressing z, got %d", m.currentView)
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

	// Normal playback gets a low-frequency position refresh, separate from Zen.
	if cmds := m.reconcileClocks(); len(cmds) != 1 {
		t.Fatalf("expected one playback timer, got %d", len(cmds))
	}
	_, cmd := m.Update(playbackTickMsg{epoch: m.playbackClockEpoch})
	if cmd == nil || !m.playbackTickScheduled {
		t.Fatal("expected playback clock to schedule its next one-second refresh")
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
	tickModel, _ := m2.Update(playbackTickMsg{epoch: m2.playbackClockEpoch})
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

func TestPinnedFooterAndHighContrastTheme(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 24
	m.theme = theme.Get("cyberpunk")

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

	// Verify background applied is truecolor escape sequences
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
	if ZenTransitionMaxFrames != 30 {
		t.Fatalf("expected ZenTransitionMaxFrames to be 30 (~1s at 33ms), got %d", ZenTransitionMaxFrames)
	}
}

func TestZenAnimationClockIsIndependentAndStopsOutsideZen(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.state = spotify.PlaybackState{IsPlaying: true, Item: &spotify.Track{DurationMS: 120000}}
	m = pressKey(m, "z")
	if !m.zenTickScheduled || !m.zenTransitionTickScheduled || m.playbackTickScheduled {
		t.Fatal("Zen must run independent ambient and entrance clocks, not the normal playback clock")
	}

	zenEpoch := m.zenClockEpoch
	updated, cmd := m.Update(zenTickMsg{epoch: zenEpoch})
	m = updated.(Model)
	if m.zenFrame != 1 || m.zenTransFrame != 0 || !m.zenTickScheduled || cmd == nil {
		t.Fatalf("ambient Zen tick changed the wrong clock: frame=%d transition=%d scheduled=%v", m.zenFrame, m.zenTransFrame, m.zenTickScheduled)
	}

	transitionEpoch := m.zenTransitionClockEpoch
	updated, cmd = m.Update(zenTransitionTickMsg{epoch: transitionEpoch})
	m = updated.(Model)
	if m.zenTransFrame != 1 || !m.zenTransitionTickScheduled || cmd == nil {
		t.Fatalf("entrance tick did not smoothly advance and reschedule its own clock: frame=%d scheduled=%v", m.zenTransFrame, m.zenTransitionTickScheduled)
	}

	m.setView(viewPlaylists)
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	if m.zenTickScheduled || m.zenTransitionTickScheduled || !m.playbackTickScheduled {
		t.Fatal("leaving Zen must stop both Zen clocks and resume only the playback-position clock")
	}

	frame := m.zenFrame
	transitionFrame := m.zenTransFrame
	updated, _ = m.Update(tea.BlurMsg{})
	m = updated.(Model)
	updated, _ = m.Update(zenTickMsg{epoch: zenEpoch})
	m = updated.(Model)
	updated, _ = m.Update(zenTransitionTickMsg{epoch: transitionEpoch})
	m = updated.(Model)
	if m.zenFrame != frame || m.zenTransFrame != transitionFrame || m.playbackTickScheduled {
		t.Fatal("stale Zen ticks must not animate after leaving Zen or losing focus")
	}
}

func TestNormalModeHasNoDecorativeAnimationClock(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.setView(viewTracks)
	if cmds := m.reconcileClocks(); len(cmds) != 0 {
		t.Fatal("idle normal mode must not schedule a decorative animation clock")
	}
	if m.zenTickScheduled || m.playbackTickScheduled {
		t.Fatal("normal menu must not activate Zen or playback clocks while idle")
	}
}

func TestStaticPlayerCardAndText(t *testing.T) {
	m := New(nil, "PC", nil, nil)
	m.width, m.height = 100, 30
	m.theme = theme.Theme{
		Border: "#27272a",
		Accent: "#1db954",
		Text:   "#ffffff",
	}

	// Trigger track transition
	m, _ = m.applyTrackTransition(player.TrackEvent{
		Name: "Starman",
		URI:  "spotify:track:starman",
	})

	// View should render immediately with the track name
	view := m.View()
	if !strings.Contains(view.Content, "Starman") {
		t.Fatalf("expected track name 'Starman' to be rendered immediately in view")
	}
}
