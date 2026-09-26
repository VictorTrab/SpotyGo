package ui

import (
	"os"
	"testing"
)

// TestMain isolates every UI test from the developer's real preferences.
//
// New() reads theme.LoadConfig() and the picker tests call theme.SaveBackground,
// which used to overwrite the actual user config.json on disk. Pointing
// LOCALAPPDATA at a throwaway directory keeps the suite hermetic.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "spotygo-ui-test")
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
