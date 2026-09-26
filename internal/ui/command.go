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
		Name:        "background",
		Aliases:     []string{"bg"},
		Description: "Cambiar fondo; flow solo se anima en modo Zen (gradient o dark en modo normal)",
		Action:      "background",
	},
	{
		Name:        "login",
		Aliases:     []string{"auth"},
		Description: "Iniciar sesión o cambiar cuenta de Spotify",
		Action:      "login",
	},
	{
		Name:        "update",
		Aliases:     []string{"upgrade"},
		Description: "Buscar e instalar actualizaciones desde GitHub",
		Action:      "update",
	},
	{
		Name:        "changelog",
		Aliases:     []string{"news", "whatsnew"},
		Description: "Ver las novedades y cambios recientes",
		Action:      "changelog",
	},
	{
		Name:        "version",
		Aliases:     []string{"about", "v"},
		Description: "Mostrar versión instalada e información de SpotyGo",
		Action:      "version",
	},
	{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Ver guía de atajos de teclado y ayuda",
		Action:      "help",
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
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, head) {
				matches = append(matches, cmd)
				break
			}
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
