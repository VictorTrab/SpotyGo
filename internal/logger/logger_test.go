package logger

import (
	"os"
	"strings"
	"testing"
)

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
