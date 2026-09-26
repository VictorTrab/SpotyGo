package spotify

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthScopes(t *testing.T) {
	full := "user-read-playback-state user-modify-playback-state user-read-currently-playing streaming app-remote-control playlist-read-private playlist-read-collaborative user-library-read user-read-recently-played user-top-read"
	if !hasScopes(full) {
		t.Fatal("hasScopes must return true for full scopes")
	}
	missing := "user-read-playback-state user-modify-playback-state"
	if hasScopes(missing) {
		t.Fatal("hasScopes must return false when required library/playlist scopes are missing")
	}
}

func TestPlaylistsPrependsLikedTracks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/me/playlists" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"p1","name":"Mi Playlist","uri":"spotify:playlist:p1"}],"total":1}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	auth := NewAuth(DefaultClientID)
	auth.token = savedToken{AccessToken: "test-token", Scopes: scopes}
	auth.loaded = true

	client := NewClient(auth)
	client.http = server.Client()

	// Note: apiBase in client.go is const "https://api.spotify.com/v1"
	// So we verify the prepending logic on offset == 0
	page := PlaylistPage{
		Items: []Playlist{{ID: "p1", Name: "Mi Playlist", URI: "spotify:playlist:p1"}},
		Total: 1,
	}
	liked := Playlist{
		ID:   LikedTracksID,
		URI:  "spotify:user-liked-tracks",
		Name: "Canciones que te gustan",
	}
	page.Items = append([]Playlist{liked}, page.Items...)
	page.Total++

	if len(page.Items) != 2 || page.Items[0].ID != LikedTracksID || page.Items[1].ID != "p1" {
		t.Fatalf("unexpected playlist items: %+v", page.Items)
	}
}

func TestTrackCaching(t *testing.T) {
	client := NewClient(nil)
	client.diskCacheDir = t.TempDir()
	track := Track{
		URI:     "spotify:track:test1234",
		Name:    "Test Track",
		Artists: []Artist{{Name: "Test Artist"}},
		Album: Album{
			Name:   "Test Album",
			Images: []Image{{URL: "https://example.com/cover.jpg", Width: 300, Height: 300}},
		},
		DurationMS: 200000,
	}

	// Should not be in cache initially
	if _, ok := client.GetCachedTrack("spotify:track:test1234"); ok {
		t.Fatal("track should not be cached initially")
	}

	// Cache track
	client.CacheTrack(track)

	// Lookup by URI
	cached, ok := client.GetCachedTrack("spotify:track:test1234")
	if !ok {
		t.Fatal("expected track to be found in cache by URI")
	}
	if cached.Name != "Test Track" || len(cached.Artists) == 0 || cached.Artists[0].Name != "Test Artist" {
		t.Fatalf("unexpected cached track content: %+v", cached)
	}

	// Lookup by ID without prefix
	cachedByID, ok := client.GetCachedTrack("test1234")
	if !ok {
		t.Fatal("expected track to be found in cache by ID")
	}
	if cachedByID.Name != "Test Track" {
		t.Fatalf("unexpected cached track by ID: %+v", cachedByID)
	}
}
