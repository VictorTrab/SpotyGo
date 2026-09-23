//go:build !windows

package spotify

import "errors"

func startSpotifyPlayerHelper(string) (func(), error) {
	return nil, errors.New("spotify-player debe estar abierto para leer esta playlist")
}
