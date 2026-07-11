package portalis

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParserPut(t *testing.T) {
	s := NewScreen(5, 10)
	p := NewParser(s)
	p.Feed([]byte("hello"))

	if s.Cells[0][0].Rune != 'h' {
		t.Errorf("expected 'h', got %c", s.Cells[0][0].Rune)
	}
	if s.Cells[0][4].Rune != 'o' {
		t.Errorf("expected 'o', got %c", s.Cells[0][4].Rune)
	}
}

func TestParserCursor(t *testing.T) {
	s := NewScreen(5, 10)
	p := NewParser(s)
	p.Feed([]byte("\x1b[3;5Hab"))

	if s.Cells[2][4].Rune != 'a' {
		t.Errorf("expected 'a' at row 3 col 5, got %c", s.Cells[2][4].Rune)
	}
}

func TestParserColor(t *testing.T) {
	s := NewScreen(2, 10)
	p := NewParser(s)
	p.Feed([]byte("\x1b[31mred\x1b[0m"))

	if s.Cells[0][0].FG != lipgloss.Color("#800000") {
		t.Errorf("expected dark red fg, got %v", s.Cells[0][0].FG)
	}
}

func TestParserClear(t *testing.T) {
	s := NewScreen(3, 10)
	p := NewParser(s)
	p.Feed([]byte("hello"))
	p.Feed([]byte("\x1b[2J"))

	if s.Cells[0][0].Rune != 0 {
		t.Error("expected screen cleared")
	}
}

func TestParserNewline(t *testing.T) {
	s := NewScreen(3, 10)
	p := NewParser(s)
	p.Feed([]byte("hello\r\nworld"))

	if s.Cells[1][0].Rune != 'w' {
		t.Errorf("expected 'w' on second line, got %c", s.Cells[1][0].Rune)
	}
}

func TestAnsi256Color(t *testing.T) {
	c := ansi256Color(9)
	if c != lipgloss.Color("#ff0000") {
		t.Errorf("expected bright red, got %v", c)
	}
}

func TestRGB(t *testing.T) {
	if rgb(255, 0, 128) != "#ff0080" {
		t.Errorf("expected #ff0080, got %s", rgb(255, 0, 128))
	}
}

func TestOSC7(t *testing.T) {
	s := NewScreen(10, 40)
	p := NewParser(s)

	var cwd string
	p.SetCWDCallback(func(path string) {
		cwd = path
	})

	p.Feed([]byte("before\x1b]7;/Users/a/foo\x1b\\after"))

	if cwd != "/Users/a/foo" {
		t.Fatalf("cwd = %q, want /Users/a/foo", cwd)
	}

	text := s.Render()
	if !strings.Contains(text, "before") {
		t.Fatalf("screen missing 'before': %q", text)
	}
	if !strings.Contains(text, "after") {
		t.Fatalf("screen missing 'after': %q", text)
	}
}
