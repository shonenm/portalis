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

func TestKeyToBytesTerminalSequences(t *testing.T) {
	tests := []struct {
		name  string
		msg   tea.KeyMsg
		modes keyEncodingModes
		want  string
	}{
		{name: "tmux prefix ctrl+b", msg: tea.KeyMsg{Type: tea.KeyCtrlB}, want: "\x02"},
		{name: "ctrl+a", msg: tea.KeyMsg{Type: tea.KeyCtrlA}, want: "\x01"},
		{name: "ctrl+w", msg: tea.KeyMsg{Type: tea.KeyCtrlW}, want: "\x17"},
		{name: "ctrl+v forwarded", msg: tea.KeyMsg{Type: tea.KeyCtrlV}, want: "\x16"},
		{name: "alt+backspace", msg: tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}, want: "\x1b\x7f"},
		{name: "alt+rune", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'å'}, Alt: true}, want: "\x1bå"},
		{name: "alt+up", msg: tea.KeyMsg{Type: tea.KeyUp, Alt: true}, want: "\x1b[1;3A"},
		{name: "ctrl+up", msg: tea.KeyMsg{Type: tea.KeyCtrlUp}, want: "\x1b[1;5A"},
		{name: "shift+right", msg: tea.KeyMsg{Type: tea.KeyShiftRight}, want: "\x1b[1;2C"},
		{name: "application cursor up", msg: tea.KeyMsg{Type: tea.KeyUp}, modes: keyEncodingModes{applicationCursor: true}, want: "\x1bOA"},
		{name: "f1", msg: tea.KeyMsg{Type: tea.KeyF1}, want: "\x1bOP"},
		{name: "f5", msg: tea.KeyMsg{Type: tea.KeyF5}, want: "\x1b[15~"},
		{name: "f12", msg: tea.KeyMsg{Type: tea.KeyF12}, want: "\x1b[24~"},
		{name: "plain paste", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo"), Paste: true}, want: "one\ntwo"},
		{name: "bracketed paste", msg: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("one\ntwo"), Paste: true}, modes: keyEncodingModes{bracketedPaste: true}, want: "\x1b[200~one\ntwo\x1b[201~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(keyToBytesWithModes(tt.msg, tt.modes))
			if got != tt.want {
				t.Fatalf("key bytes = %q, want %q", got, tt.want)
			}
		})
	}
}
