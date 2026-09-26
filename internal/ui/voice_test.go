package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestToastStringsLayoutSafety(t *testing.T) {
	toasts := []string{
		toastPlaying, toastPaused, toastNext, toastPrev,
		toastPlaylists, toastZen,
	}
	for _, msg := range toasts {
		if got := ansi.StringWidth(msg); got <= 0 {
			t.Fatalf("toast %q renders empty width", msg)
		}
	}
}

func TestFlowLabelClarifiesZenScope(t *testing.T) {
	if got := backgroundLabel("flow"); got != "flow (solo Zen)" {
		t.Fatalf("expected flow label to clarify that its animation belongs to Zen, got %q", got)
	}
}
