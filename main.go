package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/VictorTrab/SpotyGo/internal/spotify"
	"github.com/VictorTrab/SpotyGo/internal/ui"
)

func main() {
	clientID := strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID"))
	if clientID == "" {
		fmt.Fprintln(os.Stderr, "Configura SPOTIFY_CLIENT_ID con el Client ID de tu app de Spotify.")
		fmt.Fprintln(os.Stderr, "Registra http://127.0.0.1:8989/callback como Redirect URI.")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	auth := spotify.NewAuth(clientID)
	if _, err := auth.AccessToken(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Autenticación:", err)
		os.Exit(1)
	}
	program := tea.NewProgram(ui.New(spotify.NewClient(auth)))
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Interfaz:", err)
		os.Exit(1)
	}
}
