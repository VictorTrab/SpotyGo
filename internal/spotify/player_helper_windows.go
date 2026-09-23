//go:build windows

package spotify

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func startSpotifyPlayerHelper(exe string) (func(), error) {
	command := exec.Command("powershell.exe", "-NoProfile", "-Command", "Start-Process -FilePath $env:SPOTYGO_PLAYER_EXE -WindowStyle Hidden -PassThru | Select-Object -ExpandProperty Id")
	command.Env = append(os.Environ(), "SPOTYGO_PLAYER_EXE="+exe)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("iniciar spotify-player: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return nil, fmt.Errorf("pid inválido de spotify-player: %w", err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	return func() {
		_ = process.Kill()
		_ = process.Release()
	}, nil
}
