package logger

import (
	"os"
	"strings"
	"testing"
)

// TestMain keeps the suite out of the real user log file.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "spotygo-logger-test")
	if err != nil {
		os.Exit(m.Run())
	}
	previous, hadPrevious := os.LookupEnv("LOCALAPPDATA")
	_ = os.Setenv("LOCALAPPDATA", dir)

	code := m.Run()

	if hadPrevious {
		_ = os.Setenv("LOCALAPPDATA", previous)
	} else {
		_ = os.Unsetenv("LOCALAPPDATA")
	}
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestLogger(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer Close()

	Info("Test info message %d", 42)
	Warn("Test warn message")
	Error("Test error message")

	content, err := os.ReadFile(LogFilePath())
	if err != nil {
		t.Fatalf("failed reading log file: %v", err)
	}

	str := string(content)
	if !strings.Contains(str, "Test info message 42") {
		t.Errorf("expected log to contain info message, got:\n%s", str)
	}
	if !strings.Contains(str, "Test warn message") {
		t.Errorf("expected log to contain warn message, got:\n%s", str)
	}
	if !strings.Contains(str, "Test error message") {
		t.Errorf("expected log to contain error message, got:\n%s", str)
	}
}
