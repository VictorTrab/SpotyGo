package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/VictorTrab/SpotyGo/internal/logger"
	"github.com/VictorTrab/SpotyGo/internal/player"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/theme"
	"github.com/VictorTrab/SpotyGo/internal/ui"
	"github.com/VictorTrab/SpotyGo/internal/version"
)

func main() {
	_ = logger.Init()
	defer logger.Close()

	if err := run(os.Args[1:]); err != nil {
		logger.Error("Ejecución finalizada con error: %v", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	login := false
	doctor := false
	clientIDOverride := ""
	if len(args) > 0 {
		switch args[0] {
		case "version", "-v", "--version":
			fmt.Println(version.Info())
			fmt.Println(version.RepoURL)
			return nil
		case "update", "--update":
			return version.RunUpdate()
		case "login":
			login = true
			flags := flag.NewFlagSet("login", flag.ContinueOnError)
			flags.StringVar(&clientIDOverride, "client-id", "", "Client ID de la app de Spotify")
			if err := flags.Parse(args[1:]); err != nil {
				return err
			}
			if flags.NArg() != 0 {
				return errors.New("uso: spotifygo login [--client-id ID]")
			}
		case "doctor", "diag", "diagnostico":
			doctor = true
		case "background", "bg":
			flags := flag.NewFlagSet("background", flag.ContinueOnError)
			if err := flags.Parse(args[1:]); err != nil {
				return err
			}
			return runBackground(flags.Args())
		case "help", "-h", "--help":
			fmt.Printf("SpotifyGo %s - Reproductor y cliente de Spotify en terminal\n\n", version.Current)
			fmt.Println("Uso: spotifygo [comando] [opciones]")
			fmt.Println()
			fmt.Println("Comandos:")
			fmt.Println("  spotifygo                      Inicia el reproductor en la terminal")
			fmt.Println("  spotifygo login                Inicia o comprueba la sesión de Spotify")
			fmt.Println("  spotifygo doctor               Diagnóstico de cuenta, API y estado de reproducción")
			fmt.Println("  spotifygo background [modo]    Consulta o fija el fondo (flow, gradient, dark)")
			fmt.Println("  spotifygo update               Actualiza SpotifyGo a la última versión")
			fmt.Println("  spotifygo version, -v          Muestra la versión instalada")
			fmt.Println("  spotifygo help, -h             Muestra esta ayuda")
			return nil
		default:
			return fmt.Errorf("comando desconocido %q; usa spotifygo --help", args[0])
		}
	}
	clientID, err := resolveClientID(clientIDOverride)
	if err != nil {
		return err
	}
	if clientID == "" {
		return errors.New("falta el Client ID; ejecuta spotifygo login --client-id TU_CLIENT_ID")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	auth := spotify.NewAuth(clientID)
	if _, err := auth.AccessToken(ctx); err != nil {
		return fmt.Errorf("autenticación: %w", err)
	}
	if login {
		if err := saveClientID(clientID); err != nil {
			return err
		}
		fmt.Println("Sesión de Spotify activa. Ejecuta spotifygo para abrir la interfaz.")
		return nil
	}
	if doctor {
		return runDoctor(ctx, auth)
	}
	cfg := theme.LoadConfig()
	engine, err := player.Start(cfg.Bitrate)
	if err != nil {
		return err
	}
	defer engine.Stop()
	client := spotify.NewClient(auth)
	defer client.Close()
	m := ui.New(client, engine.Name, engine.Done, engine.Events, engine.VolumeEvents)
	m.SetEngineRestarter(engine.Restart)
	m.SetTrackEvents(engine.TrackEvents)
	program := tea.NewProgram(m, tea.WithContext(ctx))
	if _, err := program.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("interfaz: %w", err)
	}
	return nil
}

// runBackground prints or updates the persistent background mode from the CLI,
// mirroring the /bg command available inside the interface.
func runBackground(args []string) error {
	if len(args) == 0 {
		cfg := theme.LoadConfig()
		fmt.Printf("Fondo actual: %s\n", cfg.Background)
		fmt.Println("Modos: flow (por defecto), gradient, dark")
		return nil
	}
	if len(args) > 1 {
		return errors.New("uso: spotifygo background [flow|gradient|dark]")
	}

	mode := strings.ToLower(strings.TrimSpace(args[0]))
	switch mode {
	case "flow", "wave", "motion":
		mode = "flow"
	case "gradient", "default", "def":
		mode = "gradient"
	case "dark", "theme":
		mode = "dark"
	default:
		return fmt.Errorf("fondo desconocido %q; usa flow, gradient o dark", args[0])
	}
	if err := theme.SaveBackground(mode); err != nil {
		return err
	}
	cfg := theme.LoadConfig()
	fmt.Printf("Fondo fijado en: %s\n", cfg.Background)
	return nil
}

// runDoctor prints a read-only diagnosis of the Spotify link: who the account is,
// what is playing, which scopes were granted and whether the deprecated mood
// endpoints are still reachable for this application.
func runDoctor(ctx context.Context, auth *spotify.Auth) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client := spotify.NewClient(auth)
	client.Close()

	cfg := theme.LoadConfig()
	fmt.Printf("SpotifyGo %s · diagnóstico\n", version.Current)
	fmt.Printf("  Client ID .......... %s\n", auth.ClientID())
	fmt.Printf("  Scopes concedidos .. %s\n", scopesOrUnknown(auth.Scopes()))
	fmt.Printf("  Preferencias ....... tema: %s · fondo: %s · bitrate: %s\n", cfg.Theme, cfg.Background, cfg.Bitrate)
	fmt.Printf("  Log ................ %s\n", logger.LogFilePath())
	fmt.Println()

	profile, err := client.Me(ctx)
	switch {
	case errors.Is(err, spotify.ErrUnavailable):
		fmt.Printf("  GET /me ............ BLOQUEADO (%v)\n", err)
	case err != nil:
		fmt.Printf("  GET /me ............ ERROR (%v)\n", err)
	case strings.TrimSpace(profile.DisplayName) == "":
		fmt.Printf("  GET /me ............ OK pero sin nombre visible (id %q)\n", profile.ID)
	default:
		fmt.Printf("  GET /me ............ OK · nombre: %q (id %q, país %q, plan %q)\n",
			profile.DisplayName, profile.ID, profile.Country, profile.Product)
	}

	state, err := client.Playback(ctx)
	probeTrack := (*spotify.Track)(nil)
	switch {
	case err != nil:
		fmt.Printf("  GET /me/player ..... ERROR (%v)\n", err)
	case !state.Available || state.Item == nil:
		fmt.Println("  GET /me/player ..... sin reproducción activa")
	default:
		device := "desconocido"
		if state.Device != nil && state.Device.Name != "" {
			device = state.Device.Name
		}
		fmt.Printf("  GET /me/player ..... OK · %q de %s · %s · dispositivo: %s\n",
			state.Item.Name, artistNames(state.Item.Artists), formatMillis(state.Item.DurationMS), device)
		probeTrack = state.Item
	}

	if probeTrack == nil {
		if recent, err := client.RecentlyPlayed(ctx, 1); err == nil && len(recent) > 0 {
			probeTrack = &recent[0]
			fmt.Printf("  última escuchada ... %q de %s (para probar el análisis)\n",
				probeTrack.Name, artistNames(probeTrack.Artists))
		}
	}

	if probeTrack != nil {
		if len(probeTrack.Artists) > 0 && probeTrack.Artists[0].ID != "" {
			genres, err := client.ArtistGenres(ctx, probeTrack.Artists[0].ID)
			switch {
			case errors.Is(err, spotify.ErrUnavailable):
				fmt.Printf("  GET /artists/{id} .. BLOQUEADO (%v)\n", err)
			case err != nil:
				fmt.Printf("  GET /artists/{id} .. ERROR (%v)\n", err)
			default:
				fmt.Printf("  GET /artists/{id} .. OK · %d géneros: %s\n", len(genres), strings.Join(genres, ", "))
			}
		} else {
			fmt.Println("  GET /artists/{id} .. sin id de artista en la respuesta")
		}

		features, err := client.AudioFeatures(ctx, probeTrack.URI)
		switch {
		case errors.Is(err, spotify.ErrUnavailable):
			fmt.Printf("  audio-features ..... NO DISPONIBLE (%v)\n", err)
			fmt.Println("                       → el ánimo se estimará localmente")
		case err != nil:
			fmt.Printf("  audio-features ..... ERROR (%v)\n", err)
		default:
			fmt.Printf("  audio-features ..... OK · energía %.2f · valencia %.2f · tempo %.0f BPM\n",
				features.Energy, features.Valence, features.Tempo)
			fmt.Println("                       → se puede usar el dato real de Spotify")
		}
	}
	return nil
}

func scopesOrUnknown(granted string) string {
	if strings.TrimSpace(granted) == "" {
		return "(desconocidos)"
	}
	return granted
}

func artistNames(artists []spotify.Artist) string {
	names := make([]string, 0, len(artists))
	for _, a := range artists {
		if a.Name != "" {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		return "artista desconocido"
	}
	return strings.Join(names, ", ")
}

func formatMillis(ms int) string {
	if ms <= 0 {
		return "duración desconocida"
	}
	total := ms / 1000
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("encontrar configuración de usuario: %w", err)
	}
	return filepath.Join(dir, "SpotyGo", "config.json"), nil
}

func resolveClientID(override string) (string, error) {
	if id := strings.TrimSpace(override); id != "" {
		return id, nil
	}
	if id := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID")); id != "" {
		return id, nil
	}
	path, err := configPath()
	if err != nil {
		return spotify.DefaultClientID, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return spotify.DefaultClientID, nil
	}
	if err != nil {
		return spotify.DefaultClientID, nil
	}
	var config struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return spotify.DefaultClientID, nil
	}
	if id := strings.TrimSpace(config.ClientID); id != "" && id != "b9e100e89b6e4a17b81e3ad3803414d5" {
		return id, nil
	}
	return spotify.DefaultClientID, nil
}

func saveClientID(id string) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("crear directorio de configuración: %w", err)
	}
	data, err := json.MarshalIndent(struct {
		ClientID string `json:"client_id"`
	}{ClientID: id}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("guardar Client ID: %w", err)
	}
	return nil
}
