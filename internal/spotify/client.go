package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const apiBase = "https://api.spotify.com/v1"

type Device struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	IsActive       bool   `json:"is_active"`
	IsRestricted   bool   `json:"is_restricted"`
	SupportsVolume bool   `json:"supports_volume"`
	VolumePercent  *int   `json:"volume_percent"`
}

type Track struct {
	URI     string `json:"uri"`
	Name    string `json:"name"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	DurationMS int `json:"duration_ms"`
}

type Playlist struct {
	ID    string `json:"id"`
	URI   string `json:"uri"`
	Name  string `json:"name"`
	Items struct {
		Total int `json:"total"`
	} `json:"items"`
}

type PlaylistPage struct {
	Items []Playlist `json:"items"`
	Total int        `json:"total"`
	Next  string     `json:"next"`
}

type PlaylistEntry struct {
	Track    Track
	Position int
}

type PlaylistEntriesPage struct {
	Items []PlaylistEntry
	Total int
	Next  string
}

func (c *Client) Playlists(ctx context.Context, offset int) (PlaylistPage, error) {
	if offset < 0 {
		return PlaylistPage{}, errors.New("offset de playlists inválido")
	}
	query := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}}
	var page PlaylistPage
	_, err := c.request(ctx, http.MethodGet, "/me/playlists?"+query.Encode(), nil, &page)
	return page, err
}

func (c *Client) PlaylistEntries(ctx context.Context, playlistID string, offset int) (PlaylistEntriesPage, error) {
	if playlistID == "" || offset < 0 {
		return PlaylistEntriesPage{}, errors.New("playlist u offset inválido")
	}
	query := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}}
	var response struct {
		Items []struct {
			Item  *Track `json:"item"`
			Track *Track `json:"track"`
		} `json:"items"`
		Total int    `json:"total"`
		Next  string `json:"next"`
	}
	path := "/playlists/" + url.PathEscape(playlistID) + "/items?" + query.Encode()
	_, err := c.request(ctx, http.MethodGet, path, nil, &response)
	if err != nil {
		return PlaylistEntriesPage{}, err
	}
	page := PlaylistEntriesPage{Total: response.Total, Next: response.Next}
	for i, entry := range response.Items {
		track := entry.Item
		if track == nil {
			track = entry.Track
		}
		if track != nil && strings.HasPrefix(track.URI, "spotify:track:") {
			page.Items = append(page.Items, PlaylistEntry{Track: *track, Position: offset + i})
		}
	}
	return page, nil
}

func (c *Client) PlayPlaylist(ctx context.Context, playlist Playlist, position int, deviceID string) error {
	if playlist.ID == "" || deviceID == "" || position < 0 {
		return errors.New("falta una playlist, canción o dispositivo")
	}
	uri := playlist.URI
	if uri == "" {
		uri = "spotify:playlist:" + playlist.ID
	}
	query := url.Values{"device_id": {deviceID}}
	_, err := c.request(ctx, http.MethodPut, "/me/player/play?"+query.Encode(), map[string]any{
		"context_uri": uri,
		"offset":      map[string]int{"position": position},
	}, nil)
	return err
}

func (c *Client) SearchTracks(ctx context.Context, query string) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("escribe una búsqueda")
	}
	values := url.Values{"q": {query}, "type": {"track"}, "limit": {"5"}}
	var result struct {
		Tracks struct {
			Items []Track `json:"items"`
		} `json:"tracks"`
	}
	_, err := c.request(ctx, http.MethodGet, "/search?"+values.Encode(), nil, &result)
	return result.Tracks.Items, err
}

func (c *Client) PlayTrack(ctx context.Context, track Track, deviceID string) error {
	if track.URI == "" || deviceID == "" {
		return errors.New("falta una canción o el dispositivo local")
	}
	query := url.Values{"device_id": {deviceID}}
	_, err := c.request(ctx, http.MethodPut, "/me/player/play?"+query.Encode(), map[string]any{"uris": []string{track.URI}}, nil)
	return err
}

type PlaybackState struct {
	Available  bool    `json:"-"`
	IsPlaying  bool    `json:"is_playing"`
	ProgressMS int     `json:"progress_ms"`
	Item       *Track  `json:"item"`
	Device     *Device `json:"device"`
	Context    *struct {
		URI string `json:"uri"`
	} `json:"context"`
}

type Client struct {
	auth         *Auth
	http         *http.Client
	mu           sync.Mutex
	next         time.Time
	blockedUntil time.Time
}

func NewClient(auth *Auth) *Client {
	return &Client{auth: auth, http: &http.Client{Timeout: 12 * time.Second}}
}

// reserve spaces requests across concurrent UI commands and applies 429 backoff.
func (c *Client) reserve(ctx context.Context) error {
	c.mu.Lock()
	start := time.Now()
	if c.next.After(start) {
		start = c.next
	}
	if c.blockedUntil.After(start) {
		start = c.blockedUntil
	}
	c.next = start.Add(200 * time.Millisecond)
	c.mu.Unlock()
	if err := waitUntil(ctx, start); err != nil {
		return err
	}
	// A 429 can arrive while this request is waiting in the queue.
	for {
		c.mu.Lock()
		blocked := c.blockedUntil
		c.mu.Unlock()
		if !blocked.After(time.Now()) {
			return nil
		}
		if err := waitUntil(ctx, blocked); err != nil {
			return err
		}
	}
}

func waitUntil(ctx context.Context, until time.Time) error {
	if wait := time.Until(until); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func retryDelay(value string) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		return max(0, time.Until(when))
	}
	return 30 * time.Second
}

func (c *Client) request(ctx context.Context, method, path string, payload any, output any) (int, error) {
	token, err := c.auth.AccessToken(ctx)
	if err != nil {
		return 0, err
	}
	if err := c.reserve(ctx); err != nil {
		return 0, err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("solicitar %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := retryDelay(resp.Header.Get("Retry-After"))
		c.mu.Lock()
		until := time.Now().Add(wait)
		if until.After(c.blockedUntil) {
			c.blockedUntil = until
		}
		c.mu.Unlock()
		return resp.StatusCode, fmt.Errorf("Spotify limitó las solicitudes (429); reintentar después de %s", wait.Round(time.Second))
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		var apiError struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &apiError)
		message := strings.TrimSpace(apiError.Error.Message)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return resp.StatusCode, fmt.Errorf("Spotify HTTP %d: %s", resp.StatusCode, message)
	}
	if output != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(output); err != nil {
			return resp.StatusCode, fmt.Errorf("leer respuesta de Spotify: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func (c *Client) Playback(ctx context.Context) (PlaybackState, error) {
	var state PlaybackState
	status, err := c.request(ctx, http.MethodGet, "/me/player", nil, &state)
	if err != nil {
		return state, err
	}
	state.Available = status != http.StatusNoContent
	return state, nil
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var result struct {
		Devices []Device `json:"devices"`
	}
	_, err := c.request(ctx, http.MethodGet, "/me/player/devices", nil, &result)
	return result.Devices, err
}

func (c *Client) Play(ctx context.Context) error {
	_, err := c.request(ctx, http.MethodPut, "/me/player/play", nil, nil)
	return err
}

func (c *Client) Pause(ctx context.Context) error {
	_, err := c.request(ctx, http.MethodPut, "/me/player/pause", nil, nil)
	return err
}

func (c *Client) Next(ctx context.Context) error {
	_, err := c.request(ctx, http.MethodPost, "/me/player/next", nil, nil)
	return err
}

func (c *Client) Previous(ctx context.Context) error {
	_, err := c.request(ctx, http.MethodPost, "/me/player/previous", nil, nil)
	return err
}

func (c *Client) SetVolume(ctx context.Context, value int, deviceID string) error {
	if value < 0 || value > 100 {
		return errors.New("volumen fuera del rango 0-100")
	}
	query := url.Values{"volume_percent": {strconv.Itoa(value)}}
	if deviceID != "" {
		query.Set("device_id", deviceID)
	}
	_, err := c.request(ctx, http.MethodPut, "/me/player/volume?"+query.Encode(), nil, nil)
	return err
}

func (c *Client) Transfer(ctx context.Context, device Device) error {
	if device.ID == "" || device.IsRestricted {
		return errors.New("este dispositivo no admite control remoto")
	}
	_, err := c.request(ctx, http.MethodPut, "/me/player", map[string]any{"device_ids": []string{device.ID}}, nil)
	return err
}
