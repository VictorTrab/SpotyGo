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
	"github.com/VictorTrab/SpotyGo/internal/player"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/ui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
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
				return errors.New("uso: spotygo login [--client-id ID]")
			}
		case "help", "-h", "--help":
			fmt.Println("Uso: spotygo [login [--client-id ID]]")
			fmt.Println("  spotygo        Reproduce música en esta computadora")
			fmt.Println("  spotygo login  Inicia o comprueba la sesión de Spotify")
			return nil
		default:
			return fmt.Errorf("comando desconocido %q; usa spotygo --help", args[0])
		}
	}
	clientID, err := resolveClientID(clientIDOverride)
	if err != nil {
		return err
	}
	if clientID == "" {
		return errors.New("falta el Client ID; ejecuta spotygo login --client-id TU_CLIENT_ID")
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
		fmt.Println("Sesión de Spotify activa. Ejecuta spotygo para abrir la interfaz.")
		return nil
	}
	engine, err := player.Start()
	if err != nil {
		return err
	}
	defer engine.Stop()
	program := tea.NewProgram(ui.New(spotify.NewClient(auth), engine.Name, engine.Done))
	if _, err := program.Run(); err != nil {
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
		return "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("leer configuración: %w", err)
	}
	var config struct {
		ClientID string `json:"client_id"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("configuración inválida: %w", err)
	}
	return strings.TrimSpace(config.ClientID), nil
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
