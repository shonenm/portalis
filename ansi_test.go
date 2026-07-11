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

func TestParserTmuxCharsetSelection(t *testing.T) {
	s := NewScreen(2, 12)
	p := NewParser(s)

	p.Feed([]byte("\x1b(Bhello"))
	p.Feed([]byte("\r\n\x1b(0lqk\x1b(B"))

	if got := s.RenderLine(0); got != "hello       " {
		t.Fatalf("ASCII charset rendered %q, want %q", got, "hello       ")
	}
	if got := s.RenderLine(1); got != "┌─┐         " {
		t.Fatalf("DEC graphics rendered %q, want %q", got, "┌─┐         ")
	}
}

func TestParserTmuxDeleteLineAndReverseIndex(t *testing.T) {
	t.Run("CSI M deletes a line", func(t *testing.T) {
		s := NewScreen(4, 4)
		p := NewParser(s)
		p.Feed([]byte("1111\r\n2222\r\n3333\r\n4444"))
		p.Feed([]byte("\x1b[2;1H\x1b[M"))

		want := []string{"1111", "3333", "4444", "    "}
		for row, expected := range want {
			if got := s.RenderLine(row); got != expected {
				t.Fatalf("row %d = %q, want %q", row, got, expected)
			}
		}
	})

	t.Run("ESC M performs reverse index", func(t *testing.T) {
		s := NewScreen(3, 4)
		p := NewParser(s)
		p.Feed([]byte("AAAA\r\nBBBB\r\nCCCC"))
		p.Feed([]byte("\x1b[1;1H\x1bM"))

		want := []string{"    ", "AAAA", "BBBB"}
		for row, expected := range want {
			if got := s.RenderLine(row); got != expected {
				t.Fatalf("row %d = %q, want %q", row, got, expected)
			}
		}
	})
}

func TestParserTmuxCharacterEditing(t *testing.T) {
	tests := []struct {
		name string
		seq  string
		want string
	}{
		{name: "insert characters", seq: "\x1b[1;3H\x1b[2@", want: "AB  C"},
		{name: "delete characters", seq: "\x1b[1;3H\x1b[2P", want: "ABE  "},
		{name: "erase characters", seq: "\x1b[1;3H\x1b[2X", want: "AB  E"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScreen(1, 5)
			p := NewParser(s)
			p.Feed([]byte("ABCDE" + tt.seq))
			if got := s.RenderLine(0); got != tt.want {
				t.Fatalf("line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParserTmuxLineEditingWithinScrollRegion(t *testing.T) {
	newFixture := func() (*Screen, *Parser) {
		s := NewScreen(5, 3)
		p := NewParser(s)
		p.Feed([]byte("111\r\n222\r\n333\r\n444\r\n555"))
		p.Feed([]byte("\x1b[2;4r"))
		return s, p
	}

	t.Run("insert line", func(t *testing.T) {
		s, p := newFixture()
		p.Feed([]byte("\x1b[3;1H\x1b[L"))
		want := []string{"111", "222", "   ", "333", "555"}
		for row, expected := range want {
			if got := s.RenderLine(row); got != expected {
				t.Fatalf("row %d = %q, want %q", row, got, expected)
			}
		}
	})

	t.Run("scroll up then down", func(t *testing.T) {
		s, p := newFixture()
		p.Feed([]byte("\x1b[S"))
		wantUp := []string{"111", "333", "444", "   ", "555"}
		for row, expected := range wantUp {
			if got := s.RenderLine(row); got != expected {
				t.Fatalf("after SU row %d = %q, want %q", row, got, expected)
			}
		}

		p.Feed([]byte("\x1b[T"))
		wantDown := []string{"111", "   ", "333", "444", "555"}
		for row, expected := range wantDown {
			if got := s.RenderLine(row); got != expected {
				t.Fatalf("after SD row %d = %q, want %q", row, got, expected)
			}
		}
	})
}

func TestParserTmuxModesAndFrame(t *testing.T) {
	s := NewScreen(3, 16)
	p := NewParser(s)
	frame := "\x1b[?1;25;2004h\x1b[?25l\x1b(B\x1b[2J\x1b[Htmux\x1b[2;1Hstatus\x1b[?25h"
	p.Feed([]byte(frame))

	if !s.applicationCursor {
		t.Fatal("application cursor mode was not enabled")
	}
	if !s.bracketedPaste {
		t.Fatal("bracketed paste mode was not enabled")
	}
	if !s.CursorVisible {
		t.Fatal("cursor should be visible after DECSET ?25")
	}
	if got := s.RenderLine(0); got != "tmux            " {
		t.Fatalf("frame row 0 = %q", got)
	}
	if got := s.RenderLine(1); got != "status          " {
		t.Fatalf("frame row 1 = %q", got)
	}
}
