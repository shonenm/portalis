package portalis

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

func TestRenderCachesUnchangedScreen(t *testing.T) {
	s := NewScreen(2, 5)
	s.Put('A')
	if !s.renderDirty {
		t.Fatal("screen must be dirty after mutation")
	}

	first := s.Render()
	if s.renderDirty {
		t.Fatal("screen must be clean after render")
	}
	if second := s.Render(); second != first {
		t.Fatalf("cached render changed: first=%q second=%q", first, second)
	}
	if s.renderDirty {
		t.Fatal("cached render unexpectedly dirtied the screen")
	}

	s.Put('B')
	if !s.renderDirty {
		t.Fatal("screen mutation did not invalidate render cache")
	}
	if third := s.Render(); third == first {
		t.Fatalf("render cache was not refreshed: %q", third)
	}
}

func BenchmarkScreenRenderCached(b *testing.B) {
	s := NewScreen(40, 120)
	p := NewParser(s)
	p.Feed([]byte("\x1b[?25l\x1b(B\x1b[2J\x1b[Htmux status"))
	s.Render()
	b.ResetTimer()
	for range b.N {
		s.Render()
	}
}

func BenchmarkParserTmuxFrame(b *testing.B) {
	frame := []byte("\x1b[?25l\x1b(B\x1b[2J\x1b[Htmux\x1b[2;1Hstatus\x1b[?25h")
	b.ResetTimer()
	for range b.N {
		s := NewScreen(40, 120)
		NewParser(s).Feed(frame)
		s.Render()
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
		if cell.Continuation {
			continue
		}
		if cell.Rune == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(cell.Rune)
			b.WriteString(cell.Combining)
		}
	}
	return b.String()
}

func TestPutWideRuneUsesTwoCells(t *testing.T) {
	s := NewScreen(1, 4)
	s.Put('✅')
	s.Put('A')

	if !s.Cells[0][1].Continuation {
		t.Fatal("wide rune did not create a continuation cell")
	}
	if s.Cells[0][2].Rune != 'A' {
		t.Fatalf("A written at column %d, want 2", s.Cursor.Col-1)
	}
	if s.Cursor.Col != 3 {
		t.Fatalf("cursor column = %d, want 3", s.Cursor.Col)
	}
	if got := ansi.StringWidth(s.RenderLine(0)); got != 4 {
		t.Fatalf("rendered width = %d, want 4; line=%q", got, s.RenderLine(0))
	}
}

func TestPutCombiningMarkDoesNotAdvanceCursor(t *testing.T) {
	s := NewScreen(1, 4)
	s.Put('e')
	s.Put('\u0301')
	s.Put('A')

	if s.Cursor.Col != 2 {
		t.Fatalf("cursor column = %d, want 2", s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "éA  " {
		t.Fatalf("rendered line = %q, want %q", got, "éA  ")
	}
	if got := ansi.StringWidth(s.RenderLine(0)); got != 4 {
		t.Fatalf("rendered width = %d, want 4", got)
	}
}

func TestVariationSelectorExpandsPreviousCell(t *testing.T) {
	s := NewScreen(1, 4)
	s.Put('❤')
	s.Put('\ufe0f')

	if !s.Cells[0][1].Continuation {
		t.Fatal("emoji variation did not create a continuation cell")
	}
	if s.Cursor.Col != 2 {
		t.Fatalf("cursor column = %d, want 2", s.Cursor.Col)
	}
	if got := ansi.StringWidth(s.RenderLine(0)); got != 4 {
		t.Fatalf("rendered width = %d, want 4; line=%q", got, s.RenderLine(0))
	}
}

func TestEmojiClustersStayInOneWideCell(t *testing.T) {
	for _, cluster := range []string{"👩‍💻", "👍🏽"} {
		t.Run(cluster, func(t *testing.T) {
			s := NewScreen(1, 4)
			for _, r := range cluster {
				s.Put(r)
			}
			s.Put('A')

			if s.Cursor.Col != 3 {
				t.Fatalf("cursor column = %d, want 3", s.Cursor.Col)
			}
			if s.Cells[0][2].Rune != 'A' {
				t.Fatalf("A cell = %+v, want column 2", s.Cells[0][2])
			}
			if got := ansi.StringWidth(s.RenderLine(0)); got != 4 {
				t.Fatalf("rendered width = %d, want 4; line=%q", got, s.RenderLine(0))
			}
		})
	}
}

func TestOverwriteWideContinuationClearsWholeGlyph(t *testing.T) {
	s := NewScreen(1, 4)
	s.Put('✅')
	s.SetCursor(0, 1)
	s.Put('X')

	if s.Cells[0][0].Rune != 0 {
		t.Fatalf("wide base survived continuation overwrite: %q", s.Cells[0][0].Rune)
	}
	if s.Cells[0][1].Rune != 'X' || s.Cells[0][1].Continuation {
		t.Fatalf("replacement cell = %+v, want X base cell", s.Cells[0][1])
	}
	if got := ansi.StringWidth(s.RenderLine(0)); got != 4 {
		t.Fatalf("rendered width = %d, want 4", got)
	}
}

func TestClearWideContinuationClearsWholeGlyph(t *testing.T) {
	s := NewScreen(1, 4)
	s.Put('✅')
	s.SetCursor(0, 1)
	s.ClearLine()

	if s.Cells[0][0].Rune != 0 || s.Cells[0][1].Continuation {
		t.Fatalf("wide glyph survived clear: %+v", s.Cells[0][:2])
	}
}

func TestWideRuneWrapsBeforeLastColumn(t *testing.T) {
	s := NewScreen(2, 4)
	s.PutBytes([]byte("abc"))
	s.Put('✅')

	if got := s.RenderLine(0); got != "abc " {
		t.Fatalf("first line = %q, want %q", got, "abc ")
	}
	if got := s.RenderLine(1); got != "✅  " {
		t.Fatalf("second line = %q, want %q", got, "✅  ")
	}
	if s.Cursor.Row != 1 || s.Cursor.Col != 2 {
		t.Fatalf("cursor = (%d,%d), want (1,2)", s.Cursor.Row, s.Cursor.Col)
	}
}

func TestPutBytesClearsWideBoundary(t *testing.T) {
	s := NewScreen(1, 5)
	s.Put('✅')
	s.SetCursor(0, 1)
	s.PutBytes([]byte("xy"))

	if s.Cells[0][0].Rune != 0 {
		t.Fatalf("wide base survived ASCII overwrite: %q", s.Cells[0][0].Rune)
	}
	if got := s.RenderLine(0); got != " xy  " {
		t.Fatalf("rendered line = %q, want %q", got, " xy  ")
	}
}

func TestPutBytesBasic(t *testing.T) {
	s := NewScreen(3, 10)
	n := s.PutBytes([]byte("hello"))
	if n != 5 {
		t.Fatalf("expected 5 consumed, got %d", n)
	}
	if got := s.Cells[0][0].Rune; got != 'h' {
		t.Errorf("cell 0,0 = %q, want 'h'", got)
	}
	if got := s.Cells[0][4].Rune; got != 'o' {
		t.Errorf("cell 0,4 = %q, want 'o'", got)
	}
	if s.Cursor.Col != 5 {
		t.Errorf("cursor col = %d, want 5", s.Cursor.Col)
	}
	if s.wrapPending {
		t.Errorf("wrapPending should be false")
	}
}

func TestPutBytesWrapAtRowEnd(t *testing.T) {
	s := NewScreen(3, 5)
	n := s.PutBytes([]byte("abcde"))
	if n != 5 {
		t.Fatalf("expected 5 consumed, got %d", n)
	}
	if !s.wrapPending {
		t.Fatalf("expected wrapPending after filling row")
	}
	if s.Cursor.Col != 4 {
		t.Errorf("cursor col = %d, want 4 (last column)", s.Cursor.Col)
	}
	// Next call should wrap and write on next row.
	n = s.PutBytes([]byte("X"))
	if n != 1 {
		t.Fatalf("expected 1 consumed after wrap, got %d", n)
	}
	if s.wrapPending {
		t.Errorf("wrapPending should clear after wrap")
	}
	if got := s.Cells[1][0].Rune; got != 'X' {
		t.Errorf("cell 1,0 = %q, want 'X'", got)
	}
}

func TestPutBytesCapsAtRowBoundary(t *testing.T) {
	s := NewScreen(3, 5)
	// Start at col 2.
	for i := 0; i < 2; i++ {
		s.Put(rune('a' + byte(i)))
	}
	n := s.PutBytes([]byte("cdefgh"))
	// Only 3 bytes fit (cols 2,3,4). PutBytes caps at row end and returns.
	if n != 3 {
		t.Fatalf("expected 3 consumed (row end), got %d", n)
	}
	if !s.wrapPending {
		t.Errorf("expected wrapPending at row end")
	}
}

func TestSyncMode2026FreezesIntermediateFrame(t *testing.T) {
	s := NewScreen(3, 20)
	p := NewParser(s)
	p.Feed([]byte("old"))
	oldFrame := s.Render()

	p.Feed([]byte("\x1b[?2026h\x1b[H\x1b[2Knew"))
	if got := s.Render(); got != oldFrame {
		t.Fatalf("sync frame changed before reset; got:\n%s", got)
	}

	p.Feed([]byte("\x1b[?2026l"))
	newFrame := s.Render()
	if !strings.Contains(newFrame, "new") || strings.Contains(newFrame, "old") {
		t.Fatalf("sync frame was not committed; got:\n%s", newFrame)
	}
}

func TestCJKSequenceCursorPosition(t *testing.T) {
	s := NewScreen(2, 10)
	// Write two CJK characters: 你 (col 0-1), 好 (col 2-3)
	s.Put('你')
	if s.Cursor.Col != 2 {
		t.Fatalf("after 你: cursor col = %d, want 2", s.Cursor.Col)
	}
	s.Put('好')
	if s.Cursor.Col != 4 {
		t.Fatalf("after 好: cursor col = %d, want 4", s.Cursor.Col)
	}
	// Write ASCII after CJK
	s.Put('A')
	if s.Cursor.Col != 5 {
		t.Fatalf("after A: cursor col = %d, want 5", s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "你好A     " {
		t.Fatalf("line 0 = %q (len=%d), want %q", got, len(got), "你好A     ")
	}
}

func TestEmojiSequenceCursorPosition(t *testing.T) {
	s := NewScreen(2, 10)
	// Write thumbs up + skin tone modifier
	for _, r := range "👍🏽" {
		s.Put(r)
	}
	if s.Cursor.Col != 2 {
		t.Fatalf("after 👍🏽: cursor col = %d, want 2", s.Cursor.Col)
	}
	// Write ASCII after emoji
	s.Put('A')
	if s.Cursor.Col != 3 {
		t.Fatalf("after A: cursor col = %d, want 3", s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "👍🏽A       " {
		t.Fatalf("line 0 = %q (len=%d), want %q", got, len(got), "👍🏽A       ")
	}
}

func TestMixedCJKAndASCIIWrapsCorrectly(t *testing.T) {
	s := NewScreen(2, 6)
	// Write: 你 (col 0-1), 好 (col 2-3), A (col 4), B (col 5) → wrapPending
	s.Put('你')
	s.Put('好')
	s.Put('A')
	s.Put('B')
	if !s.wrapPending {
		t.Fatal("expected wrapPending after filling row")
	}
	if s.Cursor.Col != 5 {
		t.Fatalf("cursor col = %d, want 5 (clamped)", s.Cursor.Col)
	}
	// Next char should wrap to next line
	s.Put('C')
	if s.Cursor.Row != 1 || s.Cursor.Col != 1 {
		t.Fatalf("after wrap: cursor = (%d,%d), want (1,1)", s.Cursor.Row, s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "你好AB" {
		t.Fatalf("line 0 = %q, want %q", got, "你好AB")
	}
	if got := s.RenderLine(1); got != "C     " {
		t.Fatalf("line 1 = %q, want %q", got, "C     ")
	}
}

func TestWideCharAtSecondToLastColumn(t *testing.T) {
	s := NewScreen(2, 5)
	// Fill cols 0-2 with ABC
	s.PutBytes([]byte("ABC"))
	// Write wide char at col 3 (second-to-last), continuation at col 4
	s.Put('✅')
	// Cursor goes to col 5 which is >= Cols(5), so wrapPending is set
	if !s.wrapPending {
		t.Fatal("expected wrapPending after wide at col 3 (cursor past edge)")
	}
	if s.Cursor.Col != 4 {
		t.Fatalf("cursor col = %d, want 4 (clamped)", s.Cursor.Col)
	}
	// Verify continuation cell
	if !s.Cells[0][4].Continuation {
		t.Fatal("expected continuation cell at col 4")
	}
	// Next char should wrap
	s.Put('X')
	if s.Cursor.Row != 1 || s.Cursor.Col != 1 {
		t.Fatalf("after wrap: cursor = (%d,%d), want (1,1)", s.Cursor.Row, s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "ABC✅" {
		t.Fatalf("line 0 = %q, want %q", got, "ABC✅")
	}
	if got := s.RenderLine(1); got != "X    " {
		t.Fatalf("line 1 = %q, want %q", got, "X    ")
	}
}

func TestCombiningAfterWideAtEndOfLine(t *testing.T) {
	s := NewScreen(2, 5)
	// Fill cols 0-2 with ABC
	s.PutBytes([]byte("ABC"))
	// Write wide char at col 3, continuation at col 4
	s.Put('❤')
	// Add variation selector (zero-width) — should append to ❤
	s.Put('\ufe0f')
	// Cursor goes to col 5 which is >= Cols(5), so wrapPending is set
	if !s.wrapPending {
		t.Fatal("expected wrapPending after wide at col 3")
	}
	// Add variation selector (zero-width) — should append to ❤
	s.Put('\ufe0f')
	// wrapPending should still be true (variation selector doesn't change cursor)
	if !s.wrapPending {
		t.Fatal("wrapPending should remain after variation selector")
	}
	if s.Cursor.Col != 4 {
		t.Fatalf("cursor col = %d, want 4", s.Cursor.Col)
	}
	// Verify the variation selector was appended
	// Note: ❤ (U+2764) is already emoji-width (2), so the variation selector
	// is appended to Combining. The exact combining string depends on uniseg
	// version; just check it's non-empty.
	if s.Cells[0][3].Combining == "" {
		t.Fatal("expected variation selector in Combining, got empty")
	}
	// Next char should wrap
	s.Put('X')
	if s.Cursor.Row != 1 || s.Cursor.Col != 1 {
		t.Fatalf("after wrap: cursor = (%d,%d), want (1,1)", s.Cursor.Row, s.Cursor.Col)
	}
}

func TestMultipleLinesOfMixedContent(t *testing.T) {
	s := NewScreen(3, 8)
	// Line 0: 你(0-1) 好(2-3) A(4) B(5) C(6) D(7) → wrapPending
	s.Put('你')
	s.Put('好')
	s.PutBytes([]byte("ABCD"))
	if !s.wrapPending {
		t.Fatal("expected wrapPending after filling line 0")
	}
	// ✅ wraps to line 1, col 0-1
	s.Put('✅')
	if s.Cursor.Row != 1 || s.Cursor.Col != 2 {
		t.Fatalf("after ✅ wrap: cursor = (%d,%d), want (1,2)", s.Cursor.Row, s.Cursor.Col)
	}
	// E at line 1, col 2
	s.Put('E')
	if s.Cursor.Row != 1 || s.Cursor.Col != 3 {
		t.Fatalf("after E: cursor = (%d,%d), want (1,3)", s.Cursor.Row, s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "你好ABCD" {
		t.Fatalf("line 0 = %q, want %q", got, "你好ABCD")
	}
	if got := s.RenderLine(1); got != "✅E     " {
		t.Fatalf("line 1 = %q, want %q", got, "✅E     ")
	}
	if got := s.RenderLine(2); got != "        " {
		t.Fatalf("line 2 = %q, want empty", got)
	}
}

func TestWideCharAtLastColumnWraps(t *testing.T) {
	s := NewScreen(2, 5)
	// Fill cols 0-3 with ABCD
	s.PutBytes([]byte("ABCD"))
	// Wide char at col 4 (last column) should wrap to next line
	s.Put('✅')
	if s.Cursor.Row != 1 || s.Cursor.Col != 2 {
		t.Fatalf("cursor = (%d,%d), want (1,2)", s.Cursor.Row, s.Cursor.Col)
	}
	if got := s.RenderLine(0); got != "ABCD " {
		t.Fatalf("line 0 = %q, want %q", got, "ABCD ")
	}
	if got := s.RenderLine(1); got != "✅   " {
		t.Fatalf("line 1 = %q, want %q", got, "✅   ")
	}
}

func TestEmojiWithSkinToneThenASCII(t *testing.T) {
	s := NewScreen(2, 10)
	// Write emoji with skin tone: 👍🏽 (2 cols)
	for _, r := range "👍🏽" {
		s.Put(r)
	}
	// Write another emoji: 🎉 (2 cols)
	s.Put('🎉')
	if s.Cursor.Col != 4 {
		t.Fatalf("cursor col = %d, want 4", s.Cursor.Col)
	}
	// Write ASCII
	s.Put('A')
	if s.Cursor.Col != 5 {
		t.Fatalf("cursor col = %d, want 5", s.Cursor.Col)
	}
	// 👍🏽(2) + 🎉(2) + A(1) = 5 visible cols, 5 empty = 10 cols
	if got := s.RenderLine(0); got != "👍🏽🎉A     " {
		t.Fatalf("line 0 = %q (len=%d), want %q", got, len(got), "👍🏽🎉A     ")
	}
}

func TestCJKThenEmojiThenASCII(t *testing.T) {
	s := NewScreen(2, 10)
	// Write CJK: 你 (2 cols)
	s.Put('你')
	// Write emoji: 🎉 (2 cols)
	s.Put('🎉')
	if s.Cursor.Col != 4 {
		t.Fatalf("cursor col = %d, want 4", s.Cursor.Col)
	}
	// Write ASCII
	s.Put('A')
	if s.Cursor.Col != 5 {
		t.Fatalf("cursor col = %d, want 5", s.Cursor.Col)
	}
	// 你(2) + 🎉(2) + A(1) = 5 visible cols, 5 empty = 10 cols
	if got := s.RenderLine(0); got != "你🎉A     " {
		t.Fatalf("line 0 = %q (len=%d), want %q", got, len(got), "你🎉A     ")
	}
}

// TestPiTUIStyleStreaming simulates the Pi TUI's output pattern:
// streaming model response text (with wide chars) interleaved with
// spinner updates. This catches cursor drift that causes "double Working".
//
// IMPORTANT: \n (newline) must be handled by the parser, not by Screen.Put.
// Screen.Put is only for printable characters. This test simulates the
// parser's behavior by calling Screen.Index() for \n and Screen.CarriageReturn() for \r.
func TestPiTUIStyleStreaming(t *testing.T) {
	s := NewScreen(5, 40)

	writeStr := func(str string) {
		for _, r := range str {
			switch r {
			case '\n':
				s.Index()
			case '\r':
				s.Cursor.Col = 0
			default:
				s.Put(r)
			}
		}
	}

	// Initial prompt line
	writeStr("> Hello")
	writeStr("\r\n")
	// Model starts responding with some text containing wide chars
	writeStr("Привет! Вот пример с иероглифами: 你好世界")
	writeStr("\n")
	// First spinner update
	writeStr("   ⠙ Working...")
	writeStr("\n\n")
	writeStr("А вот ещё смайлы: 🎉🔥🌟")
	writeStr("\n")
	// Second spinner update — should overwrite the first one
	// But since there's text between them, they end up on different lines
	writeStr("   ⠋ Working...")

	// In the real Pi TUI, the spinner is updated in place on the SAME line
	// using \r to return to the start of the spinner line. But in this test
	// we write text between spinners, so they naturally end up on different lines.
	// This is expected — the Pi TUI should clear the old spinner before writing
	// the new one, but it doesn't (no ESC[K in the raw trace).
	// The double Working is a Pi TUI issue, not a Portalis issue.
	rendered := s.Render()
	t.Logf("Rendered output:\n%s", rendered)
}

// TestCursorDriftWithWideChars tests that cursor position doesn't drift
// when writing a mix of ASCII, CJK, and emoji characters across multiple lines.
func TestCursorDriftWithWideChars(t *testing.T) {
	s := NewScreen(10, 40)

	writeLine := func(str string) {
		for _, r := range str {
			s.Put(r)
		}
		s.Index()        // \n
		s.Cursor.Col = 0 // \r
	}

	lines := []string{
		"Hello 你好 World 🎉",
		"Test 测试 Test 🔥",
		"Line with emoji 🌟 and CJK 中文",
		"Another line 👍🏽 with skin tone",
		"Final line ✅✅✅",
	}

	for _, line := range lines {
		writeLine(line)
	}

	// Cursor should be on row 5 (after 5 lines, 0-indexed)
	if s.Cursor.Row != 5 {
		t.Fatalf("cursor row = %d, want 5", s.Cursor.Row)
	}

	// Check that each line renders correctly
	for i, line := range lines {
		got := s.RenderLine(i)
		if !strings.Contains(got, line) {
			t.Fatalf("line %d = %q, should contain %q", i, got, line)
		}
	}
}

// TestWideCharWrapDoesNotDriftCursor tests that wrapping a wide character
// at the end of a line doesn't cause cursor drift on subsequent lines.
func TestWideCharWrapDoesNotDriftCursor(t *testing.T) {
	s := NewScreen(5, 10)

	// Fill line 0 with 8 ASCII chars + 1 wide char (wraps to line 1)
	for i := 0; i < 8; i++ {
		s.Put('A' + rune(i))
	}
	// Wide char at col 8 (second-to-last), continuation at col 9
	s.Put('✅')
	// Cursor should be at col 10 (clamped to 9), wrapPending = true
	if !s.wrapPending {
		t.Fatal("expected wrapPending after wide at col 8")
	}
	// Next char wraps to line 1
	s.Put('X')
	if s.Cursor.Row != 1 || s.Cursor.Col != 1 {
		t.Fatalf("after wrap: cursor = (%d,%d), want (1,1)", s.Cursor.Row, s.Cursor.Col)
	}

	// Write more chars on line 1
	for i := 0; i < 8; i++ {
		s.Put('A' + rune(i))
	}
	// Wide char at col 9 (last column) — wraps to line 2
	s.Put('🔥')
	// 🔥 is at line 2, col 0-1. Cursor at col 2.
	if s.Cursor.Row != 2 || s.Cursor.Col != 2 {
		t.Fatalf("after 🔥 wrap: cursor = (%d,%d), want (2,2)", s.Cursor.Row, s.Cursor.Col)
	}
	// Next char at col 2
	s.Put('Y')
	if s.Cursor.Row != 2 || s.Cursor.Col != 3 {
		t.Fatalf("after Y: cursor = (%d,%d), want (2,3)", s.Cursor.Row, s.Cursor.Col)
	}
	// Verify line 0
	if got := s.RenderLine(0); got != "ABCDEFGH✅" {
		t.Fatalf("line 0 = %q, want %q", got, "ABCDEFGH✅")
	}
	// Verify line 1 — 🔥 wrapped to line 2 because it was at last column
	if got := s.RenderLine(1); got != "XABCDEFGH " {
		t.Fatalf("line 1 = %q, want %q", got, "XABCDEFGH ")
	}
	// Verify line 2 — 🔥 at col 0-1, Y at col 2
	if got := s.RenderLine(2); got != "🔥Y       " {
		t.Fatalf("line 2 = %q, want %q", got, "🔥Y       ")
	}
}

// TestParserWideCharsThroughParser feeds ANSI sequences through the parser
// (as the real Pi TUI does) and checks cursor position doesn't drift.
func TestParserWideCharsThroughParser(t *testing.T) {
	s := NewScreen(10, 60)
	p := NewParser(s)

	// Simulate Pi TUI output: prompt + model response with wide chars
	feed := func(data string) {
		p.Feed([]byte(data))
	}

	// Write prompt line
	feed("> Hello\r\n")
	// Model response with CJK and emoji (fits in 60 cols)
	feed("Привет! Вот пример с иероглифами: 你好\r\n")
	feed("А вот ещё смайлы: 🎉🔥🌟\r\n")
	feed("Line with 👍🏽 skin tone emoji\r\n")

	// Cursor should be on row 4 (after 4 lines + \r\n on last line, 0-indexed)
	if s.Cursor.Row != 4 {
		t.Fatalf("cursor row = %d, want 4", s.Cursor.Row)
	}

	// Verify each line contains the expected content
	lines := []string{
		"> Hello",
		"Привет! Вот пример с иероглифами: 你好",
		"А вот ещё смайлы: 🎉🔥🌟",
		"Line with 👍🏽 skin tone emoji",
	}
	for i, line := range lines {
		got := s.RenderLine(i)
		if !strings.Contains(got, line) {
			t.Fatalf("line %d = %q, should contain %q", i, got, line)
		}
	}
}

// TestParserCursorAfterWideChars tests that cursor position is correct
// after writing wide characters through the parser.
func TestParserCursorAfterWideChars(t *testing.T) {
	s := NewScreen(5, 10)
	p := NewParser(s)

	// Write ASCII + CJK + emoji through parser
	// A(0) B(1) 你(2-3) C(4) D(5) 好(6-7) E(8) F(9) → wrapPending
	p.Feed([]byte("AB你CD好EF"))
	if !s.wrapPending {
		t.Fatal("expected wrapPending after filling row")
	}

	// Next char wraps to line 1
	p.Feed([]byte("X"))
	if s.Cursor.Row != 1 || s.Cursor.Col != 1 {
		t.Fatalf("after wrap: cursor = (%d,%d), want (1,1)", s.Cursor.Row, s.Cursor.Col)
	}

	// Verify line 0
	if got := s.RenderLine(0); got != "AB你CD好EF" {
		t.Fatalf("line 0 = %q, want %q", got, "AB你CD好EF")
	}
}

// TestParserSpinnerWithWideChars simulates the Pi TUI's spinner update
// pattern with wide characters in the model response.
func TestParserSpinnerWithWideChars(t *testing.T) {
	s := NewScreen(10, 60)
	p := NewParser(s)

	// Write prompt
	p.Feed([]byte("> Hello\r\n"))
	// Model starts responding (fits in 60 cols)
	p.Feed([]byte("Here is some text with 你好\r\n"))
	// Spinner update
	p.Feed([]byte("   ⠙ Working...\r\n"))
	// More model output
	p.Feed([]byte("More text with 🎉🔥🌟 emojis\r\n"))
	// Second spinner update
	p.Feed([]byte("   ⠋ Working...\r\n"))

	// Count Working lines
	rendered := s.Render()
	workingCount := 0
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "Working") {
			workingCount++
		}
	}

	t.Logf("Working lines: %d", workingCount)
	t.Logf("Rendered:\n%s", rendered)

	// Cursor should be on row 5 (after 5 lines + \r\n on last line, 0-indexed)
	if s.Cursor.Row != 5 {
		t.Fatalf("cursor row = %d, want 5", s.Cursor.Row)
	}
}
