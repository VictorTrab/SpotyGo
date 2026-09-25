package player

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/VictorTrab/SpotyGo/internal/logger"
)

// TrackEvent represents a real-time track change event from librespot.
type TrackEvent struct {
	URI        string
	Name       string
	DurationMS int
}

// Engine owns the local Spotify Connect receiver for the life of the TUI.
type Engine struct {
	cmd          *exec.Cmd
	log          *os.File
	Name         string
	Bitrate      string
	Done         chan error
	Events       chan struct{}
	VolumeEvents chan int
	TrackEvents  chan TrackEvent
	currentTrack TrackEvent
	path         string
	root         string
	restarting   bool
	mu           sync.Mutex
}

func binary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("SPOTIFYGO_LIBRESPOT")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("SPOTIFYGO_LIBRESPOT: %w", err)
		}
		return configured, nil
	}
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
		// First check SpotifyGo path
		path := filepath.Join(dir, "SpotifyGo", "librespot", "bin", name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		// Fallback to legacy SpotyGo path
		pathLegacy := filepath.Join(dir, "SpotyGo", "librespot", "bin", name)
		if _, err := os.Stat(pathLegacy); err == nil {
			return pathLegacy, nil
		}
	}
	if path, err := exec.LookPath("librespot"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("falta el motor de audio librespot; ejecuta scripts/install.ps1 desde el proyecto")
}

func Start(optBitrate ...string) (*Engine, error) {
	bitrate := "320"
	if len(optBitrate) > 0 && optBitrate[0] != "" {
		bitrate = optBitrate[0]
	}
	path, err := binary()
	if err != nil {
		logger.Error("No se encontró el ejecutable de librespot: %v", err)
		return nil, err
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}
	root := filepath.Join(cacheRoot, "SpotifyGo")
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(filepath.Join(root, "librespot.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		logger.Error("No se pudo abrir librespot.log: %v", err)
		return nil, err
	}
	computer, _ := os.Hostname()
	if computer == "" {
		computer = "PC"
	}
	name := "SpotifyGo (" + computer + ")"
	logger.Info("Iniciando motor de audio local librespot: %s con nombre %s (bitrate: %s kbps)", path, name, bitrate)
	cmd := exec.Command(path, "--name", name, "--device-type", "computer", "--cache", filepath.Join(root, "cache"), "--enable-oauth", "--backend", "rodio", "--bitrate", bitrate, "--volume-ctrl", "linear")
	cmd.Stdout = log
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		log.Close()
		logger.Error("Error creando pipe de stderr para librespot: %v", err)
		return nil, fmt.Errorf("crear pipe de librespot: %w", err)
	}

	if err := cmd.Start(); err != nil {
		log.Close()
		logger.Error("Error al iniciar proceso librespot: %v", err)
		return nil, fmt.Errorf("iniciar librespot: %w", err)
	}

	events := make(chan struct{}, 10)
	volumeEvents := make(chan int, 10)
	trackEvents := make(chan TrackEvent, 10)
	engine := &Engine{
		cmd:          cmd,
		log:          log,
		Name:         name,
		Bitrate:      bitrate,
		Done:         make(chan error, 1),
		Events:       events,
		VolumeEvents: volumeEvents,
		TrackEvents:  trackEvents,
		path:         path,
		root:         root,
	}

	// Read stderr to capture live playback, volume, and track events and write to log file
	go engine.monitor(stderrPipe)

	go func() {
		waitErr := cmd.Wait()
		engine.mu.Lock()
		if engine.restarting {
			engine.mu.Unlock()
			return
		}
		engine.mu.Unlock()

		if waitErr != nil {
			logger.Warn("Proceso librespot finalizó con: %v", waitErr)
		}
		close(events)
		close(volumeEvents)
		close(trackEvents)
		engine.Done <- waitErr
		log.Close()
	}()
	return engine, nil
}

func (e *Engine) monitor(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(e.log, line)

		// Real-time local volume event from librespot
		if vol := parseVolume(line); vol >= 0 {
			select {
			case e.VolumeEvents <- vol:
			default:
				select {
				case <-e.VolumeEvents:
				default:
				}
				e.VolumeEvents <- vol
			}
		}

		// Real-time local track event from librespot
		if name, uri, ok := parseTrackLoading(line); ok {
			e.currentTrack = TrackEvent{URI: uri, Name: name}
			select {
			case e.TrackEvents <- e.currentTrack:
			default:
				select {
				case <-e.TrackEvents:
				default:
				}
				e.TrackEvents <- e.currentTrack
			}
		} else if name, dur, ok := parseTrackLoaded(line); ok {
			if e.currentTrack.Name == "" || strings.Contains(strings.ToLower(name), strings.ToLower(e.currentTrack.Name)) || strings.Contains(strings.ToLower(e.currentTrack.Name), strings.ToLower(name)) {
				e.currentTrack.DurationMS = dur
				if e.currentTrack.Name == "" {
					e.currentTrack.Name = name
				}
			} else {
				e.currentTrack = TrackEvent{Name: name, DurationMS: dur}
			}
			select {
			case e.TrackEvents <- e.currentTrack:
			default:
				select {
				case <-e.TrackEvents:
				default:
				}
				e.TrackEvents <- e.currentTrack
			}
		}

		lower := strings.ToLower(line)
		// Ignore internal startup/diagnostic logs
		if strings.Contains(lower, "::mixer") ||
			strings.Contains(lower, "::convert") ||
			strings.Contains(lower, "::audio_backend") ||
			strings.Contains(lower, "::session") ||
			strings.Contains(lower, "::spclient") ||
			strings.Contains(lower, "authenticat") ||
			strings.Contains(lower, "connecting to") {
			continue
		}

		if strings.Contains(lower, "action: play") ||
			strings.Contains(lower, "action: pause") ||
			strings.Contains(lower, "action: stop") ||
			strings.Contains(lower, "action: next") ||
			strings.Contains(lower, "action: prev") ||
			strings.Contains(lower, "end of track") ||
			strings.Contains(lower, "track_ended") {
			select {
			case e.Events <- struct{}{}:
			default:
			}
		}
	}
}

// Restart restarts the librespot engine process with the requested audio bitrate.
func (e *Engine) Restart(bitrate string) error {
	if e == nil {
		return fmt.Errorf("engine no inicializado")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if bitrate != "160" && bitrate != "320" && bitrate != "96" {
		bitrate = "320"
	}

	e.restarting = true
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		_ = e.cmd.Wait()
	}

	logger.Info("Reiniciando motor librespot con bitrate: %s kbps", bitrate)
	cmd := exec.Command(e.path, "--name", e.Name, "--device-type", "computer", "--cache", filepath.Join(e.root, "cache"), "--enable-oauth", "--backend", "rodio", "--bitrate", bitrate, "--volume-ctrl", "linear")
	cmd.Stdout = e.log
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		e.restarting = false
		logger.Error("Error creando pipe de stderr al reiniciar librespot: %v", err)
		return fmt.Errorf("crear pipe de librespot: %w", err)
	}

	if err := cmd.Start(); err != nil {
		e.restarting = false
		logger.Error("Error al iniciar librespot tras reinicio: %v", err)
		return fmt.Errorf("iniciar librespot: %w", err)
	}

	e.cmd = cmd
	e.Bitrate = bitrate
	e.restarting = false

	go e.monitor(stderrPipe)

	go func() {
		waitErr := cmd.Wait()
		e.mu.Lock()
		if e.restarting {
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()

		if waitErr != nil {
			logger.Warn("Proceso librespot finalizó con: %v", waitErr)
		}
		close(e.Events)
		close(e.VolumeEvents)
		close(e.TrackEvents)
		e.Done <- waitErr
		e.log.Close()
	}()

	return nil
}

// parseVolume extracts the volume percentage (0-100) from a librespot log line,
// or returns -1 if the line does not contain a volume update event.
func parseVolume(line string) int {
	lower := strings.ToLower(line)
	idx := strings.Index(lower, "volume is now")
	if idx == -1 {
		return -1
	}
	rest := strings.TrimLeft(strings.TrimSpace(lower[idx+len("volume is now"):]), ":= ")
	var digits strings.Builder
	for _, r := range rest {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		} else {
			break
		}
	}
	if digits.Len() == 0 {
		return -1
	}
	val, err := strconv.Atoi(digits.String())
	if err != nil {
		return -1
	}
	var percent int
	if val > 100 {
		// Librespot / Spotify Connect uses 16-bit volume (0..65535)
		percent = int(math.Round(float64(val) * 100.0 / 65535.0))
	} else {
		// Value is already in 0..100 range
		percent = val
	}
	if percent > 100 {
		percent = 100
	} else if percent < 0 {
		percent = 0
	}
	return percent
}

func (e *Engine) Stop() {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
}

// parseTrackLoading extracts the track name and Spotify URI from librespot log lines like:
// "Loading <Stay the Night> with Spotify URI <spotify:track:7zJnmSjZKjntHmOvEokGb3>"
// or "Loading "Stay the Night" with Spotify URI "spotify:track:7zJnmSjZKjntHmOvEokGb3""
func parseTrackLoading(line string) (name, uri string, ok bool) {
	// Look for Spotify URI
	uriIdx := strings.Index(line, "spotify:track:")
	if uriIdx != -1 {
		rest := line[uriIdx:]
		end := strings.IndexAny(rest, ">\"' \t\r\n")
		if end != -1 {
			uri = rest[:end]
		} else {
			uri = rest
		}
	}

	// Look for track name enclosed in <...> or "..."
	loadIdx := strings.Index(line, "Loading <")
	if loadIdx != -1 {
		endIdx := strings.Index(line[loadIdx+len("Loading <"):], ">")
		if endIdx != -1 {
			name = line[loadIdx+len("Loading <") : loadIdx+len("Loading <")+endIdx]
		}
	} else if loadIdx := strings.Index(line, "Loading \""); loadIdx != -1 {
		endIdx := strings.Index(line[loadIdx+len("Loading \""):], "\"")
		if endIdx != -1 {
			name = line[loadIdx+len("Loading \"") : loadIdx+len("Loading \"")+endIdx]
		}
	} else if loadIdx := strings.Index(line, "loading track \""); loadIdx != -1 {
		endIdx := strings.Index(line[loadIdx+len("loading track \""):], "\"")
		if endIdx != -1 {
			name = line[loadIdx+len("loading track \"") : loadIdx+len("loading track \"")+endIdx]
		}
	}

	if uri != "" || name != "" {
		return strings.TrimSpace(name), strings.TrimSpace(uri), true
	}
	return "", "", false
}

// parseTrackLoaded extracts the track name and duration from librespot log lines like:
// "<Stay the Night> (264893 ms) loaded"
func parseTrackLoaded(line string) (name string, durationMS int, ok bool) {
	loadedIdx := strings.Index(line, " ms) loaded")
	if loadedIdx == -1 {
		loadedIdx = strings.Index(line, "ms) loaded")
	}
	if loadedIdx == -1 {
		return "", 0, false
	}
	openParen := strings.LastIndex(line[:loadedIdx], "(")
	if openParen == -1 {
		return "", 0, false
	}
	durStr := strings.TrimSpace(line[openParen+1 : loadedIdx])
	dur, err := strconv.Atoi(durStr)
	if err != nil || dur <= 0 {
		return "", 0, false
	}
	nameStart := strings.LastIndex(line[:openParen], "<")
	nameEnd := strings.LastIndex(line[:openParen], ">")
	if nameStart != -1 && nameEnd != -1 && nameEnd > nameStart {
		name = strings.TrimSpace(line[nameStart+1 : nameEnd])
	} else {
		nameStart = strings.LastIndex(line[:openParen], "\"")
		if nameStart != -1 {
			firstQuote := strings.LastIndex(line[:nameStart], "\"")
			if firstQuote != -1 {
				name = strings.TrimSpace(line[firstQuote+1 : nameStart])
			}
		}
	}
	return name, dur, true
}

