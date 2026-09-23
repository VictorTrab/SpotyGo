package player

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Engine owns the local Spotify Connect receiver for the life of the TUI.
type Engine struct {
	cmd  *exec.Cmd
	log  *os.File
	Name string
	Done chan error
}

func binary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("SPOTYGO_LIBRESPOT")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("SPOTYGO_LIBRESPOT: %w", err)
		}
		return configured, nil
	}
	if dir, err := os.UserCacheDir(); err == nil {
		name := "librespot"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		path := filepath.Join(dir, "SpotyGo", "librespot", "bin", name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("librespot"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("falta el motor de audio librespot; ejecuta scripts/install.ps1 desde el proyecto")
}

func Start() (*Engine, error) {
	path, err := binary()
	if err != nil {
		return nil, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(cacheRoot, "SpotyGo")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(root, "librespot.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return nil, err
	}
	computer, _ := os.Hostname()
	if computer == "" {
		computer = "PC"
	}
	name := "SpotyGo (" + computer + ")"
	cmd := exec.Command(path, "--name", name, "--device-type", "computer", "--cache", filepath.Join(root, "cache"), "--enable-oauth", "--backend", "rodio", "--bitrate", "160", "--quiet")
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		log.Close()
		return nil, fmt.Errorf("iniciar librespot: %w", err)
	}
	engine := &Engine{cmd: cmd, log: log, Name: name, Done: make(chan error, 1)}
	go func() {
		engine.Done <- cmd.Wait()
		log.Close()
	}()
	return engine, nil
}

func (e *Engine) Stop() {
	if e != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
}
