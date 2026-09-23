package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type playerPlaylist struct {
	Tracks []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Duration struct {
			Secs  int `json:"secs"`
			Nanos int `json:"nanos"`
		} `json:"duration"`
	} `json:"tracks"`
}

func readSpotifyPlayerPlaylist(ctx context.Context, exe, playlistID string) (playerPlaylist, error) {
	deadline, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	output, err := exec.CommandContext(deadline, exe, "get", "item", "playlist", "--id", playlistID).Output()
	if err != nil {
		return playerPlaylist{}, fmt.Errorf("leer playlist con spotify-player: %w", err)
	}
	var playlist playerPlaylist
	if err := json.Unmarshal(output, &playlist); err != nil {
		return playerPlaylist{}, fmt.Errorf("leer datos de spotify-player: %w", err)
	}
	return playlist, nil
}

func (c *Client) playlistEntriesViaPlayer(ctx context.Context, playlistID string, offset int) (PlaylistEntriesPage, error) {
	exe, err := spotifyPlayerExecutable()
	if err != nil {
		return PlaylistEntriesPage{}, err
	}
	playlist, err := readSpotifyPlayerPlaylist(ctx, exe, playlistID)
	if err != nil {
		if err := c.ensureSpotifyPlayer(exe); err != nil {
			return PlaylistEntriesPage{}, err
		}
		for attempt := 0; attempt < 6; attempt++ {
			select {
			case <-ctx.Done():
				return PlaylistEntriesPage{}, ctx.Err()
			case <-time.After(time.Second):
			}
			playlist, err = readSpotifyPlayerPlaylist(ctx, exe, playlistID)
			if err == nil {
				break
			}
		}
		if err != nil {
			return PlaylistEntriesPage{}, err
		}
	}
	return playerPlaylistPage(playlist, offset)
}

func playerPlaylistPage(playlist playerPlaylist, offset int) (PlaylistEntriesPage, error) {
	if offset < 0 || offset > len(playlist.Tracks) {
		return PlaylistEntriesPage{}, errors.New("offset de playlist inválido")
	}
	page := PlaylistEntriesPage{Total: len(playlist.Tracks)}
	end := min(offset+50, len(playlist.Tracks))
	for i := offset; i < end; i++ {
		item := playlist.Tracks[i]
		if item.ID == "" {
			continue
		}
		track := Track{URI: "spotify:track:" + item.ID, Name: item.Name, DurationMS: item.Duration.Secs*1000 + item.Duration.Nanos/1_000_000}
		for _, artist := range item.Artists {
			track.Artists = append(track.Artists, struct {
				Name string `json:"name"`
			}{Name: artist.Name})
		}
		page.Items = append(page.Items, PlaylistEntry{Track: track, Position: i})
	}
	if end < len(playlist.Tracks) {
		page.Next = "spotify-player"
	}
	return page, nil
}
