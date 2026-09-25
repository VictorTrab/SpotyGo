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
		Aliases:     []string{"s", "find"},
		Description: "Search tracks or artists on Spotify",
		Action:      "search",
	},
	{
		Name:        "play",
		Aliases:     []string{"p", "resume"},
		Description: "Resume or toggle playback",
		Action:      "play",
	},
	{
		Name:        "pause",
		Aliases:     []string{"stop"},
		Description: "Pause playback",
		Action:      "pause",
	},
	{
		Name:        "next",
		Aliases:     []string{"n", "skip"},
		Description: "Skip to next track",
		Action:      "next",
	},
	{
		Name:        "prev",
		Aliases:     []string{"previous", "b", "back"},
		Description: "Play previous track",
		Action:      "prev",
	},
	{
		Name:        "playlists",
		Aliases:     []string{"pl", "list"},
		Description: "View saved playlists",
		Action:      "playlists",
	},
	{
		Name:        "devices",
		Aliases:     []string{"dev", "d"},
		Description: "Spotify Connect devices",
		Action:      "devices",
	},
	{
		Name:        "volume",
		Aliases:     []string{"vol", "v"},
		Description: "Set playback volume (e.g. /volume 80)",
		Action:      "volume",
	},
	{
		Name:        "theme",
		Aliases:     []string{"th", "t"},
		Description: "Change color theme",
		Action:      "theme",
	},
	{
		Name:        "background",
		Aliases:     []string{"bg"},
		Description: "Background style (default / flow / dark)",
		Action:      "background",
	},
	{
		Name:        "art",
		Aliases:     []string{"cover", "zen", "c", "z"},
		Description: "High-resolution album art in Zen mode",
		Action:      "art",
	},
	{
		Name:        "quality",
		Aliases:     []string{"bitrate", "ql"},
		Description: "Audio bitrate (320k high / 160k normal / 96k low)",
		Action:      "quality",
	},
	{
		Name:        "login",
		Aliases:     []string{"auth"},
		Description: "Authenticate or change Spotify account",
		Action:      "login",
	},
	{
		Name:        "help",
		Aliases:     []string{"h", "?"},
		Description: "Show available commands and shortcuts",
		Action:      "help",
	},
	{
		Name:        "version",
		Aliases:     []string{"v", "about"},
		Description: "Show installed SpotifyGo version",
		Action:      "version",
	},
	{
		Name:        "update",
		Aliases:     []string{"upgrade"},
		Description: "Check and apply updates from GitHub",
		Action:      "update",
	},
	{
		Name:        "quit",
		Aliases:     []string{"q", "exit"},
		Description: "Close SpotifyGo",
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
