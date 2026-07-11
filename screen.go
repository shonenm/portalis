package portalis

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StyleBits holds text style attributes.
type StyleBits uint8

const (
	StyleBold StyleBits = 1 << iota
	StyleDim
	StyleItalic
	StyleUnderline
	StyleBlink
	StyleReverse
	StyleHidden
	StyleStrikethrough
)

// Cell is a single character cell on the terminal screen.
type Cell struct {
	Rune  rune
	FG    lipgloss.Color
	BG    lipgloss.Color
	Style StyleBits
}

// Empty returns true if the cell has no content.
func (c Cell) Empty() bool {
	return c.Rune == 0 || c.Rune == ' '
}

// Screen is a 2D grid of cells.
type Screen struct {
	Rows   int
	Cols   int
	Cells  [][]Cell
	Cursor Cursor

	savedCursor            Cursor
	savedCells             [][]Cell
	scrollTop, scrollBottom int // 0-indexed, DECSTBM. Default 0, Rows-1.
	wrapPending            bool  // true after writing to last column; next char wraps.

	scrollback      [][]Cell // lines scrolled off the top
	scrollbackLimit int
	viewOffset      int      // lines scrolled back in the view

	syncActive bool   // true during synchronized output (ESC[?2026h)
	lastRender string // last committed render while sync is active

	CursorBlinkVisible bool // toggled by emulator for blinking cursor

	// selection tracks a text drag-select rectangle. -1 means unset.
	selStartRow, selStartCol int
	selEndRow, selEndCol     int
	selectionActive          bool
}

// StartSelection begins a selection at the given cell.
func (s *Screen) StartSelection(row, col int) {
	if row < 0 || row >= s.Rows || col < 0 || col >= s.Cols {
		return
	}
	s.selStartRow, s.selStartCol = row, col
	s.selEndRow, s.selEndCol = row, col
	s.selectionActive = true
}

// ExtendSelection updates the selection end to the given cell.
func (s *Screen) ExtendSelection(row, col int) {
	if !s.selectionActive {
		return
	}
	if row < 0 {
		row = 0
	}
	if row >= s.Rows {
		row = s.Rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= s.Cols {
		col = s.Cols - 1
	}
	s.selEndRow, s.selEndCol = row, col
}

// ClearSelection clears any active selection.
func (s *Screen) ClearSelection() {
	s.selectionActive = false
}

// cellInSelection reports whether the given cell is within the current
// selection rectangle. When dragging leftward on a single row, the
// character under the mouse (the cursor, which is the lower col index
// after normalization) is excluded from the selection.
func (s *Screen) cellInSelection(row, col int) bool {
	if !s.selectionActive {
		return false
	}
	aRow, aCol := s.selStartRow, s.selStartCol // anchor (press)
	cRow, cCol := s.selEndRow, s.selEndCol     // cursor (current)

	// When dragging left on the same row, exclude the column under the
	// mouse (which becomes c1 after normalization).
	exclusiveStart := aRow == cRow && aCol > cCol

	r1, c1, r2, c2 := aRow, aCol, cRow, cCol
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if row < r1 || row > r2 {
		return false
	}
	if row == r1 && row == r2 {
		if exclusiveStart {
			return col > c1 && col <= c2
		}
		return col >= c1 && col <= c2
	}
	if row == r1 {
		if exclusiveStart {
			return col > c1
		}
		return col >= c1
	}
	if row == r2 {
		return col <= c2
	}
	return true
}

// SelectionText returns the text currently highlighted by the selection,
// with each line as one string in the slice. Trailing whitespace trimmed.
// When dragging left on a single row, the column under the mouse (the
// lower col index) is excluded.
func (s *Screen) SelectionText() []string {
	if !s.selectionActive {
		return nil
	}
	aRow, aCol := s.selStartRow, s.selStartCol
	cRow, cCol := s.selEndRow, s.selEndCol
	exclusiveStart := aRow == cRow && aCol > cCol

	r1, c1, r2, c2 := aRow, aCol, cRow, cCol
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r2 < 0 || r2 >= s.Rows {
		return nil
	}
	var out []string
	for r := r1; r <= r2; r++ {
		start := 0
		end := s.Cols - 1
		if r == r1 {
			start = c1
			if exclusiveStart {
				start = c1 + 1
			}
		}
		if r == r2 {
			end = c2
		}
		if start > end {
			out = append(out, "")
			continue
		}
		if start >= len(s.Cells[r]) {
			out = append(out, "")
			continue
		}
		if end >= len(s.Cells[r]) {
			end = len(s.Cells[r]) - 1
		}
		var b strings.Builder
		for c := start; c <= end; c++ {
			rn := s.Cells[r][c].Rune
			if rn == 0 {
				rn = ' '
			}
			b.WriteRune(rn)
		}
		out = append(out, strings.TrimRight(b.String(), " "))
	}
	return out
}

// Cursor represents the terminal cursor position.
type Cursor struct {
	Row int
	Col int
	FG  lipgloss.Color
	BG  lipgloss.Color
	Style StyleBits
}

// defaultScrollbackLimit caps the scrollback buffer to prevent unbounded
// memory growth for long-running sessions.
const defaultScrollbackLimit = 10000

// NewScreen creates a new screen with the given dimensions.
// Scrollback is capped to defaultScrollbackLimit lines by default.
func NewScreen(rows, cols int) *Screen {
	s := &Screen{Rows: rows, Cols: cols, scrollBottom: rows - 1, scrollbackLimit: defaultScrollbackLimit}
	s.resize(rows, cols)
	return s
}

// SetScrollbackLimit sets the maximum number of scrollback lines. A value
// of zero or less disables the limit (not recommended for long-running
// sessions).
func (s *Screen) SetScrollbackLimit(limit int) {
	s.scrollbackLimit = limit
	if limit > 0 && len(s.scrollback) > limit {
		s.scrollback = s.scrollback[len(s.scrollback)-limit:]
	}
}

func (s *Screen) resize(rows, cols int) {
	old := s.Cells
	s.Cells = make([][]Cell, rows)
	for r := 0; r < rows; r++ {
		s.Cells[r] = make([]Cell, cols)
		if r < len(old) {
			copy(s.Cells[r], old[r])
		}
	}
	s.Rows = rows
	s.Cols = cols
	// Reset scroll region to the full screen on resize; this matches the
	// behaviour of most terminal emulators and prevents stale margins from
	// leaving unused blank lines after the terminal grows.
	s.scrollTop = 0
	s.scrollBottom = rows - 1

	// Invalidate any cached render so the next frame uses the new dimensions.
	s.lastRender = ""
}

// Resize resizes the screen, preserving existing content.
func (s *Screen) Resize(rows, cols int) {
	if rows == s.Rows && cols == s.Cols {
		return
	}
	s.resize(rows, cols)
	if s.Cursor.Row >= rows {
		s.Cursor.Row = rows - 1
	}
	if s.Cursor.Col >= cols {
		s.Cursor.Col = cols - 1
	}
}

// Clear clears the entire screen.
func (s *Screen) Clear() {
	for r := range s.Cells {
		for c := range s.Cells[r] {
			s.Cells[r][c] = Cell{}
		}
	}
	s.wrapPending = false
}

// ClearLine clears the current line from cursor to end.
func (s *Screen) ClearLine() {
	if s.Cursor.Row < 0 || s.Cursor.Row >= s.Rows {
		return
	}
	for c := s.Cursor.Col; c < s.Cols; c++ {
		s.Cells[s.Cursor.Row][c] = Cell{}
	}
	s.wrapPending = false
}

// ClearLineLeft clears from start of line to cursor.
func (s *Screen) ClearLineLeft() {
	if s.Cursor.Row < 0 || s.Cursor.Row >= s.Rows {
		return
	}
	for c := 0; c <= s.Cursor.Col && c < s.Cols; c++ {
		s.Cells[s.Cursor.Row][c] = Cell{}
	}
	s.wrapPending = false
}

// ClearLineAll clears the entire current line.
func (s *Screen) ClearLineAll() {
	if s.Cursor.Row < 0 || s.Cursor.Row >= s.Rows {
		return
	}
	for c := 0; c < s.Cols; c++ {
		s.Cells[s.Cursor.Row][c] = Cell{}
	}
	s.wrapPending = false
}

// Put writes a rune at the cursor position and advances the cursor.
// Implements the standard terminal "pending wrap" behaviour: writing to the
// last column leaves the cursor at the last column and defers wrapping until
// the next printable character.
func (s *Screen) Put(r rune) {
	if s.Cursor.Row < 0 || s.Cursor.Row >= s.Rows {
		return
	}

	if s.wrapPending {
		s.wrapPending = false
		s.Cursor.Col = 0
		s.Cursor.Row++
		if s.Cursor.Row >= s.Rows {
			s.scrollLineUp()
			s.Cursor.Row = s.Rows - 1
		}
	}

	if s.Cursor.Col < 0 || s.Cursor.Col >= s.Cols {
		return
	}

	s.Cells[s.Cursor.Row][s.Cursor.Col] = Cell{
		Rune:  r,
		FG:    s.Cursor.FG,
		BG:    s.Cursor.BG,
		Style: s.Cursor.Style,
	}

	if s.Cursor.Col == s.Cols-1 {
		s.wrapPending = true
		return
	}
	s.Cursor.Col++
}

// SetCursor sets the cursor position (1-indexed in ANSI, 0-indexed here).
func (s *Screen) SetCursor(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= s.Rows {
		row = s.Rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= s.Cols {
		col = s.Cols - 1
	}
	s.Cursor.Row = row
	s.Cursor.Col = col
	s.wrapPending = false
}

// ScrollUp scrolls the active region up by one line.
func (s *Screen) ScrollUp() {
	s.scrollLineUp()
}

// scrollLineUp moves the active region up by one line, saving the scrolled-off
// line to the scrollback buffer if the region starts at the top of the screen.
// A pending selection is cleared because the content the user was selecting
// has moved out of screen coordinates.
func (s *Screen) scrollLineUp() {
	if s.Rows <= 1 {
		return
	}
	top := s.scrollTop
	bottom := s.scrollBottom
	if bottom <= top {
		return
	}
	if s.selectionActive {
		s.selectionActive = false
	}
	if top == 0 {
		line := make([]Cell, s.Cols)
		copy(line, s.Cells[0])
		s.scrollback = append(s.scrollback, line)
		if s.scrollbackLimit > 0 && len(s.scrollback) > s.scrollbackLimit {
			s.scrollback = s.scrollback[len(s.scrollback)-s.scrollbackLimit:]
		}
	}
	for r := top + 1; r <= bottom; r++ {
		copy(s.Cells[r-1], s.Cells[r])
	}
	for c := 0; c < s.Cols; c++ {
		s.Cells[bottom][c] = Cell{}
	}
}

// SaveCursor saves the current cursor position.
func (s *Screen) SaveCursor() {
	s.savedCursor = s.Cursor
	// Don't save FG/BG — just position
	s.savedCursor.FG = ""
	s.savedCursor.BG = ""
	s.savedCursor.Style = 0
}

// RestoreCursor restores the saved cursor position.
func (s *Screen) RestoreCursor() {
	row := s.savedCursor.Row
	col := s.savedCursor.Col
	if row < 0 {
		row = 0
	}
	if row >= s.Rows {
		row = s.Rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= s.Cols {
		col = s.Cols - 1
	}
	s.Cursor.Row = row
	s.Cursor.Col = col
	s.wrapPending = false
}

// EnterAltScreen saves the current screen and clears it.
func (s *Screen) EnterAltScreen() {
	s.savedCells = make([][]Cell, s.Rows)
	for r := 0; r < s.Rows; r++ {
		s.savedCells[r] = make([]Cell, s.Cols)
		copy(s.savedCells[r], s.Cells[r])
	}
	s.savedCursor = s.Cursor
	s.Clear()
	s.SetCursor(0, 0)
}

// ExitAltScreen restores the saved screen.
func (s *Screen) ExitAltScreen() {
	if s.savedCells == nil {
		return
	}
	s.Cells = s.savedCells
	s.savedCells = nil
	row := s.savedCursor.Row
	col := s.savedCursor.Col
	if row < 0 {
		row = 0
	}
	if row >= s.Rows {
		row = s.Rows - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= s.Cols {
		col = s.Cols - 1
	}
	s.Cursor.Row = row
	s.Cursor.Col = col
	s.wrapPending = false
}

// SetScrollRegion sets the scrolling region (DECSTBM).
// top and bottom are 1-indexed (as sent by ANSI).
func (s *Screen) SetScrollRegion(top, bottom int) {
	if top < 1 {
		top = 1
	}
	if bottom > s.Rows {
		bottom = s.Rows
	}
	if bottom <= top {
		bottom = s.Rows
		top = 1
	}
	s.scrollTop = top - 1
	s.scrollBottom = bottom - 1
	s.SetCursor(0, 0)
}

// ViewOffset returns how many lines the view is scrolled back.
func (s *Screen) ViewOffset() int {
	max := len(s.scrollback)
	if s.viewOffset > max {
		return max
	}
	return s.viewOffset
}

// CursorPos returns the current cursor position.
func (s *Screen) CursorPos() (row, col int) {
	return s.Cursor.Row, s.Cursor.Col
}

// LineText returns the plain text of the given screen row, ignoring trailing
// empty cells. It is used to capture the current command line before sending
// Enter to the PTY.
func (s *Screen) LineText(row int) string {
	if row < 0 || row >= s.Rows {
		return ""
	}
	var b strings.Builder
	lastNonSpace := -1
	for c, cell := range s.Cells[row] {
		r := cell.Rune
		if r == 0 {
			r = ' '
		}
		b.WriteRune(r)
		if r != ' ' {
			lastNonSpace = c
		}
	}
	if lastNonSpace < 0 {
		return ""
	}
	return b.String()[:lastNonSpace+1]
}

// ScrollViewUp moves the view up into the scrollback by n lines.
func (s *Screen) ScrollViewUp(n int) {
	max := len(s.scrollback)
	if max == 0 {
		return
	}
	s.viewOffset += n
	if s.viewOffset > max {
		s.viewOffset = max
	}
}

// ScrollViewDown moves the view down towards the live screen by n lines.
func (s *Screen) ScrollViewDown(n int) {
	s.viewOffset -= n
	if s.viewOffset < 0 {
		s.viewOffset = 0
	}
}

// ResetView resets the view offset so the live screen is shown.
func (s *Screen) ResetView() {
	s.viewOffset = 0
}

// SetSync enables or disables synchronized output (ESC[?2026h/l).
// While synchronized output is active, Render() returns the last committed
// frame so intermediate redraw states are not visible.
func (s *Screen) SetSync(active bool) {
	s.syncActive = active
	if !active {
		s.lastRender = s.renderToString()
	}
}

// Render returns the screen as a string. If the view is scrolled back,
// lines are taken from the scrollback buffer; otherwise the live screen is
// rendered.
func (s *Screen) Render() string {
	if s.syncActive {
		return s.lastRender
	}
	s.lastRender = s.renderToString()
	return s.lastRender
}

// renderToString renders the current screen state without considering sync.
func (s *Screen) renderToString() string {
	var lines []string
	cursorHere := s.viewOffset == 0 && s.CursorBlinkVisible
	if s.viewOffset > 0 {
		max := len(s.scrollback)
		if s.viewOffset > max {
			s.viewOffset = max
		}
		for r := 0; r < s.Rows; r++ {
			sbIdx := max - s.viewOffset + r
			if sbIdx >= 0 && sbIdx < max {
				lines = append(lines, s.renderScrollbackLine(sbIdx))
			} else if sbIdx >= max {
				lines = append(lines, s.renderLine(sbIdx-max, cursorHere && sbIdx-max == s.Cursor.Row))
			} else {
				lines = append(lines, strings.Repeat(" ", s.Cols))
			}
		}
	} else {
		for r := 0; r < s.Rows; r++ {
			lines = append(lines, s.renderLine(r, cursorHere && r == s.Cursor.Row))
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Screen) renderLine(r int, cursorRow bool) string {
	if r < 0 || r >= len(s.Cells) {
		return strings.Repeat(" ", s.Cols)
	}
	return s.renderCells(s.Cells[r], cursorRow, r)
}

func (s *Screen) renderScrollbackLine(idx int) string {
	if idx < 0 || idx >= len(s.scrollback) {
		return strings.Repeat(" ", s.Cols)
	}
	return s.renderCells(s.scrollback[idx], false, idx)
}

func (s *Screen) renderCells(cells []Cell, cursorRow bool, row int) string {
	var b strings.Builder
	var lastFG, lastBG lipgloss.Color
	var lastStyle StyleBits

	for c := 0; c < s.Cols; c++ {
		cell := Cell{Rune: ' '}
		if c < len(cells) {
			cell = cells[c]
		}
		if cell.Rune == 0 {
			cell.Rune = ' '
		}

		cursorHere := cursorRow && c == s.Cursor.Col
		selHere := s.cellInSelection(row, c)

		if cursorHere || selHere {
			// Close current style before applying reverse video (cursor
			// or selection highlight).
			if lastFG != "" || lastBG != "" || lastStyle != 0 {
				b.WriteString("\x1b[0m")
			}
			b.WriteString("\x1b[7m")
			b.WriteRune(cell.Rune)
			b.WriteString("\x1b[0m")
			// Re-emit the cell's style for subsequent cells.
			if cell.FG != "" || cell.BG != "" || cell.Style != 0 {
				b.WriteString(renderStyle(cell.FG, cell.BG, cell.Style))
				lastFG = cell.FG
				lastBG = cell.BG
				lastStyle = cell.Style
			} else {
				lastFG = ""
				lastBG = ""
				lastStyle = 0
			}
			continue
		}

		// Emit style changes only when needed
		if cell.FG != lastFG || cell.BG != lastBG || cell.Style != lastStyle {
			b.WriteString(renderStyle(cell.FG, cell.BG, cell.Style))
			lastFG = cell.FG
			lastBG = cell.BG
			lastStyle = cell.Style
		}
		b.WriteRune(cell.Rune)
	}
	// Reset style at end of line
	if lastFG != "" || lastBG != "" || lastStyle != 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func renderStyle(fg, bg lipgloss.Color, style StyleBits) string {
	var parts []string
	parts = append(parts, "0") // reset first
	if style&StyleBold != 0 {
		parts = append(parts, "1")
	}
	if style&StyleDim != 0 {
		parts = append(parts, "2")
	}
	if style&StyleItalic != 0 {
		parts = append(parts, "3")
	}
	if style&StyleUnderline != 0 {
		parts = append(parts, "4")
	}
	if style&StyleBlink != 0 {
		parts = append(parts, "5")
	}
	if style&StyleReverse != 0 {
		parts = append(parts, "7")
	}
	if style&StyleHidden != 0 {
		parts = append(parts, "8")
	}
	if style&StyleStrikethrough != 0 {
		parts = append(parts, "9")
	}
	if fg != "" {
		parts = append(parts, sgrColor(fg, false))
	}
	if bg != "" {
		parts = append(parts, sgrColor(bg, true))
	}
	return "\x1b[" + strings.Join(parts, ";") + "m"
}

func sgrColor(c lipgloss.Color, bg bool) string {
	s := string(c)
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		base := 38
		if bg {
			base = 48
		}
		r, _ := strconv.ParseInt(s[1:3], 16, 0)
		g, _ := strconv.ParseInt(s[3:5], 16, 0)
		b, _ := strconv.ParseInt(s[5:7], 16, 0)
		return fmt.Sprintf("%d;2;%d;%d;%d", base, r, g, b)
	}
	return ""
}
