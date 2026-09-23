package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
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

type Model struct {
	client         *spotify.Client
	localName      string
	engineDone     <-chan error
	localReady     bool
	transferSent   bool
	transportBusy  bool
	searching      bool
	searchInput    string
	searchResults  []spotify.Track
	searchSelected int
	searchSeq      int
	showPlaylists  bool
	showTracks     bool
	playlists      []spotify.Playlist
	playlistTotal  int
	playlistMore   bool
	playlistBusy   bool
	playlistSeq    int
	playlistPick   int
	activePlaylist spotify.Playlist
	entries        []spotify.PlaylistEntry
	entriesTotal   int
	entriesMore    bool
	entriesBusy    bool
	entriesSeq     int
	entriesOffset  int
	entryPick      int
	state          spotify.PlaybackState
	devices        []spotify.Device
	showDevices    bool
	selected       int
	status         string
	width          int
	height         int
	volumeOverride *int
	volumeSeq      int
	volumeExpected *int
	volumeDeviceID string
	volumeSetAt    time.Time
	stateEpoch     int
	lastTransport  time.Time
	lastSync       time.Time
	statusError    bool
	pendingDevice  string
	pendingContext string
	pendingSince   time.Time
	desiredPlaying *bool
	desiredSince   time.Time
	spinner        spinner.Model
	progress       progress.Model
}

func New(client *spotify.Client, localName string, engineDone <-chan error) Model {
	pulse := spinner.New(spinner.WithSpinner(spinner.Spinner{
		Frames: []string{"⠁⡀⠄⠠⠂⠁⡀⠄", "⠂⣄⠆⡄⠆⠂⣄⠆", "⠆⣦⠇⣤⠇⠆⣦⠇", "⠇⣷⣿⣶⣿⠇⣷⣿", "⠆⣦⠇⣤⠇⠆⣦⠇", "⠂⣄⠆⡄⠆⠂⣄⠆"},
		FPS:    140 * time.Millisecond,
	}))
	bar := progress.New(progress.WithWidth(34), progress.WithoutPercentage(), progress.WithColors(lipgloss.Color("#1DB954"), lipgloss.Color("#5EEAD4")), progress.WithFillCharacters('=', '-'))
	return Model{client: client, localName: localName, engineDone: engineDone, width: 80, height: 24, spinner: pulse, progress: bar}
}

func (m Model) fetchState() tea.Cmd {
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
	m.showPlaylists, m.showTracks = false, false
	m.status, m.statusError = "Iniciando playlist…", false
	return m, m.act("playlist", func(ctx context.Context) error {
		return m.client.PlayPlaylist(ctx, playlist, position, device.ID)
	})
}

func poll() tea.Cmd {
	return tea.Tick(7*time.Second, func(time.Time) tea.Msg { return pollMsg{} })
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchState(), m.fetchDevices(), poll(), m.spinner.Tick, func() tea.Msg {
		return engineStoppedMsg{err: <-m.engineDone}
	})
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

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case searchMsg:
		if msg.seq != m.searchSeq || !m.searching {
			break
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
		} else {
			m.statusError = false
			m.searchResults = msg.tracks
			m.searchSelected = 0
			m.status = ""
		}
	case playlistsMsg:
		if msg.seq != m.playlistSeq || !m.showPlaylists || m.showTracks {
			break
		}
		m.playlistBusy = false
		if msg.err != nil {
			m.status, m.statusError = msg.err.Error(), true
		} else {
			if msg.offset == 0 {
				m.playlists = nil
			}
			m.playlists = append(m.playlists, msg.page.Items...)
			m.playlistTotal, m.playlistMore = msg.page.Total, msg.page.Next != ""
			m.status, m.statusError = "", false
		}
	case playlistEntriesMsg:
		if msg.seq != m.entriesSeq || !m.showTracks || msg.playlistID != m.activePlaylist.ID {
			break
		}
		m.entriesBusy = false
		if msg.err != nil {
			if strings.Contains(msg.err.Error(), "HTTP 403") {
				m.status = "Spotify limita playlists seguidas: p reproduce la playlist completa"
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
			if m.entriesMore && len(m.entries) == 0 {
				m.entriesBusy = true
				return m, m.fetchEntries(m.entriesOffset)
			}
		}
	case engineStoppedMsg:
		m.localReady = false
		m.status = fmt.Sprintf("Motor de audio detenido: %v. Revisa el registro de librespot.", msg.err)
		m.statusError = true
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.SetWidth(max(12, msg.Width-19))
	case stateMsg:
		if msg.epoch != m.stateEpoch || m.transportBusy {
			break
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
		} else {
			m.state = msg.state
			m.lastSync = time.Now()
			if m.desiredPlaying != nil {
				if m.state.IsPlaying == *m.desiredPlaying {
					m.status = ""
					m.statusError = false
					m.desiredPlaying = nil
				} else if time.Since(m.desiredSince) < 8*time.Second {
					m.state.IsPlaying = *m.desiredPlaying
				} else {
					m.desiredPlaying = nil
					m.status = "Spotify no confirmó el cambio de reproducción"
					m.statusError = true
				}
			}
			if m.volumeExpected != nil && m.state.Device != nil && m.state.Device.ID == m.volumeDeviceID {
				if m.state.Device.VolumePercent != nil && *m.state.Device.VolumePercent == *m.volumeExpected {
					m.volumeExpected = nil
					m.status = ""
				} else if time.Since(m.volumeSetAt) < 8*time.Second {
					value := *m.volumeExpected
					m.state.Device.VolumePercent = &value
				} else {
					m.volumeExpected = nil
					m.status = "Spotify aún no confirma el volumen; mostrando el valor informado por el dispositivo"
					m.statusError = true
				}
			}
			if m.pendingDevice != "" && m.state.Device != nil && m.state.Device.Name == m.pendingDevice {
				m.status = ""
				m.pendingDevice = ""
				m.statusError = false
			}
			if m.pendingContext != "" {
				if m.state.IsPlaying && m.state.Context != nil && m.state.Context.URI == m.pendingContext {
					m.pendingContext = ""
					m.status, m.statusError = "", false
				} else if time.Since(m.pendingSince) > 10*time.Second {
					m.pendingContext = ""
					m.status, m.statusError = "Spotify no confirmó la playlist", true
				}
			}
		}
	case devicesMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
		} else {
			m.devices = msg.devices
			for _, device := range m.devices {
				if device.Name == m.localName && device.ID != "" {
					m.localReady = true
					if !m.transferSent && !device.IsActive {
						m.transferSent = true
						m.transportBusy = true
						m.stateEpoch++
						m.status = "Transfiriendo música a esta computadora…"
						m.statusError = false
						m.pendingDevice = device.Name
						return m, m.act("transferencia local", func(ctx context.Context) error { return m.client.Transfer(ctx, device) })
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
			if msg.name == "pausar" || msg.name == "reproducir" {
				m.desiredPlaying = nil
			}
			if msg.name == "transferir" || msg.name == "transferencia local" {
				m.pendingDevice = ""
			}
		} else {
			m.statusError = false
			switch msg.name {
			case "volumen":
				m.status = fmt.Sprintf("Volumen fijado en %d%%", msg.value)
			case "transferir", "transferencia local":
				m.status = "Dispositivo cambiado; sincronizando reproducción…"
			case "pausar":
				m.status = "Pausa enviada · confirmando con Spotify…"
			case "reproducir", "canción", "playlist":
				m.status = "Reproducción enviada · confirmando con Spotify…"
			default:
				m.status = "Listo: " + msg.name
			}
		}
		if msg.name == "transferir" || msg.name == "transferencia local" {
			return m, tea.Batch(m.fetchState(), m.fetchDevices(), tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return refreshDueMsg{devices: true} }))
		}
		if msg.name == "pausar" || msg.name == "reproducir" || msg.name == "playlist" {
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
		if !m.localReady {
			return m, tea.Batch(m.fetchState(), m.fetchDevices(), poll())
		}
		return m, tea.Batch(m.fetchState(), poll())
	case tea.KeyPressMsg:
		key := msg.String()
		repeated := msg.Key().IsRepeat
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		if m.searching {
			switch key {
			case "esc":
				m.searching = false
				m.searchSeq++
			case "backspace":
				m.searchSeq++
				chars := []rune(m.searchInput)
				if len(chars) > 0 {
					m.searchInput = string(chars[:len(chars)-1])
				}
			case "up":
				if m.searchSelected > 0 {
					m.searchSelected--
				}
			case "down":
				if m.searchSelected < len(m.searchResults)-1 {
					m.searchSelected++
				}
			case "enter":
				if len(m.searchResults) > 0 && m.searchInput == "" {
					device, ok := m.localDevice()
					if !ok {
						m.status = "El dispositivo local aún no está listo"
						m.statusError = true
						return m, nil
					}
					track := m.searchResults[m.searchSelected]
					m.searching = false
					m.transportBusy = true
					m.stateEpoch++
					m.status = "Reproduciendo " + track.Name + " en esta computadora…"
					m.statusError = false
					return m, m.act("canción", func(ctx context.Context) error { return m.client.PlayTrack(ctx, track, device.ID) })
				}
				query := strings.TrimSpace(m.searchInput)
				if query != "" {
					m.searchSeq++
					seq := m.searchSeq
					m.searchInput = ""
					m.searchResults = nil
					m.status = "Buscando " + query + "…"
					m.statusError = false
					return m, func() tea.Msg {
						tracks, err := m.client.SearchTracks(context.Background(), query)
						return searchMsg{tracks: tracks, err: err, seq: seq}
					}
				}
			default:
				if text := msg.Key().Text; text != "" {
					m.searchSeq++
					m.searchInput += text
					m.searchResults = nil
				}
			}
			return m, nil
		}
		if key == "q" {
			return m, tea.Quit
		}
		if m.showPlaylists {
			if m.showTracks {
				switch key {
				case "esc":
					m.showTracks = false
					m.entriesSeq++
				case "j", "down":
					if m.entryPick < len(m.entries)-1 {
						m.entryPick++
					}
				case "k", "up":
					if m.entryPick > 0 {
						m.entryPick--
					}
				case "p":
					if !repeated {
						return m.playPlaylist(0)
					}
				case "enter":
					if !repeated && len(m.entries) > 0 {
						return m.playPlaylist(m.entries[m.entryPick].Position)
					}
				}
				if m.entriesMore && !m.entriesBusy && len(m.entries)-m.entryPick < 8 {
					m.entriesBusy = true
					return m, m.fetchEntries(m.entriesOffset)
				}
				return m, nil
			}
			switch key {
			case "esc", "l":
				m.showPlaylists = false
				m.playlistSeq++
			case "j", "down":
				if m.playlistPick < len(m.playlists)-1 {
					m.playlistPick++
				}
			case "k", "up":
				if m.playlistPick > 0 {
					m.playlistPick--
				}
			case "p", "space", " ":
				if !repeated && len(m.playlists) > 0 {
					m.activePlaylist = m.playlists[m.playlistPick]
					return m.playPlaylist(0)
				}
			case "enter":
				if len(m.playlists) > 0 {
					m.activePlaylist = m.playlists[m.playlistPick]
					m.showTracks = true
					m.entries, m.entryPick, m.entriesOffset = nil, 0, 0
					m.entriesSeq++
					m.entriesBusy = true
					return m, m.fetchEntries(0)
				}
			}
			if m.playlistMore && !m.playlistBusy && len(m.playlists)-m.playlistPick < 8 {
				m.playlistBusy = true
				return m, m.fetchPlaylists(len(m.playlists))
			}
			return m, nil
		}
		if m.showDevices {
			switch key {
			case "esc", "d":
				m.showDevices = false
			case "j", "down":
				if m.selected < len(m.devices)-1 {
					m.selected++
				}
			case "k", "up":
				if m.selected > 0 {
					m.selected--
				}
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
					m.showDevices = false
					if device.IsActive {
						m.status = device.Name + " ya está activo"
						return m, nil
					}
					m.transportBusy = true
					m.stateEpoch++
					m.status = "Transfiriendo a " + device.Name + "…"
					m.statusError = false
					m.pendingDevice = device.Name
					return m, m.act("transferir", func(ctx context.Context) error {
						if err := m.client.Transfer(ctx, device); err != nil {
							return fmt.Errorf("transferir a %s: %w", device.Name, err)
						}
						return nil
					})
				}
			}
			return m, nil
		}
		switch key {
		case "l":
			m.showPlaylists = true
			m.showTracks = false
			m.playlists, m.playlistPick = nil, 0
			m.playlistSeq++
			m.playlistBusy = true
			return m, m.fetchPlaylists(0)
		case "/":
			m.searching = true
			m.searchSeq++
			m.searchInput = ""
			m.searchResults = nil
			m.status = "Escribe una canción o artista y pulsa Enter"
			m.statusError = false
			return m, nil
		case "d":
			m.showDevices = true
			return m, m.fetchDevices()
		case " ", "space":
			if repeated || m.transportBusy || time.Since(m.lastTransport) < 450*time.Millisecond {
				return m, nil
			}
			if !m.state.Available {
				m.status = "Esperando el reproductor local o una canción para iniciar"
				return m, nil
			}
			m.transportBusy = true
			m.lastTransport = time.Now()
			m.stateEpoch++
			if m.state.IsPlaying {
				m.state.IsPlaying = false
				value := false
				m.desiredPlaying = &value
				m.desiredSince = time.Now()
				m.status = "Pausando…"
				m.statusError = false
				return m, m.act("pausar", m.client.Pause)
			}
			m.state.IsPlaying = true
			value := true
			m.desiredPlaying = &value
			m.desiredSince = time.Now()
			m.status = "Reproduciendo…"
			m.statusError = false
			return m, m.act("reproducir", m.client.Play)
		case "n":
			if repeated || m.transportBusy || time.Since(m.lastTransport) < 450*time.Millisecond {
				return m, nil
			}
			m.transportBusy = true
			m.lastTransport = time.Now()
			m.stateEpoch++
			m.status = "Saltando a la siguiente…"
			return m, m.act("siguiente", m.client.Next)
		case "p":
			if repeated || m.transportBusy || time.Since(m.lastTransport) < 450*time.Millisecond {
				return m, nil
			}
			m.transportBusy = true
			m.lastTransport = time.Now()
			m.stateEpoch++
			m.status = "Volviendo a la anterior…"
			return m, m.act("anterior", m.client.Previous)
		case "+", "=", "-", "_":
			if m.state.Device == nil || !m.state.Device.SupportsVolume || m.state.Device.VolumePercent == nil {
				m.status = "El dispositivo actual no permite ajustar el volumen"
				m.statusError = true
				return m, nil
			}
			value := *m.state.Device.VolumePercent
			if m.volumeOverride != nil {
				value = *m.volumeOverride
			}
			if key == "+" || key == "=" {
				value += 5
			} else {
				value -= 5
			}
			value = min(100, max(0, value))
			m.volumeOverride = &value
			m.volumeSeq++
			seq := m.volumeSeq
			return m, tea.Tick(180*time.Millisecond, func(time.Time) tea.Msg { return volumeDueMsg{seq} })
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#1DB954")).Bold(true)
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F7F7")).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#8F9AA7"))
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("#5EEAD4")).Bold(true)
	magenta := lipgloss.NewStyle().Foreground(lipgloss.Color("#EFA6E9")).Bold(true)
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color("#F5B841"))
	width, height := max(20, m.width), max(10, m.height)
	fit := func(s string) string {
		return lipgloss.PlaceHorizontal(width, lipgloss.Left, ansi.Truncate(s, width, "…"))
	}
	lines := make([]string, 0, height)
	live := green.Render("●")
	if m.lastSync.IsZero() || time.Since(m.lastSync) > 20*time.Second {
		live = warning.Render("◌")
	}
	lines = append(lines, fit(" "+green.Render("♫ SPOTYGO")+"  "+live))
	lines = append(lines, fit(muted.Render(strings.Repeat("─", width))))
	track, artist := "Nada en reproducción", ""
	if m.state.Item != nil {
		track = m.state.Item.Name
		var names []string
		for _, a := range m.state.Item.Artists {
			names = append(names, a.Name)
		}
		artist = strings.Join(names, ", ")
	}
	activity := muted.Render("⠿")
	if m.state.IsPlaying {
		activity = green.Render(m.spinner.View())
	}
	lines = append(lines, fit(" "+activity+"  "+bright.Render(track)))
	lines = append(lines, fit(" "+cyan.Render(artist)))
	if m.state.Item != nil {
		position := m.state.ProgressMS
		if m.state.IsPlaying && !m.lastSync.IsZero() {
			position += int(time.Since(m.lastSync).Milliseconds())
		}
		position = min(position, m.state.Item.DurationMS)
		fraction := 0.0
		if m.state.Item.DurationMS > 0 {
			fraction = float64(position) / float64(m.state.Item.DurationMS)
		}
		lines = append(lines, fit(" "+muted.Render(duration(position))+" "+m.progress.ViewAs(fraction)+" "+muted.Render(duration(m.state.Item.DurationMS))))
	} else {
		lines = append(lines, fit(""))
	}
	device, volume := "", ""
	if m.state.Device != nil {
		device = m.state.Device.Name
		if m.state.Device.VolumePercent != nil {
			volume = fmt.Sprintf("%d%%", *m.state.Device.VolumePercent)
		}
	}
	if m.volumeOverride != nil {
		volume = fmt.Sprintf("%d%%", *m.volumeOverride)
	}
	lines = append(lines, fit(" "+cyan.Render(device)+"  "+magenta.Render(volume)))
	lines = append(lines, fit(muted.Render(strings.Repeat("─", width))))
	body := make([]string, 0)
	footer := " espacio pausa  n/p saltar  +/- volumen  / buscar  l playlists  d dispositivos  q salir"
	if m.showPlaylists {
		if m.showTracks {
			body = append(body, " "+green.Render(m.activePlaylist.Name))
			if m.entriesBusy && len(m.entries) == 0 {
				body = append(body, " "+warning.Render(m.spinner.View()))
			} else if len(m.entries) == 0 && !m.statusError {
				body = append(body, " "+muted.Render("Sin canciones"))
			}
			rows := max(0, height-len(lines)-4)
			start := listStart(m.entryPick, len(m.entries), rows)
			for i := start; i < len(m.entries) && i < start+rows; i++ {
				entry := m.entries[i]
				prefix, style := "  ", muted
				if i == m.entryPick {
					prefix, style = "› ", bright
				}
				body = append(body, " "+style.Render(fmt.Sprintf("%s%d. %s", prefix, entry.Position+1, entry.Track.Name)))
			}
			footer = " ↑/↓ elegir  Enter reproducir  p playlist completa  Esc volver"
		} else {
			body = append(body, " "+green.Render("PLAYLISTS"))
			if m.playlistBusy && len(m.playlists) == 0 {
				body = append(body, " "+warning.Render(m.spinner.View()))
			} else if len(m.playlists) == 0 && !m.statusError {
				body = append(body, " "+muted.Render("Sin playlists"))
			}
			rows := max(0, height-len(lines)-4)
			start := listStart(m.playlistPick, len(m.playlists), rows)
			for i := start; i < len(m.playlists) && i < start+rows; i++ {
				prefix, style := "  ", muted
				if i == m.playlistPick {
					prefix, style = "› ", bright
				}
				body = append(body, " "+style.Render(prefix+m.playlists[i].Name))
			}
			footer = " ↑/↓ elegir  Enter canciones  p reproducir  Esc volver"
		}
	} else if m.searching {
		body = append(body, " "+cyan.Render("/ "+m.searchInput+"▌"))
		for i, item := range m.searchResults {
			prefix, style := "  ", muted
			if i == m.searchSelected {
				prefix, style = "› ", bright
			}
			body = append(body, " "+style.Render(prefix+item.Name))
		}
		footer = " Enter buscar/reproducir  ↑/↓ elegir  Esc volver"
	} else if m.showDevices {
		body = append(body, " "+green.Render("DISPOSITIVOS"))
		for i, item := range m.devices {
			prefix, style := "  ", muted
			if i == m.selected {
				prefix, style = "› ", bright
			}
			label := prefix + item.Name
			if item.IsActive {
				label += "  ●"
			}
			body = append(body, " "+style.Render(label))
		}
		footer = " ↑/↓ elegir  Enter transferir  Esc volver"
	}
	space := max(0, height-len(lines)-2)
	for i := 0; i < space; i++ {
		if i < len(body) {
			lines = append(lines, fit(body[i]))
		} else {
			lines = append(lines, fit(""))
		}
	}
	feedback := ""
	if m.statusError {
		feedback = warning.Render("! " + m.status)
	} else if m.status != "" && (m.transportBusy || m.pendingDevice != "" || m.pendingContext != "" || m.desiredPlaying != nil || m.volumeExpected != nil || m.playlistBusy || m.entriesBusy || strings.HasPrefix(m.status, "Buscando")) {
		feedback = warning.Render("… " + m.status)
	}
	lines = append(lines, fit(" "+feedback), fit(muted.Render(footer)))
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func listStart(selected, total, rows int) int {
	if rows <= 0 || total <= rows {
		return 0
	}
	return min(max(0, selected-rows/2), total-rows)
}
func duration(ms int) string {
	seconds := max(0, ms/1000)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
