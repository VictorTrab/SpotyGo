package ui

import (
	"context"
	"fmt"
	"image"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/VictorTrab/SpotyGo/internal/logger"
	"github.com/VictorTrab/SpotyGo/internal/player"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/theme"
	"github.com/VictorTrab/SpotyGo/internal/version"
	"github.com/charmbracelet/x/ansi"
)

type stateMsg struct {
	state spotify.PlaybackState
	err   error
	epoch int
}

type devicesMsg struct {
	devices []spotify.Device
	err     error
}

type actionMsg struct {
	name  string
	seq   int
	err   error
	value int
}

type volumeDueMsg struct{ seq int }
type pollMsg struct{}
type refreshDueMsg struct{ devices bool }
type engineStoppedMsg struct{ err error }
type engineRestartMsg struct {
	err     error
	bitrate string
}
type engineEventMsg struct{}
type trackEventMsg struct {
	event player.TrackEvent
}
type trackFetchedMsg struct {
	track spotify.Track
	err   error
	uri   string
}
type loginMsg struct{ err error }

type searchMsg struct {
	tracks []spotify.Track
	err    error
	seq    int
}

type playlistsMsg struct {
	page   spotify.PlaylistPage
	err    error
	seq    int
	offset int
}

type playlistEntriesMsg struct {
	page       spotify.PlaylistEntriesPage
	err        error
	seq        int
	offset     int
	playlistID string
}

type introTickMsg struct{}
type zenTickMsg struct{ epoch int }
type zenTransitionTickMsg struct{ epoch int }
type playbackTickMsg struct{ epoch int }
type toastExpireMsg struct{ seq int }

type viewMode int

const (
	viewIntro viewMode = iota
	viewPlaylists
	viewTracks
	viewDevices
	viewSearch
	viewThemePicker
	viewBgPicker
	viewCoverArt
	viewUpdatePrompt
	viewChangelog
)

type Model struct {
	client                     *spotify.Client
	localName                  string
	engineDone                 <-chan error
	engineEvents               <-chan struct{}
	volumeEvents               <-chan int
	trackEvents                <-chan player.TrackEvent
	bitrate                    string
	bgMode                     string
	bgPickerSelected           int
	isFocused                  bool
	boundaryProbes             int
	toastMessage               string
	toastTimer                 int
	toastSeq                   int
	introActive                bool
	introFrame                 int
	introStartTime             time.Time
	currentCoverURL            string
	updateRelease              *version.GitHubRelease
	updateInProgress           bool
	updateError                string
	updateDone                 bool
	changelogScroll            int
	coverData                  *CoverData
	bigCoverCache              map[string][]string
	zenTransFrame              int
	zenTransActive             bool
	zenTransitionClockEpoch    int
	zenTransitionTickScheduled bool
	zenFrame                   int
	zenClockEpoch              int
	zenTickScheduled           bool
	playbackClockEpoch         int
	playbackTickScheduled      bool
	zenPrevTrackID             string
	zenPrevImage               image.Image
	engineRestart              func(bitrate string) error
	localReady                 bool
	transferSent               bool
	transportBusy              bool
	currentView                viewMode
	previousView               viewMode
	theme                      theme.Theme
	themeList                  []theme.Theme
	themeSelected              int
	commandActive              bool
	commandInput               string
	commandMatches             []CommandDef
	commandSelect              int
	showPlaylists              bool
	showTracks                 bool
	showDevices                bool
	searching                  bool
	searchInput                string
	lastSearchQuery            string
	searchResults              []spotify.Track
	searchSelected             int
	searchSeq                  int
	mutePreviousVol            *int
	playlists                  []spotify.Playlist
	playlistTotal              int
	playlistMore               bool
	playlistBusy               bool
	playlistSeq                int
	playlistPick               int
	activePlaylist             spotify.Playlist
	entries                    []spotify.PlaylistEntry
	entriesTotal               int
	entriesMore                bool
	entriesBusy                bool
	entriesSeq                 int
	entriesOffset              int
	entryPick                  int
	state                      spotify.PlaybackState
	devices                    []spotify.Device
	selected                   int
	status                     string
	width                      int
	height                     int
	volumeOverride             *int
	volumeSeq                  int
	volumeExpected             *int
	volumeDeviceID             string
	volumeSetAt                time.Time
	stateEpoch                 int
	lastTransport              time.Time
	lastSync                   time.Time
	statusError                bool
	pendingDeviceID            string
	pendingDeviceName          string
	pendingDeviceSince         time.Time
	pendingContext             string
	pendingSince               time.Time
	stateFetching              bool
	lastStateFetch             time.Time
	fetchingTrackURI           string
	prefetchedTrack            *player.TrackEvent
	desiredPlaying             *bool
	desiredSince               time.Time
	progress                   progress.Model
	entriesCache               map[string][]spotify.PlaylistEntry
}

// ZenTransitionMaxFrames defines the entrance transition length (~1s at 30 FPS).
const ZenTransitionMaxFrames = 30

func (m *Model) triggerZenTransition() {
	m.zenTransActive = true
	m.zenTransFrame = 0
	m.zenPrevImage = nil // Reset so it NEVER flashes the old cover when toggling Zen!
}

func (m *Model) setView(v viewMode) {
	m.previousView = m.currentView
	m.currentView = v
	m.showPlaylists = (v == viewPlaylists)
	m.showTracks = (v == viewTracks)
	m.showDevices = (v == viewDevices)
	m.searching = (v == viewSearch)
	if v == viewCoverArt {
		m.triggerZenTransition()
	}
}

func New(client *spotify.Client, localName string, engineDone <-chan error, engineEvents <-chan struct{}, volumeEvents ...<-chan int) Model {
	cfg := theme.LoadConfig()
	th := theme.Get(cfg.Theme)
	allThemes := theme.All()
	initialSelected := 0
	for i, t := range allThemes {
		if t.ID == th.ID {
			initialSelected = i
			break
		}
	}

	bar := progress.New(
		progress.WithWidth(34),
		progress.WithoutPercentage(),
		progress.WithColors(lipgloss.Color(th.Accent), lipgloss.Color(th.Secondary)),
		progress.WithFillCharacters('━', '─'),
	)

	var volEvts <-chan int
	if len(volumeEvents) > 0 {
		volEvts = volumeEvents[0]
	}

	bitrate := cfg.Bitrate
	if bitrate == "" {
		bitrate = "320"
	}

	bgMode := cfg.Background
	if bgMode == "" {
		bgMode = "flow"
	}
	if bgMode != "default" && bgMode != "flow" && bgMode != "dark" && bgMode != "gradient" {
		bgMode = "flow"
	}

	m := Model{
		client:        client,
		localName:     localName,
		engineDone:    engineDone,
		engineEvents:  engineEvents,
		volumeEvents:  volEvts,
		bitrate:       bitrate,
		bgMode:        bgMode,
		isFocused:     true,
		width:         80,
		height:        24,
		progress:      bar,
		theme:         th,
		themeList:     allThemes,
		themeSelected: initialSelected,
		entriesCache:  make(map[string][]spotify.PlaylistEntry),
		bigCoverCache: make(map[string][]string),
	}
	m.setView(viewPlaylists)
	return m
}

func (m *Model) getBigCoverLines(widthChars, heightChars int) []string {
	if m.coverData == nil {
		return nil
	}
	if m.coverData.Image == nil {
		if len(m.coverData.BigLines) > 0 {
			if len(m.coverData.BigLines) >= heightChars {
				return m.coverData.BigLines[:heightChars]
			}
			return m.coverData.BigLines
		}
		return nil
	}
	if m.zenTransActive && m.zenTransFrame < ZenTransitionMaxFrames {
		progress := float64(m.zenTransFrame) / float64(ZenTransitionMaxFrames)
		return RenderBlurredCoverLines(m.coverData.Image, m.zenPrevImage, widthChars, heightChars, progress)
	}
	if m.bigCoverCache == nil {
		m.bigCoverCache = make(map[string][]string)
	}
	key := fmt.Sprintf("%s:%dx%d", m.currentCoverURL, widthChars, heightChars)
	if lines, ok := m.bigCoverCache[key]; ok {
		return lines
	}
	lines := RenderCoverLines(m.coverData.Image, widthChars, heightChars)
	m.bigCoverCache[key] = lines
	return lines
}

func (m *Model) triggerToast(msg string, durationTicks int) tea.Cmd {
	m.toastMessage = msg
	m.toastTimer = durationTicks
	m.toastSeq++
	seq := m.toastSeq
	duration := time.Duration(max(1, durationTicks)) * 120 * time.Millisecond
	return tea.Tick(duration, func(time.Time) tea.Msg { return toastExpireMsg{seq: seq} })
}

const (
	zenAnimationInterval    = 120 * time.Millisecond
	zenTransitionInterval   = 33 * time.Millisecond
	playbackRefreshInterval = time.Second
)

func (m *Model) reconcileClocks() []tea.Cmd {
	var cmds []tea.Cmd
	zenActive := m.isFocused && m.currentView == viewCoverArt
	if !zenActive && m.zenTickScheduled {
		m.zenTickScheduled = false
		m.zenClockEpoch++
	}
	if zenActive && !m.zenTickScheduled {
		m.zenClockEpoch++
		epoch := m.zenClockEpoch
		m.zenTickScheduled = true
		cmds = append(cmds, tea.Tick(zenAnimationInterval, func(time.Time) tea.Msg {
			return zenTickMsg{epoch: epoch}
		}))
	}
	transitionActive := zenActive && m.zenTransActive
	if !transitionActive && m.zenTransitionTickScheduled {
		m.zenTransitionTickScheduled = false
		m.zenTransitionClockEpoch++
	}
	if transitionActive && !m.zenTransitionTickScheduled {
		m.zenTransitionClockEpoch++
		epoch := m.zenTransitionClockEpoch
		m.zenTransitionTickScheduled = true
		cmds = append(cmds, tea.Tick(zenTransitionInterval, func(time.Time) tea.Msg {
			return zenTransitionTickMsg{epoch: epoch}
		}))
	}

	playbackActive := m.isFocused && m.currentView != viewCoverArt && m.currentView != viewIntro && m.progressClockRunning()
	if !playbackActive && m.playbackTickScheduled {
		m.playbackTickScheduled = false
		m.playbackClockEpoch++
	}
	if playbackActive && !m.playbackTickScheduled {
		m.playbackClockEpoch++
		epoch := m.playbackClockEpoch
		m.playbackTickScheduled = true
		cmds = append(cmds, tea.Tick(playbackRefreshInterval, func(time.Time) tea.Msg {
			return playbackTickMsg{epoch: epoch}
		}))
	}
	return cmds
}

func (m *Model) SetEngineRestarter(fn func(bitrate string) error) {
	m.engineRestart = fn
}

func (m *Model) SetTrackEvents(ch <-chan player.TrackEvent) {
	m.trackEvents = ch
}

func (m *Model) EnableIntro() {
	m.introActive = true
	m.introFrame = 0
	m.introStartTime = time.Now()
	m.setView(viewIntro)
}

func introTick() tea.Cmd {
	return tea.Tick(30*time.Millisecond, func(time.Time) tea.Msg {
		return introTickMsg{}
	})
}

func (m *Model) checkCoverFetch() tea.Cmd {
	if m.state.Item == nil {
		return nil
	}
	url := m.state.Item.CoverURL()
	if url != "" && url != m.currentCoverURL {
		m.currentCoverURL = url
		return GetCoverManager().FetchCover(url)
	}
	return nil
}

func (m *Model) fetchTrackMetadata(uri string) tea.Cmd {
	if m.client == nil || uri == "" {
		return nil
	}
	if m.fetchingTrackURI == uri {
		return nil
	}
	m.fetchingTrackURI = uri
	return func() tea.Msg {
		track, err := m.client.GetTrack(context.Background(), uri)
		return trackFetchedMsg{track: track, err: err, uri: uri}
	}
}

func (m *Model) fetchState() tea.Cmd {
	if m.stateFetching || time.Since(m.lastStateFetch) < 2000*time.Millisecond {
		return nil
	}
	m.stateFetching = true
	m.lastStateFetch = time.Now()
	epoch := m.stateEpoch
	return func() tea.Msg {
		state, err := m.client.Playback(context.Background())
		return stateMsg{state, err, epoch}
	}
}

func (m *Model) forceFetchState() tea.Cmd {
	m.stateFetching = true
	m.lastStateFetch = time.Now()
	epoch := m.stateEpoch
	return func() tea.Msg {
		state, err := m.client.Playback(context.Background())
		return stateMsg{state, err, epoch}
	}
}

func (m Model) fetchDevices() tea.Cmd {
	return func() tea.Msg {
		devices, err := m.client.Devices(context.Background())
		return devicesMsg{devices, err}
	}
}

func (m Model) fetchPlaylists(offset int) tea.Cmd {
	seq := m.playlistSeq
	return func() tea.Msg {
		page, err := m.client.Playlists(context.Background(), offset)
		return playlistsMsg{page: page, err: err, seq: seq, offset: offset}
	}
}

func (m Model) fetchEntries(offset int) tea.Cmd {
	seq, playlistID := m.entriesSeq, m.activePlaylist.ID
	return func() tea.Msg {
		page, err := m.client.PlaylistEntries(context.Background(), playlistID, offset)
		return playlistEntriesMsg{page: page, err: err, seq: seq, offset: offset, playlistID: playlistID}
	}
}

func (m Model) playPlaylist(position int) (tea.Model, tea.Cmd) {
	device, ok := m.localDevice()
	if !ok {
		if m.state.Device != nil && m.state.Device.ID != "" && !m.state.Device.IsRestricted {
			device = *m.state.Device
			ok = true
		}
	}
	if !ok {
		m.status, m.statusError = "Dispositivo local no disponible", true
		return m, nil
	}
	if m.transportBusy {
		return m, nil
	}
	playlist := m.activePlaylist
	uri := playlist.URI
	if uri == "" {
		uri = "spotify:playlist:" + playlist.ID
	}
	m.transportBusy = true
	m.stateEpoch++
	m.pendingContext, m.pendingSince = uri, time.Now()
	m.status, m.statusError = "", false

	// Optimistic immediate track activation (< 1ms)
	if position >= 0 && position < len(m.entries) {
		trackCopy := m.entries[position].Track
		m.state.Item = &trackCopy
		m.state.IsPlaying = true
		m.state.ProgressMS = 0
		m.lastSync = time.Now()
		val := true
		m.desiredPlaying = &val
		m.desiredSince = time.Now()
	}

	actCmd := m.act("playlist", func(ctx context.Context) error {
		return m.client.PlayPlaylist(ctx, playlist, position, device.ID)
	})
	if coverCmd := m.checkCoverFetch(); coverCmd != nil {
		return m, tea.Batch(actCmd, coverCmd)
	}
	return m, actCmd
}

func (m Model) playTrack(track spotify.Track) (tea.Model, tea.Cmd) {
	device, ok := m.localDevice()
	if !ok {
		if m.state.Device != nil && m.state.Device.ID != "" && !m.state.Device.IsRestricted {
			device = *m.state.Device
			ok = true
		}
	}
	if !ok {
		m.status, m.statusError = "Dispositivo local no disponible", true
		return m, nil
	}
	if m.transportBusy {
		return m, nil
	}
	m.transportBusy = true
	m.stateEpoch++
	m.status, m.statusError = "", false

	// Optimistic immediate track activation (< 1ms)
	trackCopy := track
	m.state.Item = &trackCopy
	m.state.IsPlaying = true
	m.state.ProgressMS = 0
	m.lastSync = time.Now()
	val := true
	m.desiredPlaying = &val
	m.desiredSince = time.Now()

	actCmd := m.act("canción", func(ctx context.Context) error {
		return m.client.PlayTrack(ctx, track, device.ID)
	})
	if coverCmd := m.checkCoverFetch(); coverCmd != nil {
		return m, tea.Batch(actCmd, coverCmd)
	}
	return m, actCmd
}

func (m Model) currentPositionMS() int {
	pos := m.state.ProgressMS
	if m.progressClockRunning() && !m.lastSync.IsZero() {
		pos += int(time.Since(m.lastSync).Milliseconds())
	}
	pos = max(0, pos)
	if m.state.Item != nil && m.state.Item.DurationMS > 0 {
		pos = min(pos, m.state.Item.DurationMS)
	}
	return pos
}

// progressClockRunning keeps the displayed position pinned while a pause
// request is awaiting Spotify's response. The playback icon/state itself stays
// unchanged until the API confirms the command.
func (m Model) progressClockRunning() bool {
	return m.state.IsPlaying && !(m.transportBusy && m.desiredPlaying != nil && !*m.desiredPlaying)
}

func (m *Model) snapshotPlaybackPosition() {
	m.state.ProgressMS = m.currentPositionMS()
	m.lastSync = time.Now()
}

func (m Model) applyTrackTransition(evt player.TrackEvent) (Model, tea.Cmd) {
	if m.coverData != nil && m.coverData.Image != nil {
		m.zenPrevImage = m.coverData.Image
	}
	m.coverData = nil
	m.currentCoverURL = ""
	m.bigCoverCache = make(map[string][]string)
	m.state.ProgressMS = 0
	m.lastSync = time.Now()
	m.state.IsPlaying = true
	m.boundaryProbes = 0
	if m.currentView == viewCoverArt {
		m.triggerZenTransition()
	}

	var matched *spotify.Track
	for i := range m.entries {
		if (evt.URI != "" && m.entries[i].Track.URI == evt.URI) ||
			(evt.Name != "" && strings.EqualFold(m.entries[i].Track.Name, evt.Name)) {
			matched = &m.entries[i].Track
			break
		}
	}

	if matched != nil {
		m.state.Item = matched
		if evt.DurationMS > 0 {
			m.state.Item.DurationMS = evt.DurationMS
		}
		m.state.IsPlaying = true
	} else if evt.Name != "" || evt.URI != "" {
		if m.state.Item == nil || (evt.URI != "" && m.state.Item.URI != evt.URI) {
			dur := evt.DurationMS
			if dur <= 0 && m.state.Item != nil && m.state.Item.URI == evt.URI {
				dur = m.state.Item.DurationMS
			}
			m.state.Item = &spotify.Track{
				Name:       evt.Name,
				URI:        evt.URI,
				DurationMS: dur,
			}
		} else if evt.DurationMS > 0 && m.state.Item != nil {
			m.state.Item.DurationMS = evt.DurationMS
		}
		m.state.IsPlaying = true
	}

	var cmds []tea.Cmd
	cmds = append(cmds, m.listenTrack())

	uriToFetch := ""
	if evt.URI != "" {
		uriToFetch = evt.URI
	} else if m.state.Item != nil && m.state.Item.URI != "" {
		uriToFetch = m.state.Item.URI
	}

	// Fast cache lookup
	if uriToFetch != "" && m.client != nil {
		if cached, ok := m.client.GetCachedTrack(uriToFetch); ok {
			dur := cached.DurationMS
			if evt.DurationMS > 0 {
				dur = evt.DurationMS
			} else if m.state.Item != nil && m.state.Item.DurationMS > 0 {
				dur = m.state.Item.DurationMS
			}
			cached.DurationMS = dur
			m.state.Item = &cached
		}
	}

	needsMetadata := m.state.Item == nil || len(m.state.Item.Artists) == 0 || len(m.state.Item.Album.Images) == 0 || m.state.Item.Album.Name == ""
	if uriToFetch != "" && needsMetadata {
		if fetchCmd := m.fetchTrackMetadata(uriToFetch); fetchCmd != nil {
			cmds = append(cmds, fetchCmd)
		}
	}

	if coverCmd := m.checkCoverFetch(); coverCmd != nil {
		cmds = append(cmds, coverCmd)
	}

	return m, tea.Batch(cmds...)
}

// Polling budget for the playback-state refresh loop.
//
// Spotify exposes no push channel for remote playback (the local librespot
// process only reports its own audio), so the UI has to poll. The previous fixed
// 25s tick meant a song changed on the phone could take up to 25s to appear — and
// up to 100s when the local device happened to be the active one. These constants
// trade a slightly higher request rate for near-instant synchronization while
// staying far away from the 429 threshold.
const (
	minPollDelay       = 3 * time.Second
	remotePollCeiling  = 10 * time.Second
	localPollCeiling   = 20 * time.Second
	boundaryProbeDelay = 2 * time.Second
	rateLimitedDelay   = 30 * time.Second
	unfocusedDelay     = 20 * time.Second
	pausedDelay        = 12 * time.Second
	unknownDelay       = 5 * time.Second
	maxBoundaryProbes  = 2
	boundaryWindowMS   = 1500
	keySyncStaleAfter  = 3 * time.Second
)

// pollAfter schedules the next playback-state refresh.
func pollAfter(d time.Duration) tea.Cmd {
	if d < minPollDelay {
		d = minPollDelay
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return pollMsg{} })
}

func poll() tea.Cmd {
	return pollAfter(unknownDelay)
}

func clampDuration(d, lo, hi time.Duration) time.Duration {
	if d < lo {
		return lo
	}
	if d > hi {
		return hi
	}
	return d
}

// pollDelay computes how long to wait before the next playback-state refresh.
//
// While playing it aims just past the end of the current track ("boundary probe")
// so a remote track change is picked up within a couple of seconds, and it stays
// responsive mid-track as well.
func (m Model) pollDelay() time.Duration {
	if !m.isFocused {
		return unfocusedDelay
	}
	if m.client != nil && m.client.IsBlocked() {
		return rateLimitedDelay
	}
	if m.state.Item == nil || m.state.Item.DurationMS <= 0 {
		return unknownDelay
	}
	if !m.state.IsPlaying {
		return pausedDelay
	}
	remaining := time.Duration(m.remainingMS()) * time.Millisecond
	if m.isLocalActive() {
		// librespot already streams precise track events for the local device, so
		// polling is only a safety net here.
		return clampDuration(remaining+boundaryProbeDelay, 6*time.Second, localPollCeiling)
	}
	return clampDuration(remaining+boundaryProbeDelay, minPollDelay, remotePollCeiling)
}

// remainingMS reports how much of the current track is still unplayed.
func (m Model) remainingMS() int {
	if !m.state.Available || m.state.Item == nil || m.state.Item.DurationMS <= 0 {
		return 0
	}
	return max(0, m.state.Item.DurationMS-m.currentPositionMS())
}

// atTrackBoundary reports whether the extrapolated position reached the end of the
// current track, meaning the service should already be reporting the next one.
func (m Model) atTrackBoundary() bool {
	if !m.state.IsPlaying || m.state.Item == nil || m.state.Item.DurationMS <= 0 {
		return false
	}
	return m.currentPositionMS() >= m.state.Item.DurationMS-boundaryWindowMS
}

// keySyncCmd refreshes a stale playback state after user input, so touching the UI
// always snaps it back in sync with whatever device is playing.
func (m *Model) keySyncCmd() tea.Cmd {
	if m.client == nil || m.stateFetching || time.Since(m.lastStateFetch) < keySyncStaleAfter {
		return nil
	}
	return m.forceFetchState()
}

type updateCheckMsg struct {
	release *version.GitHubRelease
	err     error
}

type updateApplyMsg struct {
	err error
}

func checkUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rel, err := version.CheckLatest(ctx)
		return updateCheckMsg{release: rel, err: err}
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.forceFetchState(),
		m.fetchDevices(),
		m.fetchPlaylists(0),
		poll(),
		m.listenEngine(),
		m.listenVolume(),
		checkUpdateCmd(),
		func() tea.Msg {
			return engineStoppedMsg{err: <-m.engineDone}
		},
	}
	if m.trackEvents != nil {
		cmds = append(cmds, m.listenTrack())
	}
	if m.introActive {
		cmds = append(cmds, introTick())
	}
	return tea.Batch(cmds...)
}

func (m Model) listenEngine() tea.Cmd {
	return func() tea.Msg {
		if m.engineEvents == nil {
			return nil
		}
		_, ok := <-m.engineEvents
		if !ok {
			return nil
		}
		return engineEventMsg{}
	}
}

type volumeEventMsg struct {
	percent int
}

func (m Model) listenVolume() tea.Cmd {
	return func() tea.Msg {
		if m.volumeEvents == nil {
			return nil
		}
		val, ok := <-m.volumeEvents
		if !ok {
			return nil
		}
		return volumeEventMsg{percent: val}
	}
}

func (m Model) listenTrack() tea.Cmd {
	return func() tea.Msg {
		if m.trackEvents == nil {
			return nil
		}
		val, ok := <-m.trackEvents
		if !ok {
			return nil
		}
		return trackEventMsg{event: val}
	}
}

func (m Model) act(name string, fn func(context.Context) error) tea.Cmd {
	return func() tea.Msg { return actionMsg{name: name, err: fn(context.Background())} }
}

func (m Model) localDevice() (spotify.Device, bool) {
	for _, device := range m.devices {
		if device.Name == m.localName && device.ID != "" && !device.IsRestricted {
			return device, true
		}
	}
	return spotify.Device{}, false
}

func (m Model) isLocalActive() bool {
	if m.state.Device == nil {
		return false
	}
	name := strings.ToLower(m.state.Device.Name)
	local := strings.ToLower(m.localName)
	return (local != "" && name == local) || strings.HasPrefix(name, "spotifygo") || strings.HasPrefix(name, "spotygo")
}

// Update dispatches a message to the model and, on user input, opportunistically
// re-synchronizes the remote playback state. Spotify offers no push channel for
// "currently playing" on another device, so a fresh state right after a keystroke
// is what makes a song changed on the phone show up immediately instead of waiting
// for the next scheduled poll.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(message)
	model, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	if clocks := model.reconcileClocks(); len(clocks) > 0 {
		cmd = tea.Batch(cmd, tea.Batch(clocks...))
	}
	if key, isKey := message.(tea.KeyPressMsg); isKey && key.String() != "ctrl+c" {
		if syncCmd := model.keySyncCmd(); syncCmd != nil {
			return model, tea.Batch(cmd, syncCmd)
		}
		return model, cmd
	}
	return model, cmd
}

func (m Model) update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case volumeEventMsg:
		val := msg.percent
		if m.state.Device == nil {
			m.state.Device = &spotify.Device{Name: m.localName}
		}
		m.state.Device.VolumePercent = &val
		m.volumeExpected = &val
		m.volumeDeviceID = m.state.Device.ID
		m.volumeSetAt = time.Now()
		m.volumeOverride = nil
		for i := range m.devices {
			if m.devices[i].Name == m.localName || (m.state.Device != nil && m.devices[i].ID == m.state.Device.ID) {
				m.devices[i].VolumePercent = &val
			}
		}
		return m, m.listenVolume()
	case coverLoadedMsg:
		if msg.url == m.currentCoverURL {
			m.coverData = &msg.data
			if m.currentView == viewCoverArt {
				m.triggerZenTransition()
			}
		}
		return m, nil
	case trackEventMsg:
		evt := msg.event
		isTrackChange := false
		if evt.URI != "" {
			if m.state.Item == nil || m.state.Item.URI != evt.URI {
				isTrackChange = true
			}
		} else if evt.Name != "" {
			if m.state.Item == nil || !strings.EqualFold(m.state.Item.Name, evt.Name) {
				isTrackChange = true
			}
		}

		if isTrackChange {
			// Check if this is a gapless pre-fetch from librespot!
			// If current track is playing, user didn't perform an explicit transport action,
			// and current track has > 4 seconds remaining, DO NOT swap UI track yet!
			remainingMS := 0
			if m.state.Item != nil && m.state.Item.DurationMS > 0 {
				remainingMS = m.state.Item.DurationMS - m.currentPositionMS()
			}
			isGaplessPrefetch := m.state.Item != nil && m.state.Item.URI != "" &&
				m.state.IsPlaying && remainingMS > 4000 &&
				!m.transportBusy && m.desiredPlaying == nil

			if isGaplessPrefetch {
				evtCopy := evt
				m.prefetchedTrack = &evtCopy
				// Pre-cache metadata and cover art in background so it's 100% ready when track finishes!
				var cmds []tea.Cmd
				cmds = append(cmds, m.listenTrack())
				if evt.URI != "" && m.client != nil {
					if _, ok := m.client.GetCachedTrack(evt.URI); !ok {
						if fetchCmd := m.fetchTrackMetadata(evt.URI); fetchCmd != nil {
							cmds = append(cmds, fetchCmd)
						}
					}
				}
				return m, tea.Batch(cmds...)
			}

			m.prefetchedTrack = nil
			return m.applyTrackTransition(evt)
		}

		// Not a track change: update duration if available and keep listening
		if evt.DurationMS > 0 && m.state.Item != nil {
			m.state.Item.DurationMS = evt.DurationMS
		}
		return m, m.listenTrack()
	case trackFetchedMsg:
		m.fetchingTrackURI = ""
		if msg.err != nil {
			logger.Warn("No se pudo obtener metadatos de Spotify para track %s: %v", msg.uri, msg.err)
			return m, nil
		}
		if m.state.Item != nil && (m.state.Item.URI == msg.track.URI || m.state.Item.URI == "" || strings.EqualFold(m.state.Item.Name, msg.track.Name)) {
			dur := m.state.Item.DurationMS
			if msg.track.DurationMS > 0 {
				dur = msg.track.DurationMS
			}
			m.state.Item = &msg.track
			m.state.Item.DurationMS = dur

			var cmds []tea.Cmd
			if coverCmd := m.checkCoverFetch(); coverCmd != nil {
				cmds = append(cmds, coverCmd)
			}
			if len(cmds) > 0 {
				return m, tea.Batch(cmds...)
			}
		}
		return m, nil
	case engineEventMsg:
		var cmds []tea.Cmd
		cmds = append(cmds, m.listenEngine())
		if stateCmd := m.fetchState(); stateCmd != nil {
			cmds = append(cmds, stateCmd)
		}
		return m, tea.Batch(cmds...)
	case tea.FocusMsg:
		m.isFocused = true
		if m.introActive {
			return m, tea.Batch(introTick(), m.fetchState())
		}
		return m, m.fetchState()
	case tea.BlurMsg:
		m.isFocused = false
		// Window lost focus: zero CPU in background! Do not schedule ticks while unfocused.
		return m, nil
	case zenTickMsg:
		if !m.zenTickScheduled || msg.epoch != m.zenClockEpoch || !m.isFocused || m.currentView != viewCoverArt {
			return m, nil
		}
		m.zenTickScheduled = false
		m.zenFrame++
		if m.atTrackBoundary() && m.prefetchedTrack != nil {
			nextEvt := *m.prefetchedTrack
			m.prefetchedTrack = nil
			return m.applyTrackTransition(nextEvt)
		}
		return m, nil
	case zenTransitionTickMsg:
		if !m.zenTransitionTickScheduled || msg.epoch != m.zenTransitionClockEpoch || !m.isFocused || m.currentView != viewCoverArt || !m.zenTransActive {
			return m, nil
		}
		m.zenTransitionTickScheduled = false
		m.zenTransFrame++
		if m.zenTransFrame >= ZenTransitionMaxFrames {
			m.zenTransActive = false
		}
		return m, nil
	case playbackTickMsg:
		if !m.playbackTickScheduled || msg.epoch != m.playbackClockEpoch || !m.isFocused || m.currentView == viewCoverArt || !m.state.IsPlaying {
			return m, nil
		}
		m.playbackTickScheduled = false
		if m.atTrackBoundary() && m.prefetchedTrack != nil {
			nextEvt := *m.prefetchedTrack
			m.prefetchedTrack = nil
			return m.applyTrackTransition(nextEvt)
		}
		return m, nil
	case toastExpireMsg:
		if msg.seq == m.toastSeq {
			m.toastMessage = ""
			m.toastTimer = 0
		}
		return m, nil
	case loginMsg:
		if msg.err != nil {
			m.status = "No pudimos entrar en Spotify: " + msg.err.Error()
			m.statusError = true
		} else {
			m.status = "¡Listo! Sesión de Spotify confirmada"
			m.statusError = false
		}
		return m, tea.Batch(m.fetchState(), m.fetchDevices(), m.fetchPlaylists(0))
	case updateCheckMsg:
		if msg.err == nil && msg.release != nil && version.IsNewer(msg.release.TagName, version.Current) {
			m.updateRelease = msg.release
			m.updateInProgress = false
			m.updateError = ""
			m.updateDone = false
			if m.currentView == viewPlaylists || m.currentView == viewCoverArt {
				m.setView(viewUpdatePrompt)
			}
		}
		return m, nil
	case updateApplyMsg:
		m.updateInProgress = false
		if msg.err != nil {
			m.updateError = msg.err.Error()
			m.status = "No pudimos actualizar: " + msg.err.Error()
			m.statusError = true
		} else {
			m.updateDone = true
			m.status = "¡Actualizado! Reinicia SpotifyGo para estrenarlo"
			m.statusError = false
		}
		return m, nil
	case searchMsg:
		if msg.seq != m.searchSeq || m.currentView != viewSearch {
			break
		}
		if msg.err != nil {
			logger.Error("Búsqueda fallida: %v", msg.err)
			m.status = msg.err.Error()
			m.statusError = true
		} else {
			m.statusError = false
			m.searchResults = msg.tracks
			m.searchSelected = 0
			m.status = ""
		}
	case playlistsMsg:
		if msg.seq != m.playlistSeq {
			break
		}
		m.playlistBusy = false
		if msg.err != nil {
			logger.Error("Carga de playlists fallida: %v", msg.err)
			m.status, m.statusError = msg.err.Error(), true
		} else {
			if msg.offset == 0 {
				m.playlists = nil
			}
			m.playlists = append(m.playlists, msg.page.Items...)
			m.playlistTotal, m.playlistMore = msg.page.Total, msg.page.Next != ""
			m.status, m.statusError = "", false
			if m.playlistPick >= len(m.playlists) {
				m.playlistPick = max(0, len(m.playlists)-1)
			}
		}
	case playlistEntriesMsg:
		if msg.seq != m.entriesSeq || m.currentView != viewTracks || msg.playlistID != m.activePlaylist.ID {
			break
		}
		m.entriesBusy = false
		if msg.err != nil {
			if strings.Contains(msg.err.Error(), "HTTP 403") {
				m.status = "Spotify limita las playlists seguidas: pulsa 'p' para reproducirla completa"
			} else {
				m.status = msg.err.Error()
			}
			m.statusError = true
		} else {
			if msg.offset == 0 {
				m.entries = nil
			}
			m.entries = append(m.entries, msg.page.Items...)
			m.entriesTotal, m.entriesMore = msg.page.Total, msg.page.Next != ""
			m.entriesOffset = msg.offset + 50
			m.status, m.statusError = "", false
			if m.entryPick >= len(m.entries) {
				m.entryPick = max(0, len(m.entries)-1)
			}
			if m.entriesCache == nil {
				m.entriesCache = make(map[string][]spotify.PlaylistEntry)
			}
			m.entriesCache[msg.playlistID] = m.entries
			if m.entriesMore && len(m.entries) == 0 {
				m.entriesBusy = true
				return m, m.fetchEntries(m.entriesOffset)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case engineStoppedMsg:
		if msg.err != nil {
			logger.Error("Motor local librespot detenido con error: %v", msg.err)
			m.status = "El motor local se detuvo: " + msg.err.Error()
			m.statusError = true
		}
		return m, nil
	case stateMsg:
		m.stateFetching = false
		if msg.epoch < m.stateEpoch {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
			return m, nil
		}
		m.statusError = false
		if m.client != nil && msg.state.Item != nil {
			m.client.CacheTrack(*msg.state.Item)
		}
		savedProgress := m.state.ProgressMS
		savedItem := m.state.Item
		savedPlaying := m.state.IsPlaying
		savedLastSync := m.lastSync
		m.state = msg.state
		m.lastSync = time.Now()
		if m.volumeOverride != nil {
			m.state.Device.VolumePercent = m.volumeOverride
		} else if m.volumeExpected != nil && m.state.Device != nil && m.state.Device.ID == m.volumeDeviceID {
			if m.state.Device.VolumePercent != nil && *m.state.Device.VolumePercent == *m.volumeExpected {
				m.volumeExpected = nil
			} else if time.Since(m.volumeSetAt) < 1800*time.Millisecond {
				m.state.Device.VolumePercent = m.volumeExpected
			} else {
				m.volumeExpected = nil
			}
		}
		if m.desiredPlaying != nil {
			if m.state.IsPlaying == *m.desiredPlaying {
				m.desiredPlaying = nil
				m.transportBusy = false
			} else if m.transportBusy || time.Since(m.desiredSince) < 2500*time.Millisecond {
				// Spotify Web API eventual consistency delay:
				// Preserve both the displayed state and its clock while a transport
				// command is still waiting for its API response.
				if m.transportBusy {
					m.state.IsPlaying = savedPlaying
					m.state.ProgressMS = savedProgress
					m.lastSync = savedLastSync
				} else if !*m.desiredPlaying {
					m.state.IsPlaying = false
					m.state.ProgressMS = savedProgress
				} else {
					// When unpausing from pause (savedPlaying == false), keep player waiting
					// until actionMsg confirms. When starting new track/playlist (savedPlaying == true),
					// keep it playing!
					m.state.IsPlaying = savedPlaying
				}
				if savedItem != nil && (m.state.Item == nil || (savedItem.URI != "" && m.state.Item.URI != savedItem.URI)) {
					m.state.Item = savedItem
				}
			} else {
				m.desiredPlaying = nil
				m.transportBusy = false
			}
		}
		if m.isLocalActive() && savedItem != nil && savedItem.URI != "" {
			if msg.state.Item == nil || (msg.state.Item.URI != savedItem.URI && (m.desiredPlaying != nil || time.Since(m.lastTransport) < 2500*time.Millisecond)) {
				// Spotify Web API lag: keep our authoritative active local track only while manual transport action is recent
				m.state.Item = savedItem
			} else if msg.state.Item != nil && msg.state.Item.URI == savedItem.URI {
				m.state.ProgressMS = msg.state.ProgressMS
				m.lastSync = time.Now()
			}
		}
		trackChanged := false
		if m.state.Item != nil {
			if savedItem == nil {
				trackChanged = true
			} else if m.state.Item.URI != "" && savedItem.URI != "" && m.state.Item.URI != savedItem.URI {
				trackChanged = true
			} else if m.state.Item.Name != "" && savedItem.Name != "" && !strings.EqualFold(m.state.Item.Name, savedItem.Name) {
				trackChanged = true
			}
		}
		if trackChanged {
			m.prefetchedTrack = nil
			m.boundaryProbes = 0
			if m.coverData != nil && m.coverData.Image != nil {
				m.zenPrevImage = m.coverData.Image
			}
			m.coverData = nil
			m.currentCoverURL = ""
			m.bigCoverCache = make(map[string][]string)
			if m.currentView == viewCoverArt {
				m.triggerZenTransition()
			}
		}
		if m.pendingContext != "" {
			if (m.state.Context != nil && m.state.Context.URI == m.pendingContext) || time.Since(m.pendingSince) > 3500*time.Millisecond {
				m.pendingContext = ""
				m.transportBusy = false
			}
		}
		if m.pendingDeviceID != "" {
			if (m.state.Device != nil && (m.state.Device.ID == m.pendingDeviceID || m.state.Device.Name == m.pendingDeviceName)) || time.Since(m.pendingDeviceSince) > 2500*time.Millisecond {
				m.pendingDeviceID = ""
				m.pendingDeviceName = ""
				m.transportBusy = false
			}
		}
		var stateCmds []tea.Cmd
		if coverCmd := m.checkCoverFetch(); coverCmd != nil {
			stateCmds = append(stateCmds, coverCmd)
		}
		if len(stateCmds) > 0 {
			return m, tea.Batch(stateCmds...)
		}
		return m, nil
	case devicesMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
			return m, nil
		}
		m.statusError = false
		m.devices = msg.devices
		for _, device := range m.devices {
			if device.Name == m.localName && device.ID != "" && !device.IsRestricted {
				m.localReady = true
				if !m.transferSent && !device.IsActive {
					m.transferSent = true
					m.transportBusy = true
					m.stateEpoch++
					m.status = "Trayendo la música a esta computadora…"
					m.statusError = false
					m.pendingDeviceID = device.ID
					m.pendingDeviceName = device.Name
					m.pendingDeviceSince = time.Now()
					var prevVol *int
					if m.state.Device != nil && m.state.Device.VolumePercent != nil {
						prevVol = m.state.Device.VolumePercent
					}
					return m, m.act("transferencia local", func(ctx context.Context) error {
						if err := m.client.Transfer(ctx, device); err != nil {
							return err
						}
						if prevVol != nil && *prevVol > 0 {
							_ = m.client.SetVolume(ctx, *prevVol, device.ID)
						}
						return nil
					})
				}
				if !m.transferSent {
					m.transferSent = true
					m.status = ""
				}
				break
			}
		}
		if m.selected >= len(m.devices) {
			m.selected = max(0, len(m.devices)-1)
		}
	case actionMsg:
		if msg.name == "volumen" && msg.seq != m.volumeSeq {
			return m, nil
		}
		if msg.name != "volumen" {
			m.transportBusy = false
			m.stateEpoch++
		}
		if msg.name == "volumen" && msg.seq == m.volumeSeq {
			m.volumeOverride = nil
			m.stateEpoch++
			if msg.err == nil && m.state.Device != nil {
				value := msg.value
				m.state.Device.VolumePercent = &value
				m.volumeExpected = &value
				m.volumeDeviceID = m.state.Device.ID
				m.volumeSetAt = time.Now()
			}
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
			if msg.name == "playlist" {
				m.pendingContext = ""
			}
			if msg.name == "pausar" {
				m.desiredPlaying = nil
				m.state.IsPlaying = true
			} else if msg.name == "reproducir" {
				m.desiredPlaying = nil
				m.state.IsPlaying = false
			}
			if msg.name == "transferir" || msg.name == "transferencia local" {
				m.pendingDeviceID = ""
				m.pendingDeviceName = ""
			}
		} else {
			m.statusError = false
			switch msg.name {
			case "volumen":
				m.status = fmt.Sprintf("Volumen al %d%%", msg.value)
			case "transferir", "transferencia local":
				m.status = ""
				m.pendingDeviceID = ""
				m.pendingDeviceName = ""
				m.transportBusy = false
			case "siguiente":
				m.status = statusNext
				m.transportBusy = false
			case "anterior":
				m.status = statusPrev
				m.transportBusy = false
			case "pausar":
				m.status = ""
				m.transportBusy = false
				if m.state.IsPlaying && !m.lastSync.IsZero() {
					m.state.ProgressMS += int(time.Since(m.lastSync).Milliseconds())
					if m.state.Item != nil && m.state.ProgressMS > m.state.Item.DurationMS {
						m.state.ProgressMS = m.state.Item.DurationMS
					}
				}
				m.lastSync = time.Now()
				m.state.IsPlaying = false
			case "reproducir":
				m.status = ""
				m.transportBusy = false
				m.state.IsPlaying = true
				m.lastSync = time.Now()
			case "canción", "playlist":
				m.status = "Dale, suena ♪"
				m.transportBusy = false
			default:
				m.status = "Hecho ♪"
				m.transportBusy = false
			}
		}
		if msg.name == "volumen" {
			return m, nil
		}
		if msg.name == "transferir" || msg.name == "transferencia local" {
			return m, tea.Batch(m.fetchState(), tea.Tick(2*time.Second, func(time.Time) tea.Msg { return refreshDueMsg{devices: true} }))
		}
		if msg.name == "pausar" {
			return m, tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return refreshDueMsg{} })
		}
		if msg.name == "reproducir" {
			return m, tea.Batch(
				tea.Tick(800*time.Millisecond, func(time.Time) tea.Msg { return refreshDueMsg{} }),
			)
		}
		if msg.name == "playlist" || msg.name == "canción" {
			return m, tea.Batch(m.fetchState(), tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return refreshDueMsg{} }))
		}
		return m, m.fetchState()
	case refreshDueMsg:
		if msg.devices {
			return m, tea.Batch(m.fetchState(), m.fetchDevices())
		}
		return m, m.fetchState()
	case volumeDueMsg:
		if msg.seq != m.volumeSeq || m.volumeOverride == nil || m.state.Device == nil {
			break
		}
		value := *m.volumeOverride
		deviceID := m.state.Device.ID
		return m, func() tea.Msg {
			err := m.client.SetVolume(context.Background(), value, deviceID)
			return actionMsg{name: "volumen", seq: msg.seq, err: err, value: value}
		}
	case pollMsg:
		if m.client != nil && m.client.IsBlocked() {
			return m, pollAfter(rateLimitedDelay)
		}
		// Boundary probe: right after a track ends Spotify may still answer with the
		// previous item, so re-ask a couple of times instead of waiting a full cycle.
		if m.atTrackBoundary() && m.boundaryProbes < maxBoundaryProbes {
			m.boundaryProbes++
			return m, tea.Batch(m.forceFetchState(), pollAfter(boundaryProbeDelay))
		}
		if !m.atTrackBoundary() {
			m.boundaryProbes = 0
		}
		cmds := []tea.Cmd{m.fetchState()}
		if !m.localReady {
			cmds = append(cmds, m.fetchDevices())
		}
		cmds = append(cmds, pollAfter(m.pollDelay()))
		return m, tea.Batch(cmds...)
	case engineRestartMsg:
		if msg.err != nil {
			m.status = "No pudimos reiniciar el motor: " + msg.err.Error()
			m.statusError = true
		} else {
			m.status = fmt.Sprintf("Motor reiniciado a %s kbps ♪", msg.bitrate)
			m.statusError = false
		}
		return m, nil
	case introTickMsg:
		if !m.introActive || m.currentView != viewIntro {
			return m, nil
		}
		if !m.isFocused {
			return m, nil
		}
		m.introFrame++
		if m.introFrame > 165 || time.Since(m.introStartTime) >= 5000*time.Millisecond {
			m.introActive = false
			m.setView(viewPlaylists)
			return m, nil
		}
		return m, introTick()
	case tea.KeyPressMsg:
		key := msg.String()
		repeated := msg.Key().IsRepeat
		if key == "ctrl+c" {
			return m, tea.Quit
		}

		if m.currentView == viewIntro {
			m.introActive = false
			m.setView(viewPlaylists)
			return m, nil
		}

		// 1. COMMAND OVERLAY MODE
		if m.commandActive {
			switch key {
			case "esc":
				m.commandActive = false
				m.commandInput = ""
				return m, nil
			case "backspace":
				chars := []rune(m.commandInput)
				if len(chars) > 0 {
					m.commandInput = string(chars[:len(chars)-1])
				}
				m.commandMatches = FilterCommands(m.commandInput)
				m.commandSelect = 0
				return m, nil
			case "up":
				if m.commandSelect > 0 {
					m.commandSelect--
				}
				return m, nil
			case "down":
				if m.commandSelect < len(m.commandMatches)-1 {
					m.commandSelect++
				}
				return m, nil
			case "enter":
				var cmdToRun CommandDef
				var ok bool
				trimmed := strings.TrimSpace(m.commandInput)
				if trimmed != "" {
					cmdToRun, ok = ResolveCommand(trimmed)
				}
				if !ok && len(m.commandMatches) > 0 && m.commandSelect < len(m.commandMatches) {
					cmdToRun = m.commandMatches[m.commandSelect]
					ok = true
				}
				m.commandActive = false
				m.commandInput = ""
				if !ok {
					m.status = fmt.Sprintf("Ese comando no existe: %q · prueba con 'help'", trimmed)
					m.statusError = true
					return m, nil
				}
				m.statusError = false
				switch cmdToRun.Action {
				case "login":
					m.status = "Abriendo el navegador para entrar en Spotify…"
					return m, func() tea.Msg {
						err := m.client.Reauth(context.Background())
						return loginMsg{err: err}
					}
				case "background":
					args := strings.Fields(trimmed)
					if len(args) > 1 {
						arg := strings.ToLower(args[1])
						targetBg := ""
						switch arg {
						case "default", "gradient", "def":
							targetBg = "gradient"
						case "flow", "wave", "motion":
							targetBg = "flow"
						case "dark", "theme":
							targetBg = "dark"
						default:
							m.status = fmt.Sprintf("Ese fondo no existe: %q · usa 'flow', 'gradient' o 'dark'", arg)
							m.statusError = true
							return m, nil
						}
						m.bgMode = targetBg
						_ = theme.SaveBackground(targetBg)
						m.status = ""
						toastCmd := m.triggerToast(fmt.Sprintf(toastBackgroundSet, backgroundLabel(targetBg)), 14)
						return m, toastCmd
					}

					// Without arguments: open interactive background modal
					m.commandActive = false
					m.commandInput = ""
					m.setView(viewBgPicker)
					m.bgPickerSelected = bgPickerIndex(m.bgMode)
					m.status = ""
					return m, nil
				case "help":
					m.status = ""
					toastCmd := m.triggerToast(toastCommands, 25)
					return m, toastCmd
				case "version":
					m.status = ""
					toastCmd := m.triggerToast("♪ SpotifyGo "+version.Current+" · github.com/VictorTrab/SpotyGo", 25)
					return m, toastCmd
				case "changelog":
					m.setView(viewChangelog)
					m.changelogScroll = 0
					m.status = ""
					return m, nil
				case "update":
					if m.updateRelease != nil && version.IsNewer(m.updateRelease.TagName, version.Current) {
						m.setView(viewUpdatePrompt)
						m.status = ""
						return m, nil
					}
					m.status = "Mirando si hay algo nuevo…"
					return m, func() tea.Msg {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						rel, err := version.CheckLatest(ctx)
						return updateCheckMsg{release: rel, err: err}
					}
				}
				return m, nil
			default:
				if text := msg.Key().Text; text != "" {
					m.commandInput += text
					m.commandMatches = FilterCommands(m.commandInput)
					m.commandSelect = 0
				}
				return m, nil
			}
		}

		// 2. VIEW: SEARCH MODE
		if m.currentView == viewSearch {
			switch key {
			case "esc":
				m.setView(m.previousView)
				m.searchSeq++
				m.searchInput = ""
				m.searchResults = nil
				m.lastSearchQuery = ""
				return m, nil
			case "backspace":
				m.searchSeq++
				chars := []rune(m.searchInput)
				if len(chars) > 0 {
					m.searchInput = string(chars[:len(chars)-1])
				}
				m.searchResults = nil
				m.lastSearchQuery = ""
				return m, nil
			case "up":
				if m.searchSelected > 0 {
					m.searchSelected--
				}
				return m, nil
			case "down":
				if m.searchSelected < len(m.searchResults)-1 {
					m.searchSelected++
				}
				return m, nil
			case "enter":
				query := strings.TrimSpace(m.searchInput)
				if len(m.searchResults) > 0 && query == m.lastSearchQuery && m.searchSelected >= 0 && m.searchSelected < len(m.searchResults) {
					device, ok := m.localDevice()
					if !ok {
						if m.state.Device != nil && m.state.Device.ID != "" && !m.state.Device.IsRestricted {
							device = *m.state.Device
							ok = true
						}
					}
					if !ok {
						m.status = "Tu PC aún no está lista, dame un segundo"
						m.statusError = true
						return m, nil
					}
					track := m.searchResults[m.searchSelected]
					m.setView(m.previousView)
					m.transportBusy = true
					m.stateEpoch++
					trackCopy := track
					m.state.Item = &trackCopy
					m.state.IsPlaying = true
					m.state.ProgressMS = 0
					m.lastSync = time.Now()
					m.status = "Suena " + track.Name + " ♪"
					m.statusError = false
					toastCmd := m.triggerToast("▶ Suena: "+track.Name, 12)
					return m, tea.Batch(m.act("canción", func(ctx context.Context) error { return m.client.PlayTrack(ctx, track, device.ID) }), toastCmd)
				}
				if query != "" {
					m.searchSeq++
					seq := m.searchSeq
					m.lastSearchQuery = query
					m.searchResults = nil
					m.searchSelected = 0
					m.status = "Buscando " + query + "…"
					m.statusError = false
					return m, func() tea.Msg {
						tracks, err := m.client.SearchTracks(context.Background(), query)
						return searchMsg{tracks: tracks, err: err, seq: seq}
					}
				}
				return m, nil
			default:
				if text := msg.Key().Text; text != "" {
					m.searchSeq++
					m.searchInput += text
					m.searchResults = nil
					m.lastSearchQuery = ""
				}
				return m, nil
			}
		}

		// 3. VIEW: THEME PICKER
		if m.currentView == viewThemePicker {
			switch key {
			case "esc":
				cfg := theme.LoadConfig()
				m.theme = theme.Get(cfg.Theme)
				m.progress = progress.New(
					progress.WithWidth(34),
					progress.WithoutPercentage(),
					progress.WithColors(lipgloss.Color(m.theme.Accent), lipgloss.Color(m.theme.Secondary)),
					progress.WithFillCharacters('━', '─'),
				)
				m.setView(m.previousView)
				m.status = toastThemeCancelled
				m.statusError = false
				return m, nil
			case "up", "k":
				if m.themeSelected > 0 {
					m.themeSelected--
				} else {
					m.themeSelected = len(m.themeList) - 1
				}
				m.theme = m.themeList[m.themeSelected]
				m.progress = progress.New(
					progress.WithWidth(34),
					progress.WithoutPercentage(),
					progress.WithColors(lipgloss.Color(m.theme.Accent), lipgloss.Color(m.theme.Secondary)),
					progress.WithFillCharacters('━', '─'),
				)
				return m, nil
			case "down", "j":
				if m.themeSelected < len(m.themeList)-1 {
					m.themeSelected++
				} else {
					m.themeSelected = 0
				}
				m.theme = m.themeList[m.themeSelected]
				m.progress = progress.New(
					progress.WithWidth(34),
					progress.WithoutPercentage(),
					progress.WithColors(lipgloss.Color(m.theme.Accent), lipgloss.Color(m.theme.Secondary)),
					progress.WithFillCharacters('━', '─'),
				)
				return m, nil
			case "enter":
				_ = theme.SaveTheme(m.theme.ID)
				m.setView(m.previousView)
				m.status = fmt.Sprintf(toastThemeApplied, m.theme.Name)
				m.statusError = false
				toastCmd := m.triggerToast(fmt.Sprintf(toastThemeApplied, m.theme.Name), 14)
				return m, toastCmd
			}
			return m, nil
		}

		// 3b. VIEW: BACKGROUND PICKER MODAL
		if m.currentView == viewBgPicker {
			switch key {
			case "esc":
				cfg := theme.LoadConfig()
				m.bgMode = cfg.Background
				if m.bgMode == "" {
					m.bgMode = "flow"
				}
				m.setView(m.previousView)
				return m, nil
			case "up", "k":
				if m.bgPickerSelected > 0 {
					m.bgPickerSelected--
				} else {
					m.bgPickerSelected = bgPickerOptions - 1
				}
				m.bgMode = bgPickerMode(m.bgPickerSelected)
				return m, nil
			case "down", "j":
				if m.bgPickerSelected < bgPickerOptions-1 {
					m.bgPickerSelected++
				} else {
					m.bgPickerSelected = 0
				}
				m.bgMode = bgPickerMode(m.bgPickerSelected)
				return m, nil
			case "enter":
				selectedMode := bgPickerMode(m.bgPickerSelected)
				m.bgMode = selectedMode
				_ = theme.SaveBackground(selectedMode)
				m.setView(m.previousView)
				m.status = ""
				toastCmd := m.triggerToast(fmt.Sprintf(toastBackgroundSet, backgroundLabel(selectedMode)), 14)
				return m, toastCmd
			}
			return m, nil
		}

		// 3c. VIEW: UPDATE PROMPT MODAL
		if m.currentView == viewUpdatePrompt {
			switch key {
			case "esc", "n", "N":
				target := m.previousView
				if target == viewUpdatePrompt || target == viewChangelog {
					target = viewPlaylists
				}
				m.setView(target)
				toastCmd := m.triggerToast(toastKeepForLater, 10)
				return m, toastCmd
			case "c", "C":
				m.setView(viewChangelog)
				return m, nil
			case "enter", "y", "Y":
				if m.updateDone {
					target := m.previousView
					if target == viewUpdatePrompt || target == viewChangelog {
						target = viewPlaylists
					}
					m.setView(target)
					return m, nil
				}
				if m.updateInProgress {
					return m, nil
				}
				if m.updateRelease == nil || m.updateRelease.TagName == "" {
					target := m.previousView
					if target == viewUpdatePrompt || target == viewChangelog {
						target = viewPlaylists
					}
					m.setView(target)
					return m, nil
				}
				m.updateInProgress = true
				m.updateError = ""
				tag := m.updateRelease.TagName
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					err := version.DownloadAndApplyUpdate(ctx, tag)
					return updateApplyMsg{err: err}
				}
			}
			return m, nil
		}

		// 3d. VIEW: CHANGELOG MODAL
		if m.currentView == viewChangelog {
			switch key {
			case "esc", "q", "enter":
				target := m.previousView
				if target == viewChangelog {
					target = viewPlaylists
				}
				m.setView(target)
				return m, nil
			case "u", "U":
				if m.updateRelease != nil && version.IsNewer(m.updateRelease.TagName, version.Current) {
					m.setView(viewUpdatePrompt)
					return m, nil
				}
				toastCmd := m.triggerToast(fmt.Sprintf(toastUpToDate, version.Current), 12)
				return m, toastCmd
			case "up", "k":
				if m.changelogScroll > 0 {
					m.changelogScroll--
				}
				return m, nil
			case "down", "j":
				m.changelogScroll++
				return m, nil
			}
			return m, nil
		}

		// 4. VIEW: DEVICES SELECTOR
		if m.currentView == viewDevices {
			switch key {
			case "esc", "d":
				m.setView(m.previousView)
				return m, nil
			case "j", "down":
				if m.selected < len(m.devices)-1 {
					m.selected++
				}
				return m, nil
			case "k", "up":
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			case "enter":
				if len(m.devices) > 0 {
					device := m.devices[m.selected]
					if repeated || m.transportBusy {
						return m, nil
					}
					if device.IsRestricted || device.ID == "" {
						m.status = device.Name + " no permite transferir la reproducción"
						m.statusError = true
						return m, nil
					}
					m.setView(m.previousView)
					if device.IsActive {
						m.status = device.Name + " ya está activo"
						return m, nil
					}
					m.transportBusy = true
					m.stateEpoch++
					m.status = "Llevando la música a " + device.Name + "…"
					m.statusError = false
					m.pendingDeviceID = device.ID
					m.pendingDeviceName = device.Name
					m.pendingDeviceSince = time.Now()

					var prevVol *int
					if m.state.Device != nil && m.state.Device.VolumePercent != nil {
						prevVol = m.state.Device.VolumePercent
					}

					return m, m.act("transferir", func(ctx context.Context) error {
						if err := m.client.Transfer(ctx, device); err != nil {
							return err
						}
						if prevVol != nil && *prevVol > 0 {
							_ = m.client.SetVolume(ctx, *prevVol, device.ID)
						}
						return nil
					})
				}
				return m, nil
			}
			return m, nil
		}

		// 5. VIEW: TRACKS (INSIDE PLAYLIST)
		if m.currentView == viewTracks {
			switch key {
			case "esc":
				m.setView(viewPlaylists)
				m.entriesSeq++
				return m, nil
			case "j", "down":
				if m.entryPick < len(m.entries)-1 {
					m.entryPick++
				}
				if m.entriesMore && !m.entriesBusy && len(m.entries)-m.entryPick < 8 {
					m.entriesBusy = true
					return m, m.fetchEntries(m.entriesOffset)
				}
				return m, nil
			case "k", "up":
				if m.entryPick > 0 {
					m.entryPick--
				}
				return m, nil
			case "enter":
				if !repeated && len(m.entries) > 0 {
					if m.entryPick >= len(m.entries) {
						m.entryPick = len(m.entries) - 1
					} else if m.entryPick < 0 {
						m.entryPick = 0
					}
					return m.playTrack(m.entries[m.entryPick].Track)
				}
				return m, nil
			case "p":
				if !repeated {
					return m.playPlaylist(0)
				}
				return m, nil
			case "r":
				delete(m.entriesCache, m.activePlaylist.ID)
				m.entries, m.entryPick, m.entriesOffset = nil, 0, 0
				m.entriesSeq++
				m.entriesBusy = true
				return m, m.fetchEntries(0)
			}
		}

		// 6. VIEW: PLAYLISTS LIST
		if m.currentView == viewPlaylists {
			switch key {
			case "j", "down":
				if m.playlistPick < len(m.playlists)-1 {
					m.playlistPick++
				}
				if m.playlistMore && !m.playlistBusy && len(m.playlists)-m.playlistPick < 8 {
					m.playlistBusy = true
					return m, m.fetchPlaylists(len(m.playlists))
				}
				return m, nil
			case "k", "up":
				if m.playlistPick > 0 {
					m.playlistPick--
				}
				return m, nil
			case "enter":
				if len(m.playlists) > 0 {
					if m.playlistPick >= len(m.playlists) {
						m.playlistPick = len(m.playlists) - 1
					} else if m.playlistPick < 0 {
						m.playlistPick = 0
					}
					m.activePlaylist = m.playlists[m.playlistPick]
					m.setView(viewTracks)
					if cached, ok := m.entriesCache[m.activePlaylist.ID]; ok && len(cached) > 0 {
						m.entries = cached
						m.entryPick = 0
						m.entriesOffset = len(cached)
						m.entriesBusy = false
						return m, nil
					}
					m.entries, m.entryPick, m.entriesOffset = nil, 0, 0
					m.entriesSeq++
					m.entriesBusy = true
					return m, m.fetchEntries(0)
				}
				return m, nil
			case "p":
				if !repeated && len(m.playlists) > 0 && m.playlistPick >= 0 && m.playlistPick < len(m.playlists) {
					m.activePlaylist = m.playlists[m.playlistPick]
					return m.playPlaylist(0)
				}
				return m, nil
			case "l":
				m.playlists, m.playlistPick = nil, 0
				m.playlistSeq++
				m.playlistBusy = true
				return m, m.fetchPlaylists(0)
			}
		}

		// 6.5. VIEW: COVER ART (BIG GALLERY MODE)
		if m.currentView == viewCoverArt {
			switch key {
			case "esc", "enter", "z", "c":
				m.setView(viewPlaylists)
				toastCmd := m.triggerToast(toastPlaylists, 12)
				return m, toastCmd
			}
		}

		// 7. GLOBAL CONTROLS
		switch key {
		case "q":
			return m, tea.Quit
		case "z", "c":
			if m.currentView == viewCoverArt {
				m.setView(viewPlaylists)
				toastCmd := m.triggerToast(toastPlaylists, 12)
				return m, toastCmd
			}
			m.setView(viewCoverArt)
			toastCmd := m.triggerToast(toastZen, 14)
			return m, toastCmd
		case "/":
			m.commandActive = true
			m.commandInput = ""
			m.commandMatches = FilterCommands("")
			m.commandSelect = 0
			return m, nil
		case "?", "h":
			m.commandActive = true
			m.commandInput = ""
			m.commandMatches = FilterCommands("")
			m.commandSelect = 0
			return m, nil
		case "t":
			m.themeList = theme.All()
			for idx, th := range m.themeList {
				if th.ID == m.theme.ID {
					m.themeSelected = idx
					break
				}
			}
			m.setView(viewThemePicker)
			return m, nil
		case "m", "M":
			if m.state.Device == nil {
				return m, nil
			}
			current := 50
			if m.volumeOverride != nil {
				current = *m.volumeOverride
			} else if m.state.Device.VolumePercent != nil {
				current = *m.state.Device.VolumePercent
			}
			var next int
			var toastMsg string
			if current > 0 {
				m.mutePreviousVol = &current
				next = 0
				toastMsg = "🔇 Mute (silenciado)"
			} else {
				restore := 50
				if m.mutePreviousVol != nil && *m.mutePreviousVol > 0 {
					restore = *m.mutePreviousVol
				}
				next = restore
				toastMsg = fmt.Sprintf("🔊 Sonido restaurado (%d%%)", next)
			}
			m.volumeOverride = &next
			m.volumeSeq++
			m.stateEpoch++
			seq := m.volumeSeq
			toastCmd := m.triggerToast(toastMsg, 12)
			return m, tea.Batch(tea.Tick(280*time.Millisecond, func(time.Time) tea.Msg { return volumeDueMsg{seq: seq} }), toastCmd)
		case "S", "shift+s", "s":
			m.setView(viewSearch)
			m.searchInput = ""
			m.searchResults = nil
			m.searchSelected = 0
			m.status = "Escribe una canción o artista y pulsa Enter ♪"
			m.statusError = false
			return m, nil
		case "d":
			m.setView(viewDevices)
			return m, m.fetchDevices()
		case " ", "space":
			if repeated || m.transportBusy || time.Since(m.lastTransport) < 200*time.Millisecond {
				return m, nil
			}
			if !m.state.Available {
				m.status = "Elige una canción para empezar ♪"
				return m, nil
			}
			m.transportBusy = true
			m.lastTransport = time.Now()
			m.stateEpoch++
			if m.state.IsPlaying {
				m.snapshotPlaybackPosition()
				value := false
				m.desiredPlaying = &value
				m.desiredSince = time.Now()
				m.status = ""
				m.statusError = false
				toastCmd := m.triggerToast(toastPaused, 10)
				return m, tea.Batch(m.act("pausar", m.client.Pause), toastCmd)
			}
			value := true
			m.desiredPlaying = &value
			m.desiredSince = time.Now()
			m.status = ""
			m.statusError = false
			toastCmd := m.triggerToast(toastPlaying, 10)
			return m, tea.Batch(m.act("reproducir", m.client.Play), toastCmd)
		case "right", "n":
			if repeated || m.transportBusy {
				return m, nil
			}
			m.transportBusy = true
			m.stateEpoch++
			m.status = statusNext
			m.statusError = false
			toastCmd := m.triggerToast(toastNext, 12)
			return m, tea.Batch(m.act("siguiente", m.client.Next), toastCmd)
		case "left", "b":
			if repeated || m.transportBusy {
				return m, nil
			}
			m.transportBusy = true
			m.stateEpoch++
			m.status = statusPrev
			m.statusError = false
			toastCmd := m.triggerToast(toastPrev, 12)
			return m, tea.Batch(m.act("anterior", m.client.Previous), toastCmd)
		case "+", "=":
			if m.state.Device == nil {
				return m, nil
			}
			current := 50
			if m.volumeOverride != nil {
				current = *m.volumeOverride
			} else if m.state.Device.VolumePercent != nil {
				current = *m.state.Device.VolumePercent
			}
			next := min(100, current+5)
			m.volumeOverride = &next
			m.volumeSeq++
			m.stateEpoch++
			m.status = fmt.Sprintf("Volumen al %d%%", next)
			seq := m.volumeSeq
			toastCmd := m.triggerToast(volumeToast(next), 12)
			return m, tea.Batch(tea.Tick(280*time.Millisecond, func(time.Time) tea.Msg { return volumeDueMsg{seq: seq} }), toastCmd)
		case "-", "_":
			if m.state.Device == nil {
				return m, nil
			}
			current := 50
			if m.volumeOverride != nil {
				current = *m.volumeOverride
			} else if m.state.Device.VolumePercent != nil {
				current = *m.state.Device.VolumePercent
			}
			next := max(0, current-5)
			m.volumeOverride = &next
			m.volumeSeq++
			m.stateEpoch++
			m.status = fmt.Sprintf("Volumen al %d%%", next)
			seq := m.volumeSeq
			toastCmd := m.triggerToast(volumeToast(next), 12)
			return m, tea.Batch(tea.Tick(280*time.Millisecond, func(time.Time) tea.Msg { return volumeDueMsg{seq: seq} }), toastCmd)
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	width, height := max(20, m.width), max(10, m.height)
	fit := func(s string) string {
		return lipgloss.PlaceHorizontal(width, lipgloss.Left, ansi.Truncate(s, width, "…"))
	}

	if m.currentView == viewIntro {
		bannerLines := RenderBeamIntro(m.theme, m.introFrame, width, height)
		lines := make([]string, 0, height)
		logoH := len(bannerLines)
		padTop := max(1, (height-logoH)/2)
		for i := 0; i < padTop; i++ {
			lines = append(lines, fit(""))
		}
		for _, bl := range bannerLines {
			w := ansi.StringWidth(bl)
			padLeft := max(0, (width-w)/2)
			lines = append(lines, fit(strings.Repeat(" ", padLeft)+bl))
		}
		targetHeight := max(1, height-1)
		for len(lines) < targetHeight {
			lines = append(lines, fit(""))
		}
		if len(lines) > targetHeight {
			lines = lines[:targetHeight]
		}
		for i := range lines {
			t := float64(i) / float64(max(1, len(lines)-1))
			bgHex := LerpHex(m.theme.Accent, m.theme.BgBase, t*1.2)
			lines[i] = ApplyRowBackground(lines[i], bgHex)
		}
		view := tea.NewView(strings.Join(lines, "\n"))
		view.AltScreen = true
		view.ReportFocus = true
		return view
	}

	if m.currentView == viewCoverArt {
		return m.renderCoverArtZenView(width, height, fit)
	}

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent)).Bold(true)
	secondary := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Secondary)).Bold(true)
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Text)).Bold(true)
	playingColor := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Playing)).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Dim))
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Warning))
	errColor := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Error))
	cardBorder := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Border))
	waveTopColor := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.WaveTop))
	waveBotColor := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.WaveBot))

	lines := make([]string, 0, height)

	// Top Bar: Dynamic alert / status / task indicator (Text on left, animation on right)
	taskIndicator := ""
	if m.statusError && m.status != "" {
		taskIndicator = errColor.Render("! " + m.status)
	} else if m.toastMessage != "" {
		// Eye-catching floating notification pill badge
		toastFg := "#0c0d0e"
		if m.theme.IsLight() {
			toastFg = "#ffffff"
		}
		toastPill := lipgloss.NewStyle().
			Foreground(lipgloss.Color(toastFg)).
			Background(lipgloss.Color(m.theme.Accent)).
			Bold(true).
			Padding(0, 1).
			Render(m.toastMessage)
		taskIndicator = toastPill
	} else if m.playlistBusy {
		taskIndicator = warning.Render("Cargando playlists…")
	} else if m.entriesBusy {
		taskIndicator = warning.Render("Cargando canciones…")
	} else if m.transportBusy {
		taskIndicator = warning.Render("Esperando a Spotify…")
	} else if m.pendingDeviceID != "" && time.Since(m.pendingDeviceSince) <= 2500*time.Millisecond {
		taskIndicator = warning.Render("Conectando…")
	} else if m.currentView == viewSearch && len(m.searchInput) > 0 {
		taskIndicator = secondary.Render("Buscando…")
	}

	var devVol *int
	if m.state.Device != nil && m.state.Device.VolumePercent != nil {
		devVol = m.state.Device.VolumePercent
	}
	devWidget := secondary.Render(formatHeaderDeviceBadge(m.state.Device))
	volWidget := secondary.Render(formatVolumeWidget(devVol, m.volumeOverride))
	rightHeader := devWidget + "   " + volWidget + "  "

	leftHeader := "  "
	if taskIndicator != "" {
		leftHeader = "  " + taskIndicator
	}

	avail := width - ansi.StringWidth(rightHeader) - 1
	if avail > 0 && ansi.StringWidth(leftHeader) > avail {
		leftHeader = ansi.Truncate(leftHeader, avail, "…")
	}

	headerSpace := width - ansi.StringWidth(leftHeader) - ansi.StringWidth(rightHeader)
	var headerLine string
	if headerSpace > 0 {
		headerLine = leftHeader + strings.Repeat(" ", headerSpace) + rightHeader
	} else {
		headerLine = leftHeader
	}

	// Vertical breathing room from terminal top edge (so indicators aren't glued to the title bar)
	if height >= 16 {
		lines = append(lines, fit(""))
	}
	lines = append(lines, fit(headerLine))

	// Card Top Border
	cardTop := cardBorder.Render("╭" + strings.Repeat("─", max(0, width-2)) + "╮")
	lines = append(lines, fit(cardTop))

	innerW := max(20, width-4)

	if height >= 22 {
		// 5-line Card Layout (with Album Cover on the left + Album/Year metadata)
		coverAreaW := 12 // 10 chars cover + 2 spaces
		rightW := max(10, innerW-coverAreaW)

		var coverLines []string
		if m.coverData != nil && len(m.coverData.Lines) == 5 {
			coverLines = m.coverData.Lines
		} else {
			coverLines = []string{
				muted.Render("┌────────┐"),
				muted.Render("│   ♫    │"),
				muted.Render("│ SPOTIFY│"),
				muted.Render("│   ♪    │"),
				muted.Render("└────────┘"),
			}
		}

		var rightLines [5]string
		if m.state.Item != nil {
			// Row 0: Title
			rightLines[0] = bright.Render(m.state.Item.Name)

			// Row 1: Artists
			var names []string
			for _, a := range m.state.Item.Artists {
				names = append(names, a.Name)
			}
			artistStr := ""
			if len(names) > 0 {
				artistStr = strings.Join(names, ", ")
			}
			rightLines[1] = secondary.Render(artistStr)

			// Row 2: Album & Year
			albumYear := ""
			if m.state.Item.Album.Name != "" {
				albumYear = "💿 " + m.state.Item.Album.Name
				if yr := m.state.Item.ReleaseYear(); yr != "" {
					albumYear += " · " + yr
				}
			}
			rightLines[2] = muted.Render(albumYear)

			// Row 3: Play/Pause Icon + Fine Progress Bar
			position := m.currentPositionMS()
			position = min(position, m.state.Item.DurationMS)
			fraction := 0.0
			if m.state.Item.DurationMS > 0 {
				fraction = float64(position) / float64(m.state.Item.DurationMS)
			}
			var statusIcon string
			if m.state.IsPlaying {
				statusIcon = accent.Render("▶") + " "
			} else {
				statusIcon = secondary.Render("⏸") + "  "
			}
			currStr := muted.Render(duration(position))
			totStr := muted.Render(duration(m.state.Item.DurationMS))

			// Row 3: Vertical breathing room between album and progress bar
			rightLines[3] = ""

			// Row 4: Play/Pause Icon + Fine Progress Bar
			barW := max(6, min(48, rightW-ansi.StringWidth(statusIcon)-ansi.StringWidth(currStr)-ansi.StringWidth(totStr)-4))
			fineBar := renderFineProgressBar(barW, fraction, m.state.IsPlaying, m.theme)
			rightLines[4] = statusIcon + currStr + " " + fineBar + " " + totStr
		} else {
			rightLines[0] = bright.Render("SpotifyGo")
			rightLines[1] = muted.Render("Listo para reproducir música")
			rightLines[2] = muted.Render("Selecciona una playlist para comenzar")
			rightLines[3] = ""
			rightLines[4] = muted.Render("────────────────────────")
		}

		for i := 0; i < 5; i++ {
			cPart := coverLines[i] + "  "
			rPart := ansi.Truncate(rightLines[i], rightW, "…")
			pad := max(0, rightW-ansi.StringWidth(rPart))
			lines = append(lines, fit(cardRow(cPart+rPart+strings.Repeat(" ", pad), width, cardBorder)))
		}
	} else {
		// Compact 2-line Card Layout (for tight terminal heights)
		waveW := 11
		var waveLine1, waveLine2 string
		if m.state.IsPlaying {
			waveLine1 = waveTopColor.Render(" ▂ ▄ ▆ ▄ ") + "  "
			waveLine2 = waveBotColor.Render(" ▄ ▆ ▄ ▂ ") + "  "
		} else if m.state.Item != nil {
			waveLine1 = muted.Render(" ▂ ▂ ▂ ▂ ") + "  "
			waveLine2 = muted.Render(" ▂ ▂ ▂ ▂ ") + "  "
		} else {
			waveLine1 = muted.Render(" ─ ─ ─ ─ ") + "  "
			waveLine2 = muted.Render(" ─ ─ ─ ─ ") + "  "
		}

		rightW := max(10, innerW-waveW)

		var titleStr string
		if m.state.Item != nil {
			var names []string
			for _, a := range m.state.Item.Artists {
				names = append(names, a.Name)
			}
			artistText := ""
			if len(names) > 0 {
				artistText = secondary.Render(" — " + strings.Join(names, ", "))
			}
			albumText := ""
			if m.state.Item.Album.Name != "" {
				albumText = " 💿 " + m.state.Item.Album.Name
				if yr := m.state.Item.ReleaseYear(); yr != "" {
					albumText += " · " + yr
				}
				albumText = muted.Render(albumText)
			}
			titleStr = bright.Render(m.state.Item.Name) + artistText + albumText
		} else {
			titleStr = bright.Render("SpotifyGo") + muted.Render(" — Listo para reproducir")
		}
		titleW := ansi.StringWidth(titleStr)

		var progStr string
		var progW int
		if m.state.Item != nil {
			position := m.currentPositionMS()
			position = min(position, m.state.Item.DurationMS)
			fraction := 0.0
			if m.state.Item.DurationMS > 0 {
				fraction = float64(position) / float64(m.state.Item.DurationMS)
			}
			prog := m.progress
			barW := max(10, min(44, rightW-16))
			prog.SetWidth(barW)
			currStr := muted.Render(duration(position))
			totStr := muted.Render(duration(m.state.Item.DurationMS))
			progStr = currStr + " " + prog.ViewAs(fraction) + " " + totStr
			progW = ansi.StringWidth(progStr)
		}

		padTitleLeft := max(0, (rightW-titleW)/2)
		padTitleRight := max(0, rightW-titleW-padTitleLeft)
		var titleCell string
		if titleW > rightW {
			titleCell = ansi.Truncate(titleStr, rightW, "…")
		} else {
			titleCell = strings.Repeat(" ", padTitleLeft) + titleStr + strings.Repeat(" ", padTitleRight)
		}

		padProgLeft := max(0, (rightW-progW)/2)
		padProgRight := max(0, rightW-progW-padProgLeft)
		var progCell string
		if progW > rightW {
			progCell = ansi.Truncate(progStr, rightW, "…")
		} else {
			progCell = strings.Repeat(" ", padProgLeft) + progStr + strings.Repeat(" ", padProgRight)
		}

		lines = append(lines, fit(cardRow(waveLine1+titleCell, width, cardBorder)))
		lines = append(lines, fit(cardRow(waveLine2+progCell, width, cardBorder)))
	}

	// Card Bottom Border
	cardBottom := cardBorder.Render("╰" + strings.Repeat("─", max(0, width-2)) + "╯")
	lines = append(lines, fit(cardBottom))

	// Body Panels
	body := make([]string, 0)

	switch m.currentView {
	case viewTracks:
		body = append(body, " "+accent.Render("📁 "+m.activePlaylist.Name)+" "+muted.Render(fmt.Sprintf("(%d canciones)", len(m.entries))))
		if !m.entriesBusy && len(m.entries) == 0 && !m.statusError {
			body = append(body, " "+muted.Render("Sin canciones"))
		} else if len(m.entries) > 0 {
			avail := max(20, width-24)
			titleWidth := int(float64(avail) * 0.55)
			artistWidth := avail - titleWidth
			body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))

			rows := max(0, height-len(lines)-4)
			start := listStart(m.entryPick, len(m.entries), rows)
			for i := start; i < len(m.entries) && i < start+rows; i++ {
				entry := m.entries[i]
				isCurrent := m.state.Item != nil && m.state.Item.URI == entry.Track.URI

				var artists []string
				for _, a := range entry.Track.Artists {
					artists = append(artists, a.Name)
				}
				artistStr := strings.Join(artists, ", ")

				trackName := ansi.Truncate(entry.Track.Name, titleWidth-1, "…")
				artistName := ansi.Truncate(artistStr, artistWidth-1, "…")
				durStr := duration(entry.Track.DurationMS)

				prefix := "   "
				if i == m.entryPick {
					prefix = " › "
				}
				nowIcon := "  "
				if isCurrent {
					nowIcon = "♫ "
				}

				rowStr := fmt.Sprintf("%s%-4d %-*s %-*s  %6s %s", prefix, entry.Position+1, titleWidth, trackName, artistWidth, artistName, durStr, nowIcon)
				var rowFormatted string
				if isCurrent {
					rowFormatted = playingColor.Render(rowStr)
				} else if i == m.entryPick {
					rowFormatted = bright.Render(rowStr)
				} else {
					rowFormatted = muted.Render(rowStr)
				}
				body = append(body, rowFormatted)
			}
		}

	case viewDevices:
		body = append(body, " "+accent.Render("💻 DISPOSITIVOS DISPONIBLES")+" "+muted.Render(fmt.Sprintf("(%d)", len(m.devices))))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		for i, item := range m.devices {
			prefix, style := "  ", muted
			if i == m.selected {
				prefix, style = "› ", bright
			}
			badge := formatDeviceBadge(&item)
			if item.IsActive {
				badge += "  " + accent.Render("● ACTIVO")
			}
			body = append(body, " "+style.Render(prefix+badge))
		}

	case viewSearch:
		body = append(body, " "+secondary.Render("🔍 BUSCAR EN SPOTIFY"))
		body = append(body, " "+secondary.Render("S: "+m.searchInput+"▌"))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		if len(m.searchResults) == 0 {
			if strings.TrimSpace(m.searchInput) == "" {
				body = append(body, " "+muted.Render("Escribe canción o artista y pulsa Enter"))
			} else if !m.entriesBusy {
				body = append(body, " "+muted.Render("Pulsa Enter para buscar '"+m.searchInput+"'"))
			}
		} else {
			avail := max(20, width-14)
			titleWidth := int(float64(avail) * 0.50)
			artistWidth := avail - titleWidth
			for i, item := range m.searchResults {
				prefix, style := "   ", muted
				if i == m.searchSelected {
					prefix, style = " › ", bright
				}
				var artists []string
				for _, a := range item.Artists {
					artists = append(artists, a.Name)
				}
				artistStr := strings.Join(artists, ", ")
				tName := ansi.Truncate(item.Name, titleWidth-1, "…")
				aName := ansi.Truncate(artistStr, artistWidth-1, "…")
				durStr := duration(item.DurationMS)
				rowStr := fmt.Sprintf("%s%-*s %-*s  %5s", prefix, titleWidth, tName, artistWidth, aName, durStr)
				body = append(body, " "+style.Render(rowStr))
			}
		}

	case viewThemePicker:
		body = append(body, " "+accent.Render("🎨 SELECCIONA UN TEMA")+" "+muted.Render(fmt.Sprintf("(%d)", len(m.themeList))))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		body = append(body, "")
		for i, t := range m.themeList {
			prefix := "   "
			if i == m.themeSelected {
				prefix = " › "
				themeCard := accent.Render(prefix+"[●] "+t.Name) + "  " + bright.Render(t.Description)
				body = append(body, " "+themeCard)
			} else {
				themeCard := muted.Render(prefix+"[ ] "+t.Name) + "  " + dim.Render(t.Description)
				body = append(body, " "+themeCard)
			}
		}

	case viewBgPicker:
		body = append(body, " "+accent.Render("🎨 ESTILO DE FONDO (BACKGROUND)")+" "+muted.Render(fmt.Sprintf("(%d opciones)", bgPickerOptions)))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		body = append(body, " "+muted.Render("Selecciona el estilo de fondo para la terminal:"))
		body = append(body, " "+dim.Render("La interfaz normal es estática · flow solo se anima en modo Zen"))
		body = append(body, "")

		bgOptions := []struct {
			Name        string
			Description string
		}{
			{"flow", "— Flow animado solo en Zen; degradado estático en modo normal (por defecto)"},
			{"gradient", "— Degradado vertical con la carátula, sin animación"},
			{"dark", "— Fondo sólido del tema (" + m.theme.Name + "), único modo que lo usa"},
		}
		for i, opt := range bgOptions {
			radio, prefix, nameStyle, descStyle := "[ ]", "   ", muted, dim
			if m.bgPickerSelected == i {
				radio, prefix, nameStyle, descStyle = "[●]", " › ", bright, secondary
			}
			body = append(body, " "+accent.Render(prefix+radio)+" "+nameStyle.Render(fmt.Sprintf("%-9s", opt.Name))+" "+descStyle.Render(opt.Description))
		}

	case viewUpdatePrompt:
		body = append(body, " "+accent.Render("🚀 ACTUALIZACIÓN DISPONIBLE"))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		body = append(body, "")

		tag := "v1.0.3"
		if m.updateRelease != nil && m.updateRelease.TagName != "" {
			tag = m.updateRelease.TagName
		}

		if m.updateInProgress {
			body = append(body, " "+bright.Render("⏳ Descargando e instalando actualización "+tag+"…"))
			body = append(body, " "+muted.Render("Por favor espera unos segundos mientras se reemplaza el binario."))
			body = append(body, "")
			body = append(body, " "+dim.Render("Descarga directa desde GitHub Releases…"))
		} else if m.updateDone {
			body = append(body, " "+accent.Render("🎉 ¡SpotifyGo se actualizó con éxito a "+tag+"!"))
			body = append(body, "")
			body = append(body, " "+bright.Render("La próxima vez que abras SpotifyGo disfrutarás de todas las mejoras."))
			body = append(body, " "+muted.Render("Pulsa Enter o Esc para continuar usando la aplicación."))
		} else if m.updateError != "" {
			body = append(body, " "+errColor.Render("! Error al descargar la actualización automática:"))
			body = append(body, " "+muted.Render(m.updateError))
			body = append(body, "")
			body = append(body, " "+bright.Render("Puedes actualizar manualmente ejecutando en tu terminal:"))
			body = append(body, " "+accent.Render("  spotifygo update"))
		} else {
			body = append(body, " "+bright.Render("Se ha publicado una nueva versión:")+" "+accent.Render(tag)+" "+muted.Render("(instalada: "+version.Current+")"))
			body = append(body, "")
			body = append(body, " "+bright.Render("¿Deseas descargar e instalar la actualización ahora mismo?"))
			body = append(body, " "+muted.Render("La actualización se aplica en segundos y mantiene tu sesión y configuración intactas."))
		}

	case viewChangelog:
		body = append(body, " "+accent.Render("✨ NOVEDADES Y CAMBIOS RECIENTES")+" "+muted.Render("(SpotifyGo "+version.Current+")"))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		body = append(body, "")

		highlights := version.RecentHighlights()
		for _, h := range highlights {
			badge := accent.Render("● "+h.Version) + " " + dim.Render("· "+h.Tagline)
			body = append(body, " "+badge)
			for _, pt := range h.Points {
				body = append(body, "   "+muted.Render("· "+pt))
			}
			body = append(body, "")
		}

	case viewPlaylists:
		fallthrough
	default:
		body = append(body, " "+accent.Render("📁 TUS PLAYLISTS")+" "+muted.Render(fmt.Sprintf("(%d)", len(m.playlists))))
		body = append(body, dim.Render(" "+strings.Repeat("─", max(10, width-2))))
		if !m.playlistBusy && len(m.playlists) == 0 && !m.statusError {
			body = append(body, " "+muted.Render("Aún no hay playlists por aquí"))
		}
		rows := max(0, height-len(lines)-5)
		start := listStart(m.playlistPick, len(m.playlists), rows)
		for i := start; i < len(m.playlists) && i < start+rows; i++ {
			p := m.playlists[i]
			displayName := strings.TrimSpace(p.Name)
			displayName = strings.TrimPrefix(displayName, "♥")
			displayName = strings.TrimSpace(displayName)
			icon := "• "
			if strings.Contains(p.Name, "Canciones que te gustan") || p.ID == spotify.LikedTracksID {
				icon = "♥ "
			} else if strings.Contains(p.Name, "Radio") || strings.Contains(p.Name, "Mix") {
				icon = "📻 "
			}
			isPick := (i == m.playlistPick)
			var rowText string
			if isPick {
				rowText = bright.Render("› " + icon + displayName)
			} else {
				rowText = muted.Render("  " + icon + displayName)
			}
			body = append(body, " "+rowText)
		}
	}

	// Command Mode Floating Box (if active)
	if m.commandActive {
		body = nil
		body = append(body, "")

		boxBorder := cardBorder
		boxWidth := min(76, max(44, width-4))
		padLeft := "  "
		innerW := boxWidth - 4 // content width between "│ " and " │"

		// 1. Top border: "╭─ COMMANDS ────────────────────────╮"
		title := " COMMANDS "
		dashCount := max(0, boxWidth-2-1-len(title)) // -2 for "╭─", -1 for "╮"
		topLine := padLeft + boxBorder.Render("╭─"+bright.Bold(true).Render(title)+strings.Repeat("─", dashCount)+"╮")
		body = append(body, topLine)

		// 2. Search input row: "│ / input▌                          │"
		prompt := accent.Render("/ ") + bright.Render(m.commandInput) + accent.Render("▌")
		promptW := 2 + len(m.commandInput) + 1
		padPrompt := max(0, innerW-promptW)
		inputLine := padLeft + boxBorder.Render("│ ") + prompt + strings.Repeat(" ", padPrompt) + boxBorder.Render(" │")
		body = append(body, inputLine)

		// 3. Middle divider: "├───────────────────────────────────┤"
		midLine := padLeft + boxBorder.Render("├"+strings.Repeat("─", boxWidth-2)+"┤")
		body = append(body, midLine)

		// 4. Commands list with scroll viewport
		if len(m.commandMatches) == 0 {
			emptyText := dim.Render("Sin coincidencias: prueba background, login, update, changelog, version o help")
			padEmpty := max(0, innerW-ansi.StringWidth(emptyText))
			emptyLine := padLeft + boxBorder.Render("│ ") + emptyText + strings.Repeat(" ", padEmpty) + boxBorder.Render(" │")
			body = append(body, emptyLine)
		} else {
			availableRows := max(4, min(8, height-len(lines)-8))
			total := len(m.commandMatches)
			start := listStart(m.commandSelect, total, availableRows)
			end := min(total, start+availableRows)

			for i := start; i < end; i++ {
				cmd := m.commandMatches[i]
				prefix := "   "
				nameStyle := secondary
				descStyle := muted
				if i == m.commandSelect {
					prefix = " › "
					nameStyle = bright.Bold(true)
					descStyle = bright
				}

				aliasStr := ""
				if len(cmd.Aliases) > 0 {
					aliasStr = " (" + strings.Join(cmd.Aliases, ", ") + ")"
				}
				fullName := cmd.Name + aliasStr
				nameColW := 22
				nameCol := fmt.Sprintf("%-22s", fullName)
				if ansi.StringWidth(nameCol) > nameColW {
					nameCol = ansi.Truncate(nameCol, nameColW, "…")
				}

				maxDescW := max(10, innerW-3-nameColW-1)
				descCol := ansi.Truncate(cmd.Description, maxDescW, "…")

				rowContent := prefix + nameStyle.Render(nameCol) + " " + descStyle.Render(descCol)
				contentW := ansi.StringWidth(rowContent)
				padRow := max(0, innerW-contentW)
				itemLine := padLeft + boxBorder.Render("│ ") + rowContent + strings.Repeat(" ", padRow) + boxBorder.Render(" │")
				body = append(body, itemLine)
			}
		}

		// 5. Bottom border with scroll indicator / total count
		total := len(m.commandMatches)
		var bottomLine string
		if total > 0 {
			indicator := fmt.Sprintf(" %d/%d ", m.commandSelect+1, total)
			indW := len(indicator)
			dashesLeft := 2
			dashesRight := max(0, boxWidth-2-dashesLeft-indW)
			bottomLine = padLeft + boxBorder.Render("╰"+strings.Repeat("─", dashesLeft)) + muted.Render(indicator) + boxBorder.Render(strings.Repeat("─", dashesRight)+"╯")
		} else {
			bottomLine = padLeft + boxBorder.Render("╰"+strings.Repeat("─", boxWidth-2)+"╯")
		}
		body = append(body, bottomLine)
		body = append(body, padLeft+" "+muted.Render("↑/↓: navigate · Enter: select · Esc: cancel"))
	}

	targetHeight := max(1, height-1)
	hasFooter := height >= 14 && !m.commandActive
	bodyLimit := targetHeight
	if hasFooter {
		bodyLimit = targetHeight - 1
	}

	for i := 0; len(lines) < bodyLimit; i++ {
		if i < len(body) {
			lines = append(lines, fit(body[i]))
		} else {
			lines = append(lines, fit(""))
		}
	}
	if len(lines) > bodyLimit {
		lines = lines[:bodyLimit]
	}

	if hasFooter {
		lines = append(lines, fit(m.renderFooter(width)))
	}

	// Normal mode stays static. The saved "flow" choice is animated only in Zen.
	canvasMode := m.bgMode
	if canvasMode == "flow" {
		canvasMode = "gradient"
	}
	canvas := NewCanvas(canvasMode, m.coverData, m.theme.BgBase, m.theme.Text, m.theme.Muted, 0)
	lines = canvas.PaintLines(lines)

	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.ReportFocus = true
	return view
}

func (m Model) renderFooter(width int) string {
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Dim))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.PillKeyFg)).Background(lipgloss.Color(m.theme.PillKeyBg)).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Muted))
	sep := dim.Render(" · ")

	pill := func(key, desc string) string {
		return keyStyle.Render(" "+key+" ") + " " + descStyle.Render(desc)
	}

	var items []string
	switch m.currentView {
	case viewTracks:
		items = []string{
			pill("Enter", "Tocar"),
			pill("p", "Playlist"),
			pill("Esc", "Volver"),
			pill("Space", "Play/Pausa"),
			pill("n/b", "Sig/Ant"),
			pill("r", "Recargar"),
			pill("q", "Salir"),
		}
	case viewSearch:
		items = []string{
			pill("Enter", "Buscar/Tocar"),
			pill("↑/↓", "Navegar"),
			pill("Esc", "Volver"),
			pill("q", "Salir"),
		}
	case viewDevices:
		items = []string{
			pill("Enter", "Transferir"),
			pill("↑/↓", "Elegir"),
			pill("Esc", "Volver"),
			pill("q", "Salir"),
		}
	case viewThemePicker:
		items = []string{
			pill("Enter", "Aplicar"),
			pill("↑/↓", "Previsualizar"),
			pill("Esc", "Cancelar"),
		}
	case viewBgPicker:
		items = []string{
			pill("Enter", "Confirmar"),
			pill("↑/↓", "Previsualizar"),
			pill("Esc", "Cancelar"),
		}
	case viewUpdatePrompt:
		if m.updateDone {
			items = []string{
				pill("Enter/Esc", "Continuar"),
				pill("q", "Salir"),
			}
		} else if m.updateInProgress {
			items = []string{
				pill("⏳", "Instalando..."),
			}
		} else if m.updateError != "" {
			items = []string{
				pill("Esc", "Cerrar"),
				pill("q", "Salir"),
			}
		} else {
			items = []string{
				pill("Enter/y", "Actualizar"),
				pill("c", "Novedades"),
				pill("Esc/n", "Omitir"),
			}
		}
	case viewChangelog:
		items = []string{
			pill("Esc/Enter", "Volver"),
			pill("↑/↓", "Desplazar"),
			pill("u", "Actualizar"),
			pill("q", "Salir"),
		}
	default:
		items = []string{
			pill("Space", "Play"),
			pill("n/b", "Skip"),
			pill("+/-", "Vol"),
			pill("m", "Mute"),
			pill("s", "Buscar"),
			pill("t", "Tema"),
			pill("d", "Disp"),
			pill("z", "Zen"),
			pill("/", "Cmds"),
			pill("q", "Salir"),
		}
	}

	result := " "
	for i, item := range items {
		candidate := result
		if i > 0 {
			candidate += sep
		}
		candidate += item
		if ansi.StringWidth(candidate) > width-2 {
			break
		}
		result = candidate
	}
	return result
}

func cardRow(content string, width int, borderStyle lipgloss.Style) string {
	innerW := max(0, width-4)
	c := ansi.Truncate(content, innerW, "…")
	padLen := max(0, innerW-ansi.StringWidth(c))
	return borderStyle.Render("│") + " " + c + strings.Repeat(" ", padLen) + " " + borderStyle.Render("│")
}

func centerText(s string, totalWidth int, style lipgloss.Style) string {
	w := ansi.StringWidth(s)
	if w >= totalWidth {
		return style.Render(s)
	}
	pad := max(0, (totalWidth-w)/2)
	return strings.Repeat(" ", pad) + style.Render(s)
}

func formatHeaderDeviceBadge(device *spotify.Device) string {
	if device == nil || device.Name == "" {
		return "🎧 Sin disp."
	}
	icon := "🎧"
	t := strings.ToLower(device.Type)
	switch {
	case strings.Contains(t, "comp") || device.Type == "Computer":
		icon = "💻"
	case strings.Contains(t, "phone") || strings.Contains(t, "smart"):
		icon = "📱"
	case strings.Contains(t, "speaker") || strings.Contains(t, "audio"):
		icon = "🔊"
	case strings.Contains(t, "tv"):
		icon = "📺"
	case strings.Contains(t, "tablet"):
		icon = "📱"
	}

	label := "PC"
	if icon == "📱" {
		label = "Móvil"
	} else if icon == "🔊" {
		label = "Altavoz"
	} else if icon == "📺" {
		label = "TV"
	} else if icon == "💻" {
		label = "PC"
	} else {
		label = "Equipo"
	}
	return fmt.Sprintf("%s %s", icon, label)
}

func formatQualityBadge(bitrate string) string {
	switch bitrate {
	case "160":
		return "MQ 160k"
	case "96":
		return "LQ 96k"
	default:
		return "HQ 320k"
	}
}

func formatDeviceBadge(device *spotify.Device) string {
	if device == nil || device.Name == "" {
		return "[ 🎧 Sin dispositivo ]"
	}
	icon := "🎧"
	t := strings.ToLower(device.Type)
	switch {
	case strings.Contains(t, "comp") || device.Type == "Computer":
		icon = "💻"
	case strings.Contains(t, "phone") || strings.Contains(t, "smart"):
		icon = "📱"
	case strings.Contains(t, "speaker") || strings.Contains(t, "audio"):
		icon = "🔊"
	case strings.Contains(t, "tv"):
		icon = "📺"
	case strings.Contains(t, "tablet"):
		icon = "📱"
	}

	name := device.Name
	if strings.HasPrefix(name, "SpotifyGo") || strings.HasPrefix(name, "SpotyGo") {
		name = "SpotifyGo (Local)"
	} else if strings.HasPrefix(name, "DESKTOP-") || strings.HasPrefix(name, "LAPTOP-") {
		name = "Esta PC"
	} else if len(name) > 16 {
		name = ansi.Truncate(name, 16, "…")
	}
	return fmt.Sprintf("[ %s %s ]", icon, name)
}

func formatVolumeWidget(deviceVol *int, override *int) string {
	v := 50
	if deviceVol != nil {
		v = *deviceVol
	}
	if override != nil {
		v = *override
	}
	v = min(100, max(0, v))

	icon := "🔊"
	switch {
	case v == 0:
		icon = "🔇"
	case v < 33:
		icon = "🔈"
	case v < 67:
		icon = "🔉"
	default:
		icon = "🔊"
	}

	filled := (v + 5) / 10
	filled = min(10, max(0, filled))
	empty := 10 - filled
	bar := strings.Repeat("━", filled) + strings.Repeat("─", empty)

	return fmt.Sprintf("%s [%s] %d%%", icon, bar, v)
}

func formatPill(key, action, keyFg, keyBg string, descStyle lipgloss.Style) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(keyFg)).Background(lipgloss.Color(keyBg)).Bold(true).Padding(0, 1)
	return keyStyle.Render("["+key+"]") + " " + descStyle.Render(action)
}

func listStart(selected, total, rows int) int {
	if rows <= 0 || total <= rows {
		return 0
	}
	return min(max(0, selected-rows/2), total-rows)
}

func duration(ms int) string {
	seconds := max(0, ms/1000)
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func renderFineProgressBar(barW int, fraction float64, isPlaying bool, th theme.Theme) string {
	if barW <= 2 {
		return ""
	}
	fraction = max(0.0, min(1.0, fraction))
	filledCount := int(fraction * float64(barW-1))
	if filledCount >= barW {
		filledCount = barW - 1
	}

	accentHex := th.Accent
	if accentHex == "" {
		accentHex = "#1DB954"
	}
	secondHex := th.Secondary
	if secondHex == "" {
		secondHex = "#1ED760"
	}
	dimHex := th.Dim
	if dimHex == "" {
		dimHex = "#475569"
	}

	knobColor := accentHex
	if !isPlaying {
		knobColor = LerpHex(accentHex, dimHex, 0.4)
	}
	knobStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(knobColor)).Bold(true)

	var sb strings.Builder

	for i := 0; i < barW; i++ {
		if i == filledCount {
			sb.WriteString(knobStyle.Render("●"))
		} else if i < filledCount {
			t := float64(i) / float64(max(1, barW))
			c := LerpHex(accentHex, secondHex, t*0.5)
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(c))
			sb.WriteString(st.Render("━"))
		} else {
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(dimHex))
			sb.WriteString(st.Render("─"))
		}
	}
	return sb.String()
}
