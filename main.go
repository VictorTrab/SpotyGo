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

	tea "charm.land/bubbletea/v2"
	"github.com/VictorTrab/SpotyGo/internal/logger"
	"github.com/VictorTrab/SpotyGo/internal/player"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/theme"
	"github.com/VictorTrab/SpotyGo/internal/ui"
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
	clientIDOverride := ""
	if len(args) > 0 {
		switch args[0] {
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
		case "help", "-h", "--help":
			fmt.Println("Uso: spotifygo [login [--client-id ID]]")
			fmt.Println("  spotifygo        Reproduce música en esta computadora")
			fmt.Println("  spotifygo login  Inicia o comprueba la sesión de Spotify")
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
	m.EnableIntro()
	program := tea.NewProgram(m, tea.WithContext(ctx))
	if _, err := program.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("interfaz: %w", err)
	}
	return nil
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
