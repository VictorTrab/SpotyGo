package spotify

import (
	"encoding/json"
	"testing"
)

func TestPlayerPlaylistPageKeepsTrackPositions(t *testing.T) {
	var playlist playerPlaylist
	if err := json.Unmarshal([]byte(`{"tracks":[{"id":"one","name":"Primera","artists":[{"name":"Artista"}],"duration":{"secs":123,"nanos":456000000}},{"id":"two","name":"Segunda","duration":{"secs":45,"nanos":0}}]}`), &playlist); err != nil {
		t.Fatal(err)
	}
	page, err := playerPlaylistPage(playlist, 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Position != 1 || page.Items[0].Track.URI != "spotify:track:two" {
		t.Fatalf("incorrect playlist page: %+v", page)
	}
	first, err := playerPlaylistPage(playlist, 0)
	if err != nil || first.Items[0].Track.DurationMS != 123456 || first.Items[0].Track.Artists[0].Name != "Artista" {
		t.Fatalf("incorrect track metadata: %+v, %v", first, err)
	}
}
