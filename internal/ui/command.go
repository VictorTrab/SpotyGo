package ui

import "strings"

// CommandDef defines an executable command in the command bar.
type CommandDef struct {
	Name        string
	Aliases     []string
	Description string
	Action      string
}

var registeredCommands = []CommandDef{
	{
		Name:        "search",
		Aliases:     []string{"s", "buscar", "find"},
		Description: "Buscar canciones o artistas en Spotify",
		Action:      "search",
	},
	{
		Name:        "play",
		Aliases:     []string{"p", "reproducir", "resume", "reanudar"},
		Description: "Reanudar o pausar reproducción",
		Action:      "play",
	},
	{
		Name:        "pause",
		Aliases:     []string{"pausa", "stop"},
		Description: "Pausar reproducción",
		Action:      "pause",
	},
	{
		Name:        "next",
		Aliases:     []string{"n", "sig"},
		Description: "Siguiente canción",
		Action:      "next",
	},
	{
		Name:        "prev",
		Aliases:     []string{"previous", "b", "ant"},
		Description: "Canción anterior",
		Action:      "prev",
	},
	{
		Name:        "playlists",
		Aliases:     []string{"pl", "listas"},
		Description: "Ver tus playlists guardadas",
		Action:      "playlists",
	},
	{
		Name:        "devices",
		Aliases:     []string{"dev", "d"},
		Description: "Dispositivos Spotify Connect",
		Action:      "devices",
	},
	{
		Name:        "volume",
		Aliases:     []string{"vol", "v"},
		Description: "Ajustar volumen (ej: /volume 80)",
		Action:      "volume",
	},
	{
		Name:        "theme",
		Aliases:     []string{"th", "t"},
		Description: "Cambiar paleta de colores",
		Action:      "theme",
	},
	{
		Name:        "background",
		Aliases:     []string{"bg", "fondo"},
		Description: "Estilo de fondo (default / flow / dark)",
		Action:      "background",
	},
	{
		Name:        "art",
		Aliases:     []string{"cover", "zen", "caratula", "c", "z"},
		Description: "Mostrar carátula en alta definición / ocultar playlists",
		Action:      "art",
	},
	{
		Name:        "quality",
		Aliases:     []string{"bitrate", "calidad", "ql"},
		Description: "Calidad de audio (320 alta / 160 media)",
		Action:      "quality",
	},
	{
		Name:        "login",
		Aliases:     []string{"auth"},
		Description: "Vincular o cambiar cuenta de Spotify",
		Action:      "login",
	},
	{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Ver comandos y atajos disponibles",
		Action:      "help",
	},
	{
		Name:        "quit",
		Aliases:     []string{"q", "exit"},
		Description: "Cerrar SpotifyGo",
		Action:      "quit",
	},
}

// FilterCommands returns commands matching the given prefix query for live autocomplete.
func FilterCommands(input string) []CommandDef {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" {
		return registeredCommands
	}
	head := trimmed
	if fields := strings.Fields(trimmed); len(fields) > 0 {
		head = fields[0]
	}
	var matches []CommandDef
	for _, cmd := range registeredCommands {
		if strings.HasPrefix(cmd.Name, head) {
			matches = append(matches, cmd)
			continue
		}
		matchedAlias := false
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, head) {
				matches = append(matches, cmd)
				matchedAlias = true
				break
			}
		}
		if !matchedAlias && (strings.Contains(cmd.Name, head) || strings.Contains(strings.ToLower(cmd.Description), head)) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// ResolveCommand resolves input to a command definition by exact alias or prefix.
func ResolveCommand(input string) (CommandDef, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" {
		return CommandDef{}, false
	}
	head := trimmed
	if fields := strings.Fields(trimmed); len(fields) > 0 {
		head = fields[0]
	}

	// 1. Exact match on Name or Aliases
	for _, cmd := range registeredCommands {
		if cmd.Name == head {
			return cmd, true
		}
		for _, a := range cmd.Aliases {
			if a == head {
				return cmd, true
			}
		}
	}
	// 2. Prefix match on Name or Aliases
	for _, cmd := range registeredCommands {
		if strings.HasPrefix(cmd.Name, head) {
			return cmd, true
		}
		for _, a := range cmd.Aliases {
			if strings.HasPrefix(a, head) {
				return cmd, true
			}
		}
	}
	return CommandDef{}, false
}
