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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VictorTrab/SpotyGo/internal/logger"
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

type Image struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type Album struct {
	Name        string  `json:"name"`
	ReleaseDate string  `json:"release_date"`
	Images      []Image `json:"images"`
}

type Artist struct {
	Name string `json:"name"`
}

type Track struct {
	URI        string   `json:"uri"`
	Name       string   `json:"name"`
	Artists    []Artist `json:"artists"`
	Album      Album    `json:"album"`
	DurationMS int      `json:"duration_ms"`
	Popularity int      `json:"popularity"`
}

// ReleaseYear returns the 4-digit release year of the track's album, or empty string.
func (t Track) ReleaseYear() string {
	if len(t.Album.ReleaseDate) >= 4 {
		return t.Album.ReleaseDate[:4]
	}
	return ""
}

// CoverURL returns the best album cover image URL (preferring medium size ~300px or fallback to first image).
func (t Track) CoverURL() string {
	if len(t.Album.Images) == 0 {
		return ""
	}
	for _, img := range t.Album.Images {
		if img.Width >= 200 && img.Width <= 400 && img.URL != "" {
			return img.URL
		}
	}
	return t.Album.Images[0].URL
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

const LikedTracksID = "user-liked-tracks"

func (c *Client) Playlists(ctx context.Context, offset int) (PlaylistPage, error) {
	if offset < 0 {
		return PlaylistPage{}, errors.New("offset de playlists inválido")
	}
	query := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}, "market": {"from_token"}}
	var page PlaylistPage
	_, err := c.request(ctx, http.MethodGet, "/me/playlists?"+query.Encode(), nil, &page)
	if err != nil {
		return page, err
	}
	if offset == 0 {
		liked := Playlist{
			ID:   LikedTracksID,
			URI:  "spotify:user-liked-tracks",
			Name: "♥ Canciones que te gustan",
		}
		page.Items = append([]Playlist{liked}, page.Items...)
		page.Total++
	}
	return page, nil
}

func (c *Client) PlaylistEntries(ctx context.Context, playlistID string, offset int) (PlaylistEntriesPage, error) {
	if playlistID == "" || offset < 0 {
		return PlaylistEntriesPage{}, errors.New("playlist u offset inválido")
	}
	if playlistID == LikedTracksID {
		query := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}, "market": {"from_token"}}
		var response struct {
			Items []struct {
				Track Track `json:"track"`
			} `json:"items"`
			Total int    `json:"total"`
			Next  string `json:"next"`
		}
		_, err := c.request(ctx, http.MethodGet, "/me/tracks?"+query.Encode(), nil, &response)
		if err != nil {
			return PlaylistEntriesPage{}, err
		}
		page := PlaylistEntriesPage{Total: response.Total, Next: response.Next}
		for i, entry := range response.Items {
			if strings.HasPrefix(entry.Track.URI, "spotify:track:") {
				page.Items = append(page.Items, PlaylistEntry{Track: entry.Track, Position: offset + i})
				c.CacheTrack(entry.Track)
			}
		}
		return page, nil
	}
	query := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}, "market": {"from_token"}}
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
			c.CacheTrack(*track)
		}
	}
	return page, nil
}

func (c *Client) PlayPlaylist(ctx context.Context, playlist Playlist, position int, deviceID string) error {
	if playlist.ID == "" || deviceID == "" || position < 0 {
		return errors.New("falta una playlist, canción o dispositivo")
	}
	if playlist.ID == LikedTracksID {
		tracksPage, err := c.PlaylistEntries(ctx, playlist.ID, position)
		if err != nil {
			return err
		}
		var uris []string
		for _, item := range tracksPage.Items {
			uris = append(uris, item.Track.URI)
		}
		if len(uris) == 0 {
			return errors.New("no hay canciones en Canciones que te gustan")
		}
		query := url.Values{"device_id": {deviceID}}
		_, err = c.request(ctx, http.MethodPut, "/me/player/play?"+query.Encode(), map[string]any{"uris": uris}, nil)
		return err
	}
	uri := playlist.URI
	if uri == "" {
		uri = "spotify:playlist:" + playlist.ID
	}
	query := url.Values{"device_id": {deviceID}}
	path := "/me/player/play?" + query.Encode()
	payload := map[string]any{
		"context_uri": uri,
		"offset":      map[string]int{"position": position},
	}
	status, err := c.request(ctx, http.MethodPut, path, payload, nil)
	if status >= 502 && status <= 504 {
		if waitErr := waitUntil(ctx, time.Now().Add(400*time.Millisecond)); waitErr != nil {
			return waitErr
		}
		_, err = c.request(ctx, http.MethodPut, path, payload, nil)
	}
	return err
}

func (c *Client) SearchTracks(ctx context.Context, query string) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("escribe una búsqueda")
	}
	values := url.Values{"q": {query}, "type": {"track"}, "limit": {"10"}}
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

	trackCacheMu sync.RWMutex
	trackCache   map[string]Track
	diskCacheDir string
}

// Close cleans up resources if any.
func (c *Client) Close() {}

// IsBlocked reports whether Spotify rate limit backoff is currently active.
func (c *Client) IsBlocked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blockedUntil.After(time.Now())
}

func NewClient(auth *Auth) *Client {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	cacheDir := filepath.Join(dir, "SpotifyGo", "cache", "tracks")
	_ = os.MkdirAll(cacheDir, 0755)

	return &Client{
		auth:         auth,
		http:         &http.Client{Timeout: 12 * time.Second},
		trackCache:   make(map[string]Track),
		diskCacheDir: cacheDir,
	}
}

// CacheTrack stores a track's metadata into memory and disk cache if it has complete metadata.
func (c *Client) CacheTrack(track Track) {
	if track.URI == "" || track.Name == "" || len(track.Artists) == 0 || len(track.Album.Images) == 0 {
		return
	}
	id := strings.TrimPrefix(track.URI, "spotify:track:")
	c.trackCacheMu.Lock()
	if c.trackCache == nil {
		c.trackCache = make(map[string]Track)
	}
	c.trackCache[id] = track
	c.trackCache[track.URI] = track
	c.trackCacheMu.Unlock()

	if c.diskCacheDir != "" {
		filePath := filepath.Join(c.diskCacheDir, id+".json")
		if data, err := json.Marshal(track); err == nil {
			_ = os.WriteFile(filePath, data, 0644)
		}
	}
}

// GetCachedTrack returns a track from memory or disk cache if available.
func (c *Client) GetCachedTrack(idOrURI string) (Track, bool) {
	id := strings.TrimPrefix(idOrURI, "spotify:track:")
	if idx := strings.LastIndex(id, "/"); idx != -1 {
		id = id[idx+1:]
	}
	if idx := strings.Index(id, "?"); idx != -1 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Track{}, false
	}

	// 1. In-memory lookup (0.00 ms)
	c.trackCacheMu.RLock()
	if cached, ok := c.trackCache[id]; ok && cached.Name != "" && len(cached.Artists) > 0 && len(cached.Album.Images) > 0 {
		c.trackCacheMu.RUnlock()
		return cached, true
	}
	c.trackCacheMu.RUnlock()

	// 2. Disk cache lookup (~1 ms)
	if c.diskCacheDir != "" {
		filePath := filepath.Join(c.diskCacheDir, id+".json")
		if data, err := os.ReadFile(filePath); err == nil {
			var cached Track
			if err := json.Unmarshal(data, &cached); err == nil && cached.Name != "" && len(cached.Artists) > 0 {
				c.trackCacheMu.Lock()
				if c.trackCache == nil {
					c.trackCache = make(map[string]Track)
				}
				c.trackCache[id] = cached
				c.trackCache[cached.URI] = cached
				c.trackCacheMu.Unlock()
				return cached, true
			}
		}
	}
	return Track{}, false
}

// Reauth launches the Spotify OAuth browser authorization flow to re-link or switch accounts.
func (c *Client) Reauth(ctx context.Context) error {
	if c.auth == nil {
		return errors.New("autenticación no inicializada")
	}
	return c.auth.authorize(ctx)
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
	c.next = start.Add(150 * time.Millisecond)
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
	for attempt := 0; attempt < 3; attempt++ {
		token, err := c.auth.AccessToken(ctx)
		if err != nil {
			logger.Error("Error al obtener token de Spotify para %s %s: %v", method, path, err)
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
			logger.Error("Error de red en solicitud %s %s: %v", method, path, err)
			return 0, fmt.Errorf("solicitar %s: %w", path, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			wait := retryDelay(resp.Header.Get("Retry-After"))
			logger.Warn("Spotify 429 Too Many Requests en %s %s; Retry-After=%s (intento %d/3)", method, path, wait, attempt+1)
			c.mu.Lock()
			until := time.Now().Add(wait)
			if until.After(c.blockedUntil) {
				c.blockedUntil = until
			}
			c.mu.Unlock()
			if attempt < 2 {
				if waitErr := waitUntil(ctx, until); waitErr != nil {
					return resp.StatusCode, waitErr
				}
				continue
			}
			logger.Error("Spotify 429 agotó reintentos en %s %s", method, path)
			return resp.StatusCode, fmt.Errorf("Spotify limitó las solicitudes (429); reintentar después de %s", wait.Round(time.Second))
		}
		defer resp.Body.Close()
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
			logger.Error("Spotify API HTTP %d en %s %s: %s", resp.StatusCode, method, path, message)
			return resp.StatusCode, fmt.Errorf("Spotify HTTP %d: %s", resp.StatusCode, message)
		}
		if output != nil && resp.StatusCode != http.StatusNoContent {
			if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(output); err != nil {
				return resp.StatusCode, fmt.Errorf("leer respuesta de Spotify: %w", err)
			}
		}
		return resp.StatusCode, nil
	}
	return 0, errors.New("reintentos de solicitud agotados")
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
	body := map[string]any{
		"device_ids": []string{device.ID},
		"play":       true,
	}
	_, err := c.request(ctx, http.MethodPut, "/me/player", body, nil)
	return err
}

// GetTrack fetches full metadata for a track by ID or Spotify URI (including artists, album name, release date, and cover images).
func (c *Client) GetTrack(ctx context.Context, idOrURI string) (Track, error) {
	var track Track
	id := strings.TrimPrefix(idOrURI, "spotify:track:")
	if idx := strings.LastIndex(id, "/"); idx != -1 {
		id = id[idx+1:]
	}
	if idx := strings.Index(id, "?"); idx != -1 {
		id = id[:idx]
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return track, errors.New("track ID vacío")
	}

	if cached, ok := c.GetCachedTrack(id); ok {
		return cached, nil
	}

	_, err := c.request(ctx, http.MethodGet, "/tracks/"+url.PathEscape(id), nil, &track)
	if err != nil {
		return track, err
	}
	c.CacheTrack(track)
	return track, nil
}
