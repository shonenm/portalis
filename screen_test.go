package portalis

import (
	"strings"
	"testing"
)

func TestResizeUpPreservesContent(t *testing.T) {
	s := NewScreen(5, 10)
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			s.Put('X')
		}
	}

	s.Resize(10, 10)

	for r := 0; r < 5; r++ {
		line := s.RenderLine(r)
		if line != strings.Repeat("X", 10) {
			t.Errorf("row %d: expected old content, got %q", r, line)
		}
	}
	for r := 5; r < 10; r++ {
		line := s.RenderLine(r)
		if line != strings.Repeat(" ", 10) {
			t.Errorf("row %d: expected empty new row, got %q", r, line)
		}
	}
}

func TestSelectionSingleLine(t *testing.T) {
	s := NewScreen(3, 10)
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			s.Put('A' + rune(r))
		}
	}
	s.StartSelection(1, 2)
	s.ExtendSelection(1, 5)
	text := s.SelectionText()
	if len(text) != 1 || text[0] != "BBBB" {
		t.Errorf("expected [BBBB], got %v", text)
	}
}

func TestSelectionMultiLine(t *testing.T) {
	s := NewScreen(3, 10)
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			s.Put('A' + rune(r))
		}
	}
	s.StartSelection(0, 2)
	s.ExtendSelection(2, 5)
	text := s.SelectionText()
	if len(text) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(text))
	}
	if text[0] != "AAAAAAAA"[:8] {
		t.Errorf("line 0: expected truncated AAA..., got %q", text[0])
	}
}

func TestSelectionReverseDirection(t *testing.T) {
	s := NewScreen(3, 10)
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			s.Put('A' + rune(r))
		}
	}
	// Drag from (2,5) back to (1,2) — reverse direction.
	s.StartSelection(2, 5)
	s.ExtendSelection(1, 2)
	text := s.SelectionText()
	if len(text) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(text), text)
	}
}

func TestSelectionExclusiveEndLeft(t *testing.T) {
	s := NewScreen(1, 10)
	for c := 0; c < s.Cols; c++ {
		s.Put('A' + rune(c))
	}
	// Cells: A B C D E F G H I J (cols 0..9).
	// Drag from col 7 (H) to col 3 (D) — leftward.
	// Mouse is at col 3, character under mouse ('D') must NOT be selected.
	s.StartSelection(0, 7)
	s.ExtendSelection(0, 3)
	text := s.SelectionText()
	if len(text) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(text), text)
	}
	// Expected: cols 4..7 = E F G H
	if text[0] != "EFGH" {
		t.Errorf("expected exclusive end EFGH, got %q", text[0])
	}
}

func TestSelectionInclusiveEndRight(t *testing.T) {
	s := NewScreen(1, 10)
	for c := 0; c < s.Cols; c++ {
		s.Put('A' + rune(c))
	}
	// Drag from col 3 (D) to col 7 (H) — rightward, full inclusive.
	s.StartSelection(0, 3)
	s.ExtendSelection(0, 7)
	text := s.SelectionText()
	if len(text) != 1 || text[0] != "DEFGH" {
		t.Errorf("expected inclusive end DEFGH, got %q", text[0])
	}
}

func TestResizeUpResetsScrollRegion(t *testing.T) {
	s := NewScreen(5, 10)
	s.SetScrollRegion(2, 4)
	if s.scrollTop != 1 || s.scrollBottom != 3 {
		t.Fatalf("unexpected initial scroll region: %d-%d", s.scrollTop, s.scrollBottom)
	}

	s.Resize(10, 10)

	if s.scrollTop != 0 || s.scrollBottom != 9 {
		t.Errorf("scroll region not reset after resize: %d-%d", s.scrollTop, s.scrollBottom)
	}
}

func TestClearLineResetsWrapPending(t *testing.T) {
	s := NewScreen(3, 5)
	for c := 0; c < s.Cols; c++ {
		s.Put('X')
	}
	if !s.wrapPending {
		t.Fatal("expected wrapPending after writing to last column")
	}

	s.ClearLineAll()
	s.SetCursor(0, 0)
	s.Put('Y')

	// After clearing the line and resetting wrapPending, writing a character
	// should stay on the same row instead of wrapping to the next line.
	if s.Cursor.Row != 0 || s.Cursor.Col != 1 {
		t.Errorf("cursor moved unexpectedly: row=%d col=%d", s.Cursor.Row, s.Cursor.Col)
	}
	if s.Cells[0][0].Rune != 'Y' {
		t.Errorf("expected Y at (0,0), got %q", s.Cells[0][0].Rune)
	}
}

func TestViewOffsetClamp(t *testing.T) {
	s := NewScreen(3, 5)
	s.ScrollViewUp(10)
	if s.viewOffset != 0 {
		t.Errorf("viewOffset should be 0 when no scrollback, got %d", s.viewOffset)
	}
}

// RenderLine returns the raw content of a single row for testing.
func (s *Screen) RenderLine(r int) string {
	if r < 0 || r >= s.Rows {
		return ""
	}
	var b strings.Builder
	for c := 0; c < s.Cols; c++ {
		cell := s.Cells[r][c]
		if cell.Rune == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(cell.Rune)
		}
	}
	return b.String()
}
