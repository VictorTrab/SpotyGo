package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
)

type stateMsg struct {
	state spotify.PlaybackState
	err   error
}

type devicesMsg struct {
	devices []spotify.Device
	err     error
}

type actionMsg struct {
	name string
	seq  int
	err  error
}

type volumeDueMsg struct{ seq int }
type pollMsg struct{}
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
}

func New(client *spotify.Client, localName string, engineDone <-chan error) Model {
	return Model{client: client, localName: localName, engineDone: engineDone, width: 68, status: "Iniciando audio en esta computadora…"}
}

func (m Model) fetchState() tea.Cmd {
	return func() tea.Msg {
		state, err := m.client.Playback(context.Background())
		return stateMsg{state, err}
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
	return tea.Batch(m.fetchState(), m.fetchDevices(), poll(), func() tea.Msg {
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
	case searchMsg:
		if msg.seq != m.searchSeq || !m.searching {
			break
		}
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case stateMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.state = msg.state
			if m.status == "Conectando con Spotify…" {
				m.status = "Listo"
			}
		}
	case devicesMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.devices = msg.devices
			for _, device := range m.devices {
				if device.Name == m.localName && device.ID != "" {
					m.localReady = true
					if !m.transferSent && !device.IsActive {
						m.transferSent = true
						m.status = "Transfiriendo música a esta computadora…"
						return m, m.act("transferir", func(ctx context.Context) error { return m.client.Transfer(ctx, device) })
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
		if msg.name != "volumen" {
			m.transportBusy = false
		}
		if msg.name == "volumen" && msg.seq == m.volumeSeq {
			m.volumeOverride = nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			if msg.name == "transferir" {
				m.transferSent = false
			}
		} else {
			m.status = "Acción completada: " + msg.name
		}
		if msg.name == "transferir" {
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
			return actionMsg{name: "volumen", seq: msg.seq, err: err}
		}
	case pollMsg:
		if !m.localReady {
			return m, tea.Batch(m.fetchState(), m.fetchDevices(), poll())
		}
		return m, tea.Batch(m.fetchState(), poll())
	case tea.KeyPressMsg:
		key := msg.String()
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
						return m, nil
					}
					track := m.searchResults[m.searchSelected]
					m.searching = false
					m.transportBusy = true
					m.status = "Reproduciendo " + track.Name + " en esta computadora…"
					return m, m.act("canción", func(ctx context.Context) error { return m.client.PlayTrack(ctx, track, device.ID) })
				}
				query := strings.TrimSpace(m.searchInput)
				if query != "" {
					m.searchSeq++
					seq := m.searchSeq
					m.searchInput = ""
					m.searchResults = nil
					m.status = "Buscando " + query + "…"
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
					m.showDevices = false
					m.status = "Transfiriendo a " + device.Name + "…"
					return m, m.act("transferir", func(ctx context.Context) error { return m.client.Transfer(ctx, device) })
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
			return m, nil
		case "d":
			m.showDevices = true
			return m, m.fetchDevices()
		case " ", "space":
			if m.transportBusy {
				return m, nil
			}
			if !m.state.Available {
				m.status = "Esperando el reproductor local o una canción para iniciar"
				return m, nil
			}
			m.transportBusy = true
			if m.state.IsPlaying {
				m.state.IsPlaying = false
				return m, m.act("pausar", m.client.Pause)
			}
			m.state.IsPlaying = true
			return m, m.act("reproducir", m.client.Play)
		case "n":
			if m.transportBusy {
				return m, nil
			}
			m.transportBusy = true
			return m, m.act("siguiente", m.client.Next)
		case "p":
			if m.transportBusy {
				return m, nil
			}
			m.transportBusy = true
			return m, m.act("anterior", m.client.Previous)
		case "+", "=", "-", "_":
			if m.state.Device == nil || !m.state.Device.SupportsVolume || m.state.Device.VolumePercent == nil {
				m.status = "El dispositivo actual no permite ajustar el volumen"
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
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#8F9AA7"))
	warning := lipgloss.NewStyle().Foreground(lipgloss.Color("#F5B841"))
	boxWidth := max(34, min(80, m.width-4))
	box := lipgloss.NewStyle().Width(boxWidth).Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#1DB954"))

	var lines []string
	lines = append(lines, green.Render("♫ SpotyGo")+"  "+muted.Render("Audio en esta computadora"), "")
	if !m.state.Available {
		lines = append(lines, "No hay reproducción activa.", muted.Render("Esperando el reproductor local o una canción."))
	} else {
		icon := "Ⅱ"
		if m.state.IsPlaying {
			icon = "▶"
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
		lines = append(lines, green.Render(icon)+"  "+track, muted.Render(artist))
		if m.state.Item != nil {
			lines = append(lines, muted.Render(fmt.Sprintf("%s / %s", duration(m.state.ProgressMS), duration(m.state.Item.DurationMS))))
		}
		if m.state.Device != nil {
			volume := "—"
			if m.state.Device.VolumePercent != nil {
				volume = fmt.Sprintf("%d%%", *m.state.Device.VolumePercent)
			}
			if m.volumeOverride != nil {
				volume = fmt.Sprintf("%d%%", *m.volumeOverride)
			}
			lines = append(lines, "", "Dispositivo: "+m.state.Device.Name+"  |  Volumen: "+volume)
		}
	}
	lines = append(lines, "")
	if !m.localReady {
		lines = append(lines, warning.Render("Iniciando el dispositivo local; autoriza Spotify en el navegador si se abre."), "")
	}
	if m.searching {
		lines = append(lines, green.Render("Buscar: ")+m.searchInput+"▌")
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
				lines = append(lines, prefix+track.Name+" — "+strings.Join(artists, ", "))
			}
			lines = append(lines, "", muted.Render("↑/↓ elegir · Enter reproducir · Esc cerrar"))
		} else {
			lines = append(lines, muted.Render("Enter buscar · Esc cerrar"))
		}
	} else if m.showDevices {
		lines = append(lines, green.Render("Dispositivos"))
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
			lines = append(lines, label)
		}
		lines = append(lines, "", muted.Render("j/k elegir · Enter transferir · Esc cerrar"))
	} else {
		lines = append(lines, muted.Render("Espacio play/pausa · n/p saltar · +/- volumen"))
		lines = append(lines, muted.Render("/ buscar música · d dispositivos · q salir"))
	}
	lines = append(lines, "", warning.Render(m.status))
	view := tea.NewView(box.Render(strings.Join(lines, "\n")))
	view.AltScreen = true
	return view
}

func duration(ms int) string {
	seconds := max(0, ms/1000)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
