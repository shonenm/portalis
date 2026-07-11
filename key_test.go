package portalis

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyToBytesEnter(t *testing.T) {
	if got := keyToBytes(tea.KeyMsg{Type: tea.KeyEnter}); string(got) != "\r" {
		t.Fatalf("Enter = %q, want CR", got)
	}
}

func TestKeyToBytesShiftEnter(t *testing.T) {
	if got := keyToBytes(tea.KeyMsg{Type: tea.KeyEnter}); string(got) == "\n" {
		t.Fatalf("plain Enter should not send LF")
	}
	// Bubble Tea may deliver Shift+Enter as a named key or as a literal LF rune.
	if got := keyToBytes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}}); string(got) != "\n" {
		t.Fatalf("Shift+Enter as LF rune = %q, want LF", got)
	}
}
