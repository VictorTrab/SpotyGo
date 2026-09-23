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
	state          spotify.PlaybackState
	devices        []spotify.Device
	showDevices    bool
	selected       int
	status         string
	width          int
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
	desiredPlaying *bool
	desiredSince   time.Time
	spinner        spinner.Model
	progress       progress.Model
}

func New(client *spotify.Client, localName string, engineDone <-chan error) Model {
	pulse := spinner.New(spinner.WithSpinner(spinner.Spinner{
		Frames: []string{"[| . . .]", "[| | . .]", "[| | | .]", "[| | | |]", "[| | | .]", "[| | . .]"},
		FPS:    180 * time.Millisecond,
	}))
	bar := progress.New(progress.WithWidth(34), progress.WithoutPercentage(), progress.WithColors(lipgloss.Color("#1DB954"), lipgloss.Color("#5EEAD4")), progress.WithFillCharacters('=', '-'))
	return Model{client: client, localName: localName, engineDone: engineDone, width: 68, status: "Iniciando audio en esta computadora…", spinner: pulse, progress: bar}
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
			if len(msg.tracks) == 0 {
				m.status = "No se encontraron canciones"
			} else {
				m.status = fmt.Sprintf("%d canciones encontradas", len(msg.tracks))
			}
		}
	case engineStoppedMsg:
		m.localReady = false
		m.status = fmt.Sprintf("Motor de audio detenido: %v. Revisa el registro de librespot.", msg.err)
		m.statusError = true
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.progress.SetWidth(max(12, min(40, msg.Width-38)))
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
					if m.state.IsPlaying {
						m.status = "Reproduciendo"
					} else {
						m.status = "Pausado"
					}
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
				m.status = "Audio activo en " + m.pendingDevice
				m.pendingDevice = ""
				m.statusError = false
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
						m.status = "Audio local listo"
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
			case "reproducir", "canción":
				m.status = "Reproducción enviada · confirmando con Spotify…"
			default:
				m.status = "Listo: " + msg.name
			}
		}
		if msg.name == "transferir" || msg.name == "transferencia local" {
			return m, tea.Batch(m.fetchState(), m.fetchDevices(), tea.Tick(1200*time.Millisecond, func(time.Time) tea.Msg { return refreshDueMsg{devices: true} }))
		}
		if msg.name == "pausar" || msg.name == "reproducir" {
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
	boxWidth := max(24, min(92, m.width-6))
	box := lipgloss.NewStyle().Width(boxWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#1DB954"))

	var lines []string
	liveness := "○ CONECTANDO"
	if !m.lastSync.IsZero() {
		age := time.Since(m.lastSync).Round(time.Second)
		if age < 20*time.Second {
			liveness = "● EN LÍNEA · actualizado hace " + age.String()
		} else {
			liveness = "◌ SINCRONIZANDO · última respuesta hace " + age.String()
		}
	}
	liveStyle := green
	if m.lastSync.IsZero() || time.Since(m.lastSync) >= 20*time.Second {
		liveStyle = warning
	}
	lines = append(lines, green.Render("♫  SPOTYGO")+"  "+cyan.Render("/  TU REPRODUCTOR LOCAL"), liveStyle.Render(liveness), "")
	if !m.state.Available {
		lines = append(lines, bright.Render("Elige algo para escuchar"), muted.Render("Pulsa / para buscar una canción o un artista."))
	} else {
		icon := "Ⅱ [ . . . .]"
		if m.state.IsPlaying {
			icon = "▶ " + m.spinner.View()
		}
		track := "Sin canción"
		artist := ""
		if m.state.Item != nil {
			track = m.state.Item.Name
			var names []string
			for _, entry := range m.state.Item.Artists {
				names = append(names, entry.Name)
			}
			artist = strings.Join(names, ", ")
		}
		lines = append(lines, green.Render(icon)+"  "+bright.Render(track), cyan.Render("   "+artist))
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
			lines = append(lines, "", muted.Render(duration(position))+"  "+m.progress.ViewAs(fraction)+"  "+muted.Render(duration(m.state.Item.DurationMS)))
		}
		if m.state.Device != nil {
			volume := "—"
			if m.state.Device.VolumePercent != nil {
				volume = fmt.Sprintf("%d%%", *m.state.Device.VolumePercent)
			}
			if m.volumeOverride != nil {
				volume = fmt.Sprintf("%d%%", *m.volumeOverride)
			}
			lines = append(lines, "", muted.Render("SALIDA  ")+cyan.Render(m.state.Device.Name), muted.Render("VOLUMEN ")+magenta.Render(volume))
		}
	}
	lines = append(lines, "", green.Render(strings.Repeat("─", max(20, min(58, boxWidth-4)))), "")
	if !m.localReady {
		lines = append(lines, warning.Render("Iniciando el dispositivo local; autoriza Spotify en el navegador si se abre."), "")
	}
	if m.searching {
		lines = append(lines, cyan.Render("BUSCAR  ")+bright.Render(m.searchInput)+green.Render("▌"))
		if len(m.searchResults) > 0 {
			for i, track := range m.searchResults {
				prefix := "  "
				if i == m.searchSelected {
					prefix = "> "
				}
				artists := make([]string, 0, len(track.Artists))
				for _, artist := range track.Artists {
					artists = append(artists, artist.Name)
				}
				label := prefix + track.Name + " — " + strings.Join(artists, ", ")
				if i == m.searchSelected {
					lines = append(lines, cyan.Render(label))
				} else {
					lines = append(lines, muted.Render(label))
				}
			}
			lines = append(lines, "", muted.Render("↑/↓ elegir · Enter reproducir · Esc cerrar"))
		} else {
			lines = append(lines, muted.Render("Enter buscar · Esc cerrar"))
		}
	} else if m.showDevices {
		lines = append(lines, cyan.Render("DISPOSITIVOS"))
		if len(m.devices) == 0 {
			lines = append(lines, muted.Render("No hay dispositivos disponibles."))
		}
		for i, device := range m.devices {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			label := fmt.Sprintf("%s%s (%s)", prefix, device.Name, device.Type)
			if device.IsActive {
				label += " • activo"
			}
			if device.IsRestricted || device.ID == "" {
				label += " • no controlable"
			}
			if i == m.selected {
				lines = append(lines, cyan.Render(label))
			} else {
				lines = append(lines, muted.Render(label))
			}
		}
		lines = append(lines, "", muted.Render("j/k elegir · Enter transferir · Esc cerrar"))
	} else {
		lines = append(lines, green.Render("ESPACIO")+muted.Render(" play/pausa   ")+cyan.Render("N/P")+muted.Render(" saltar   ")+magenta.Render("+/-")+muted.Render(" volumen"))
		lines = append(lines, cyan.Render("/")+muted.Render(" buscar música   ")+cyan.Render("D")+muted.Render(" dispositivos   ")+cyan.Render("Q")+muted.Render(" salir"))
	}
	feedback := green.Render("✓ " + m.status)
	if m.statusError {
		feedback = warning.Render("! " + m.status)
	} else if m.transportBusy || strings.HasPrefix(m.status, "Buscando") {
		feedback = warning.Render("… " + m.status)
	}
	lines = append(lines, "", feedback)
	rendered := box.Render(strings.Join(lines, "\n"))
	view := tea.NewView(lipgloss.PlaceHorizontal(max(m.width, boxWidth+6), lipgloss.Center, rendered))
	view.AltScreen = true
	return view
}

func duration(ms int) string {
	seconds := max(0, ms/1000)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
