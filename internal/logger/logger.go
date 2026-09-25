package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

// Init initializes the persistent log file in the user's cache directory under SpotifyGo.
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		return nil
	}

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}

	dir := filepath.Join(cacheRoot, "SpotifyGo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	logPath := filepath.Join(dir, "spotifygo.log")
	if fi, err := os.Stat(logPath); err == nil && fi.Size() > 5*1024*1024 {
		oldPath := filepath.Join(dir, "spotifygo.log.old")
		_ = os.Remove(oldPath)
		_ = os.Rename(logPath, oldPath)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	logFile = f
	writeEntry("INFO", "=== SpotifyGo logger initialized ===")
	return nil
}

// LogFilePath returns the path of the current log file.
func LogFilePath() string {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}
	return filepath.Join(cacheRoot, "SpotifyGo", "spotifygo.log")
}

func writeEntry(level, msg string) {
	if logFile == nil {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [%s] %s\n", timestamp, level, msg)
	_, _ = logFile.WriteString(entry)
}

// Info logs an informational message.
func Info(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	writeEntry("INFO", fmt.Sprintf(format, args...))
}

// Warn logs a warning message.
func Warn(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	writeEntry("WARN", fmt.Sprintf(format, args...))
}

// Error logs an error message.
func Error(format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	writeEntry("ERROR", fmt.Sprintf(format, args...))
}

// Close closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		writeEntry("INFO", "=== SpotifyGo session closed ===")
		_ = logFile.Close()
		logFile = nil
	}
}
